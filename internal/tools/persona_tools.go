package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/smtp"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/everydev1618/tron/internal/knowledge"
	"github.com/everydev1618/tron/internal/notification"
	"github.com/everydev1618/tron/internal/subdomain"
	"github.com/everydev1618/govega"
	"github.com/everydev1618/govega/container"
	"github.com/everydev1618/govega/dsl"
	"gopkg.in/yaml.v3"
)

// SlackPoster interface for sending Slack messages (for testability)
type SlackPoster interface {
	SendMessage(channel, text string) error
}

// PersonaTools provides Tony's orchestration tools
type PersonaTools struct {
	orch       *vega.Orchestrator
	config     *dsl.Document
	contacts   *ContactDB
	workingDir string
	tronDir    string

	// Container management
	containers *container.Manager
	projects   *container.ProjectRegistry

	// Server process management (for *.hellotron.com routing)
	processManager *subdomain.ProcessManager

	// Track spawned agents and their callbacks
	callbacks   map[string]CallbackConfig
	callbacksMu sync.RWMutex

	// Channel context for spawned processes (for notifications)
	processChannels   map[string]notification.ChannelContext
	processChannelsMu sync.RWMutex

	// Slack client for notifications
	slackClient SlackPoster

	// Memory storage
	directives    map[string]string
	personMemory  map[string]map[string]string
	directivesMu  sync.RWMutex
	personMemMu   sync.RWMutex

	// Shared knowledge store
	knowledgeStore *knowledge.Store
}

// CallbackConfig stores callback information for spawned agents
type CallbackConfig struct {
	Email   string
	Subject string
	SpawnedAt time.Time
}

// ContactDB provides contact lookup
type ContactDB struct {
	contacts map[string]Contact
	mu       sync.RWMutex
}

// Contact represents a contact entry
type Contact struct {
	Name    string            `yaml:"name"`
	Phone   string            `yaml:"phone"`
	Email   string            `yaml:"email"`
	Company string            `yaml:"company,omitempty"`
	Role    string            `yaml:"role,omitempty"`
	Notes   string            `yaml:"notes,omitempty"`
	Tags    []string          `yaml:"tags,omitempty"`
	Meta    map[string]string `yaml:"meta,omitempty"`
}

// NewPersonaTools creates a new PersonaTools instance
func NewPersonaTools(orch *vega.Orchestrator, config *dsl.Document, workingDir, tronDir string, cm *container.Manager) *PersonaTools {
	pt := &PersonaTools{
		orch:            orch,
		config:          config,
		contacts:        &ContactDB{contacts: make(map[string]Contact)},
		workingDir:      workingDir,
		tronDir:         tronDir,
		containers:      cm,
		callbacks:       make(map[string]CallbackConfig),
		processChannels: make(map[string]notification.ChannelContext),
		directives:      make(map[string]string),
		personMemory:    make(map[string]map[string]string),
	}

	// Initialize shared knowledge store
	if ks, err := knowledge.NewStore(tronDir); err == nil {
		pt.knowledgeStore = ks
	} else {
		log.Printf("[tools] Failed to initialize knowledge store: %v", err)
	}

	// Create project registry if container manager is available
	if cm != nil {
		registry, err := container.NewProjectRegistry(workingDir, cm)
		if err == nil {
			pt.projects = registry
		}
	}

	// Load contacts from knowledge directory (check tron dir first, then current dir)
	knowledgeDir := filepath.Join(tronDir, "knowledge")
	if err := pt.loadContacts(filepath.Join(knowledgeDir, "contacts.yaml")); err != nil {
		// Try current directory as fallback
		pt.loadContacts("knowledge/contacts.yaml")
	}

	return pt
}

// SetProcessManager sets the server process manager for subdomain routing
func (pt *PersonaTools) SetProcessManager(pm *subdomain.ProcessManager) {
	pt.processManager = pm
}

// loadContacts loads contacts from a YAML file
func (pt *PersonaTools) loadContacts(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err // File may not exist yet
	}

	var contactList struct {
		Contacts []Contact `yaml:"contacts"`
	}
	if err := yaml.Unmarshal(data, &contactList); err != nil {
		return err
	}

	pt.contacts.mu.Lock()
	defer pt.contacts.mu.Unlock()

	for _, c := range contactList.Contacts {
		// Index by phone number (normalized)
		phone := normalizePhone(c.Phone)
		pt.contacts.contacts[phone] = c
	}

	return nil
}

// normalizePhone strips non-numeric characters from phone numbers
func normalizePhone(phone string) string {
	var normalized strings.Builder
	for _, r := range phone {
		if r >= '0' && r <= '9' {
			normalized.WriteRune(r)
		}
	}
	return normalized.String()
}

// RegisterTo registers all persona tools to a vega.Tools instance
func (pt *PersonaTools) RegisterTo(tools *vega.Tools) {
	// spawn_agent - Delegate work to a team member
	tools.Register("spawn_agent", vega.ToolDef{
		Description: "Spawn a team member agent to handle a task. Returns the process ID.",
		Fn:          pt.spawnAgent,
		Params: map[string]vega.ParamDef{
			"agent": {
				Type:        "string",
				Description: "Name of the team member (Gary, Sarah, Derek, Kathy, Tiffany, Marcus, Vera, Claire, Leo)",
				Required:    true,
			},
			"task": {
				Type:        "string",
				Description: "Description of the task to delegate",
				Required:    true,
			},
			"context": {
				Type:        "string",
				Description: "Additional context or files to provide",
				Required:    false,
			},
		},
	})

	// schedule_callback - Request notification when work completes
	tools.Register("schedule_callback", vega.ToolDef{
		Description: "Schedule an email notification when a spawned agent completes its work",
		Fn:          pt.scheduleCallback,
		Params: map[string]vega.ParamDef{
			"process_id": {
				Type:        "string",
				Description: "Process ID from spawn_agent",
				Required:    true,
			},
			"email": {
				Type:        "string",
				Description: "Email address to notify",
				Required:    true,
			},
			"subject": {
				Type:        "string",
				Description: "Subject line for the notification email",
				Required:    false,
			},
		},
	})

	// identify_caller - Look up caller by phone number
	tools.Register("identify_caller", vega.ToolDef{
		Description: "Look up a caller by their phone number",
		Fn:          pt.identifyCallerTool,
		Params: map[string]vega.ParamDef{
			"phone": {
				Type:        "string",
				Description: "Phone number to look up",
				Required:    true,
			},
		},
	})

	// create_project - Set up a new project workspace
	tools.Register("create_project", vega.ToolDef{
		Description: "Create a new project workspace in the work directory",
		Fn:          pt.createProject,
		Params: map[string]vega.ParamDef{
			"name": {
				Type:        "string",
				Description: "Project name (will be used as directory name)",
				Required:    true,
			},
			"description": {
				Type:        "string",
				Description: "Brief description of the project",
				Required:    false,
			},
			"template": {
				Type:        "string",
				Description: "Project template (go, python, node, react, empty)",
				Required:    false,
			},
		},
	})

	// save_directive - Save an important instruction
	tools.Register("save_directive", vega.ToolDef{
		Description: "Save an important instruction or directive for future reference",
		Fn:          pt.saveDirective,
		Params: map[string]vega.ParamDef{
			"key": {
				Type:        "string",
				Description: "Short key to identify the directive",
				Required:    true,
			},
			"directive": {
				Type:        "string",
				Description: "The directive or instruction to remember",
				Required:    true,
			},
		},
	})

	// save_person_memory - Remember facts about a person
	tools.Register("save_person_memory", vega.ToolDef{
		Description: "Save facts about a person for future conversations",
		Fn:          pt.savePersonMemory,
		Params: map[string]vega.ParamDef{
			"person": {
				Type:        "string",
				Description: "Person's name or identifier",
				Required:    true,
			},
			"key": {
				Type:        "string",
				Description: "Category of fact (e.g., 'preference', 'project', 'context')",
				Required:    true,
			},
			"fact": {
				Type:        "string",
				Description: "The fact to remember",
				Required:    true,
			},
		},
	})

	// web_search - Search the web
	tools.Register("web_search", vega.ToolDef{
		Description: "Search the web for current information (stub - implement with real search API)",
		Fn:          pt.webSearch,
		Params: map[string]vega.ParamDef{
			"query": {
				Type:        "string",
				Description: "Search query",
				Required:    true,
			},
		},
	})

	// execute - Run shell commands (in container if available)
	execDesc := "Execute a shell command in the working directory"
	if pt.containers != nil && pt.containers.IsAvailable() {
		execDesc = "Execute a shell command. If a project is specified, runs inside the project's Docker container"
	}
	tools.Register("execute", vega.ToolDef{
		Description: execDesc,
		Fn:          pt.execute,
		Params: map[string]vega.ParamDef{
			"command": {
				Type:        "string",
				Description: "The shell command to execute",
				Required:    true,
			},
			"project": {
				Type:        "string",
				Description: "Project name to execute in (uses container if available)",
				Required:    false,
			},
		},
	})

	// get_project_status - Check container status for a project
	tools.Register("get_project_status", vega.ToolDef{
		Description: "Get the status of a project's container (running, stopped, etc.)",
		Fn:          pt.getProjectStatus,
		Params: map[string]vega.ParamDef{
			"project": {
				Type:        "string",
				Description: "Project name to check",
				Required:    true,
			},
		},
	})

	// start_server - Start a server process for a project and get its public URL
	tools.Register("start_server", vega.ToolDef{
		Description: "Start a server process for a project. Returns a unique public URL (https://xxxx.hellotron.com) that routes to the server.",
		Fn:          pt.startServer,
		Params: map[string]vega.ParamDef{
			"project": {
				Type:        "string",
				Description: "Project name (must exist)",
				Required:    true,
			},
			"command": {
				Type:        "string",
				Description: "Command to start the server (will receive PORT env variable)",
				Required:    true,
			},
		},
	})

	// stop_server - Stop a running server
	tools.Register("stop_server", vega.ToolDef{
		Description: "Stop a running server for a project",
		Fn:          pt.stopServer,
		Params: map[string]vega.ParamDef{
			"project": {
				Type:        "string",
				Description: "Project name",
				Required:    true,
			},
		},
	})

	// get_server_url - Get the URL of a running server
	tools.Register("get_server_url", vega.ToolDef{
		Description: "Get the public URL of a running server for a project",
		Fn:          pt.getServerURL,
		Params: map[string]vega.ParamDef{
			"project": {
				Type:        "string",
				Description: "Project name",
				Required:    true,
			},
		},
	})

	// list_servers - List all running servers
	tools.Register("list_servers", vega.ToolDef{
		Description: "List all running project servers with their URLs",
		Fn:          pt.listServers,
		Params:      map[string]vega.ParamDef{},
	})

	// list_projects - List all projects
	tools.Register("list_projects", vega.ToolDef{
		Description: "List all projects in the work directory. Use this to see what projects exist before answering questions about current work.",
		Fn:          pt.listProjects,
		Params:      map[string]vega.ParamDef{},
	})

	// share_knowledge - Share a discovery, insight, or decision with the team
	tools.Register("share_knowledge", vega.ToolDef{
		Description: "Share a discovery, insight, decision, or task result with the team. Other team members will see this in their knowledge feed.",
		Fn:          pt.shareKnowledge,
		Params: map[string]vega.ParamDef{
			"type": {
				Type:        "string",
				Description: "Type of knowledge: discovery, insight, decision, task_result, or resource",
				Required:    true,
			},
			"title": {
				Type:        "string",
				Description: "Brief title summarizing the knowledge (1-2 sentences)",
				Required:    true,
			},
			"content": {
				Type:        "string",
				Description: "Full details of the knowledge to share",
				Required:    true,
			},
			"domain": {
				Type:        "string",
				Description: "Domain: tech, marketing, finance, ops, product, or general (defaults to your domain)",
				Required:    false,
			},
			"tags": {
				Type:        "string",
				Description: "Comma-separated tags for categorization",
				Required:    false,
			},
		},
	})

	// query_knowledge - Search the shared knowledge base
	tools.Register("query_knowledge", vega.ToolDef{
		Description: "Search the shared knowledge base for entries by domain, author, type, or tags. Use this to find what other team members have discovered.",
		Fn:          pt.queryKnowledge,
		Params: map[string]vega.ParamDef{
			"domain": {
				Type:        "string",
				Description: "Filter by domain: tech, marketing, finance, ops, product, general",
				Required:    false,
			},
			"author": {
				Type:        "string",
				Description: "Filter by author name (e.g., Tony, Maya, Gary)",
				Required:    false,
			},
			"type": {
				Type:        "string",
				Description: "Filter by type: discovery, insight, decision, task_result, resource",
				Required:    false,
			},
			"tags": {
				Type:        "string",
				Description: "Comma-separated tags to filter by",
				Required:    false,
			},
			"limit": {
				Type:        "number",
				Description: "Maximum number of results (default 10)",
				Required:    false,
			},
		},
	})

	// get_knowledge_feed - Get recent team activity
	tools.Register("get_knowledge_feed", vega.ToolDef{
		Description: "Get a digest of recent team knowledge and activity from the last 24 hours. Shows what other team members have discovered or decided.",
		Fn:          pt.getKnowledgeFeed,
		Params:      map[string]vega.ParamDef{},
	})
}

// spawnAgent spawns a team member agent
func (pt *PersonaTools) spawnAgent(ctx context.Context, params map[string]any) (string, error) {
	agentName, _ := params["agent"].(string)
	task, _ := params["task"].(string)
	taskContext, _ := params["context"].(string)

	// Get agent definition from config
	agentDef, ok := pt.config.Agents[agentName]
	if !ok {
		return "", fmt.Errorf("unknown team member: %s", agentName)
	}

	// Build the agent with both builtin and custom tools
	vegaTools := vega.NewTools(vega.WithSandbox(pt.workingDir))
	vegaTools.RegisterBuiltins()

	// Register custom persona tools (list_servers, start_server, etc.)
	pt.RegisterTo(vegaTools)

	if len(agentDef.Tools) > 0 {
		vegaTools = vegaTools.Filter(agentDef.Tools...)
	}

	agent := vega.Agent{
		Name:   agentDef.Name,
		Model:  agentDef.Model,
		System: vega.StaticPrompt(agentDef.System),
		Tools:  vegaTools,
	}

	if agentDef.Temperature != nil {
		agent.Temperature = agentDef.Temperature
	}
	if agentDef.Budget != "" {
		agent.Budget = parseBudget(agentDef.Budget)
	}

	// Create supervision from config
	var supervision vega.Supervision
	if agentDef.Supervision != nil {
		supervision = vega.Supervision{
			Strategy:    parseStrategy(agentDef.Supervision.Strategy),
			MaxRestarts: agentDef.Supervision.MaxRestarts,
			Window:      parseWindow(agentDef.Supervision.Window),
		}
	} else {
		supervision = vega.Supervision{
			Strategy:    vega.Restart,
			MaxRestarts: 3,
		}
	}

	// Build spawn options
	spawnOpts := []vega.SpawnOption{
		vega.WithSupervision(supervision),
		vega.WithTask(task),
		vega.WithWorkDir(pt.workingDir),
		vega.WithSpawnReason(task),
	}

	// Get the parent process from context for spawn tree tracking
	if parentProc := vega.ProcessFromContext(ctx); parentProc != nil {
		spawnOpts = append(spawnOpts, vega.WithParent(parentProc))
	}

	// Spawn the process
	proc, err := pt.orch.Spawn(agent, spawnOpts...)
	if err != nil {
		return "", fmt.Errorf("failed to spawn %s: %w", agentName, err)
	}

	// Send the initial task
	fullTask := task
	if taskContext != "" {
		fullTask = fmt.Sprintf("%s\n\nContext:\n%s", task, taskContext)
	}

	// After spawning, capture channel context for automatic notifications
	if ch, ok := notification.ChannelFromContext(ctx); ok {
		pt.processChannelsMu.Lock()
		pt.processChannels[proc.ID] = ch
		pt.processChannelsMu.Unlock()
	}

	// Set up the callback handler (idempotent, only runs once)
	pt.setupCallbackHandlerOnce()

	// Send the task and handle completion in background
	future := proc.SendAsync(fullTask)

	// Wait for completion and mark process as done
	go func() {
		result, err := future.Await(context.Background())
		if err != nil {
			proc.Fail(err)
		} else {
			proc.Complete(result)
		}
	}()

	return fmt.Sprintf("Spawned %s (process ID: %s) to work on: %s", agentName, proc.ID, task), nil
}

// parseStrategy converts a string strategy to vega.Strategy
func parseStrategy(s string) vega.Strategy {
	switch strings.ToLower(s) {
	case "restart":
		return vega.Restart
	case "stop":
		return vega.Stop
	case "escalate":
		return vega.Escalate
	case "restartall":
		return vega.RestartAll
	default:
		return vega.Restart
	}
}

// parseWindow converts a duration string like "10m" to time.Duration
func parseWindow(s string) time.Duration {
	d, _ := time.ParseDuration(s)
	return d
}

// parseBudget converts a budget string like "$5.00" to a Budget struct
func parseBudget(s string) *vega.Budget {
	s = strings.TrimPrefix(s, "$")
	limit, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return nil
	}
	return &vega.Budget{
		Limit:    limit,
		OnExceed: vega.BudgetWarn,
	}
}

// scheduleCallback schedules a notification callback
func (pt *PersonaTools) scheduleCallback(ctx context.Context, params map[string]any) (string, error) {
	processID, _ := params["process_id"].(string)
	email, _ := params["email"].(string)
	subject, _ := params["subject"].(string)

	if subject == "" {
		subject = "Task Completed"
	}

	// Get the process
	proc := pt.orch.Get(processID)
	if proc == nil {
		return "", fmt.Errorf("process not found: %s", processID)
	}

	pt.callbacksMu.Lock()
	pt.callbacks[processID] = CallbackConfig{
		Email:     email,
		Subject:   subject,
		SpawnedAt: time.Now(),
	}
	pt.callbacksMu.Unlock()

	// Note: OnProcessComplete is a global callback, so we check the process ID in the callback
	// This is a one-time setup - multiple schedules for different processes are okay
	pt.setupCallbackHandlerOnce()

	return fmt.Sprintf("Callback scheduled. Will notify %s when process %s completes.", email, processID), nil
}

var callbackHandlerSetup sync.Once

// setupCallbackHandlerOnce sets up the global completion callback (only once)
func (pt *PersonaTools) setupCallbackHandlerOnce() {
	callbackHandlerSetup.Do(func() {
		pt.orch.OnProcessComplete(func(p *vega.Process, result string) {
			// Existing email callback logic
			pt.callbacksMu.RLock()
			callback, ok := pt.callbacks[p.ID]
			pt.callbacksMu.RUnlock()

			if ok {
				pt.sendCallbackEmail(callback.Email, callback.Subject, result)

				// Clean up after sending
				pt.callbacksMu.Lock()
				delete(pt.callbacks, p.ID)
				pt.callbacksMu.Unlock()
			}

			// Channel-aware notifications
			pt.processChannelsMu.RLock()
			ch, hasChannel := pt.processChannels[p.ID]
			pt.processChannelsMu.RUnlock()

			if hasChannel {
				pt.notifyChannel(ch, p, result)
				pt.processChannelsMu.Lock()
				delete(pt.processChannels, p.ID)
				pt.processChannelsMu.Unlock()
			}
		})
	})
}

// sendCallbackEmail sends a notification email
func (pt *PersonaTools) sendCallbackEmail(to, subject, body string) error {
	smtpHost := os.Getenv("SMTP_HOST")
	smtpPort := os.Getenv("SMTP_PORT")
	smtpUser := os.Getenv("SMTP_USER")
	smtpPass := os.Getenv("SMTP_PASS")
	fromEmail := os.Getenv("SMTP_FROM")

	if smtpHost == "" {
		// Log but don't fail if SMTP not configured
		fmt.Printf("SMTP not configured, would send email to %s: %s\n", to, subject)
		return nil
	}

	if smtpPort == "" {
		smtpPort = "587"
	}
	if fromEmail == "" {
		fromEmail = smtpUser
	}

	msg := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\n\r\n%s",
		fromEmail, to, subject, body)

	auth := smtp.PlainAuth("", smtpUser, smtpPass, smtpHost)
	return smtp.SendMail(smtpHost+":"+smtpPort, auth, fromEmail, []string{to}, []byte(msg))
}

// SetSlackClient sets the Slack client for sending notifications
func (pt *PersonaTools) SetSlackClient(client SlackPoster) {
	pt.slackClient = client
}

// notifyChannel sends a completion notification to the appropriate channel
func (pt *PersonaTools) notifyChannel(ch notification.ChannelContext, p *vega.Process, result string) {
	agentName := "Agent"
	if p.Agent != nil {
		agentName = p.Agent.Name
	}

	switch ch.Type {
	case notification.ChannelSlack:
		if pt.slackClient != nil {
			msg := fmt.Sprintf("*%s* completed: _%s_\n\n%s",
				agentName, p.Task, summarizeResult(result, 500))
			if err := pt.slackClient.SendMessage(ch.ChannelID, msg); err != nil {
				log.Printf("[notification] Failed to send Slack notification: %v", err)
			}
		} else {
			log.Printf("[notification] Slack client not configured, cannot notify channel %s", ch.ChannelID)
		}

	case notification.ChannelVoice:
		// Voice calls have ended - send email if available
		if ch.Email != "" {
			pt.sendCallbackEmail(ch.Email,
				fmt.Sprintf("%s completed your request", agentName),
				result)
		} else {
			// Otherwise log only - user can't be notified
			log.Printf("[notification] Voice call completed for %s, no notification channel available", ch.UserID)
		}

	case notification.ChannelAPI:
		// API calls are synchronous, no notification needed
		log.Printf("[notification] API process %s completed", p.ID)
	}
}

// summarizeResult truncates result to maxLen characters
func summarizeResult(result string, maxLen int) string {
	if len(result) <= maxLen {
		return result
	}
	return result[:maxLen] + "..."
}

// identifyCallerTool wraps IdentifyCaller as a tool
func (pt *PersonaTools) identifyCallerTool(ctx context.Context, params map[string]any) (string, error) {
	phone, _ := params["phone"].(string)
	result := pt.IdentifyCaller(phone)
	if result == "" {
		return "Unknown caller", nil
	}
	return result, nil
}

// IdentifyCaller looks up a caller by phone number (exported for server use)
func (pt *PersonaTools) IdentifyCaller(phone string) string {
	normalized := normalizePhone(phone)

	pt.contacts.mu.RLock()
	defer pt.contacts.mu.RUnlock()

	contact, ok := pt.contacts.contacts[normalized]
	if !ok {
		return ""
	}

	var info strings.Builder
	info.WriteString(fmt.Sprintf("Name: %s\n", contact.Name))
	if contact.Company != "" {
		info.WriteString(fmt.Sprintf("Company: %s\n", contact.Company))
	}
	if contact.Role != "" {
		info.WriteString(fmt.Sprintf("Role: %s\n", contact.Role))
	}
	if contact.Email != "" {
		info.WriteString(fmt.Sprintf("Email: %s\n", contact.Email))
	}
	if contact.Notes != "" {
		info.WriteString(fmt.Sprintf("Notes: %s\n", contact.Notes))
	}
	if len(contact.Tags) > 0 {
		info.WriteString(fmt.Sprintf("Tags: %s\n", strings.Join(contact.Tags, ", ")))
	}

	return info.String()
}

// createProject creates a new project workspace
func (pt *PersonaTools) createProject(ctx context.Context, params map[string]any) (string, error) {
	name, _ := params["name"].(string)
	description, _ := params["description"].(string)
	template, _ := params["template"].(string)

	// Sanitize project name
	safeName := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			return r
		}
		return '-'
	}, name)

	var projectDir string
	var containerStatus string

	// Use project registry if available (creates container)
	if pt.projects != nil {
		project, err := pt.projects.GetOrCreateProject(ctx, safeName, description, "")
		if err != nil {
			return "", fmt.Errorf("failed to create project: %w", err)
		}
		projectDir = pt.projects.GetProjectPath(safeName)
		containerStatus = fmt.Sprintf("\nContainer status: %s", project.Status)
	} else {
		// Fallback to simple directory creation
		projectDir = filepath.Join(pt.workingDir, "projects", safeName)
		if err := os.MkdirAll(projectDir, 0755); err != nil {
			return "", fmt.Errorf("failed to create project directory: %w", err)
		}
	}

	// Create README
	readme := fmt.Sprintf("# %s\n\n%s\n\nCreated: %s\n", name, description, time.Now().Format(time.RFC3339))
	if err := os.WriteFile(filepath.Join(projectDir, "README.md"), []byte(readme), 0644); err != nil {
		return "", fmt.Errorf("failed to create README: %w", err)
	}

	// Apply template if specified
	if template != "" {
		if err := pt.applyTemplate(projectDir, template); err != nil {
			return "", fmt.Errorf("failed to apply template: %w", err)
		}
	}

	return fmt.Sprintf("Created project '%s' at %s%s", name, projectDir, containerStatus), nil
}

// applyTemplate applies a project template
func (pt *PersonaTools) applyTemplate(dir, template string) error {
	switch template {
	case "go":
		return os.WriteFile(filepath.Join(dir, "main.go"), []byte(`package main

import "fmt"

func main() {
	fmt.Println("Hello, World!")
}
`), 0644)

	case "python":
		return os.WriteFile(filepath.Join(dir, "main.py"), []byte(`#!/usr/bin/env python3

def main():
    print("Hello, World!")

if __name__ == "__main__":
    main()
`), 0644)

	case "node":
		os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{
  "name": "project",
  "version": "1.0.0",
  "main": "index.js"
}
`), 0644)
		return os.WriteFile(filepath.Join(dir, "index.js"), []byte(`console.log("Hello, World!");
`), 0644)

	case "react":
		// Just create a basic structure
		os.MkdirAll(filepath.Join(dir, "src"), 0755)
		return os.WriteFile(filepath.Join(dir, "src", "App.jsx"), []byte(`export default function App() {
  return <h1>Hello, World!</h1>;
}
`), 0644)

	case "empty":
		return nil

	default:
		return fmt.Errorf("unknown template: %s", template)
	}
}

// saveDirective saves a directive
func (pt *PersonaTools) saveDirective(ctx context.Context, params map[string]any) (string, error) {
	key, _ := params["key"].(string)
	directive, _ := params["directive"].(string)

	pt.directivesMu.Lock()
	pt.directives[key] = directive
	pt.directivesMu.Unlock()

	// Persist to file
	pt.persistDirectives()

	return fmt.Sprintf("Saved directive '%s': %s", key, directive), nil
}

// persistDirectives saves directives to disk
func (pt *PersonaTools) persistDirectives() error {
	pt.directivesMu.RLock()
	defer pt.directivesMu.RUnlock()

	data, err := yaml.Marshal(pt.directives)
	if err != nil {
		return err
	}

	knowledgeDir := filepath.Join(pt.tronDir, "knowledge")
	os.MkdirAll(knowledgeDir, 0755)
	return os.WriteFile(filepath.Join(knowledgeDir, "directives.yaml"), data, 0644)
}

// savePersonMemory saves memory about a person
func (pt *PersonaTools) savePersonMemory(ctx context.Context, params map[string]any) (string, error) {
	person, _ := params["person"].(string)
	key, _ := params["key"].(string)
	fact, _ := params["fact"].(string)

	pt.personMemMu.Lock()
	if pt.personMemory[person] == nil {
		pt.personMemory[person] = make(map[string]string)
	}
	pt.personMemory[person][key] = fact
	pt.personMemMu.Unlock()

	// Persist to file
	pt.persistPersonMemory()

	return fmt.Sprintf("Remembered about %s: %s = %s", person, key, fact), nil
}

// persistPersonMemory saves person memory to disk
func (pt *PersonaTools) persistPersonMemory() error {
	pt.personMemMu.RLock()
	defer pt.personMemMu.RUnlock()

	data, err := yaml.Marshal(pt.personMemory)
	if err != nil {
		return err
	}

	knowledgeDir := filepath.Join(pt.tronDir, "knowledge")
	os.MkdirAll(knowledgeDir, 0755)
	return os.WriteFile(filepath.Join(knowledgeDir, "person_memory.yaml"), data, 0644)
}

// webSearch performs a web search using Brave Search API
func (pt *PersonaTools) webSearch(ctx context.Context, params map[string]any) (string, error) {
	query, _ := params["query"].(string)
	if query == "" {
		return "", fmt.Errorf("query is required")
	}

	apiKey := os.Getenv("BRAVE_SEARCH_API_KEY")
	if apiKey == "" {
		return "", fmt.Errorf("BRAVE_SEARCH_API_KEY not configured")
	}

	// Build request
	searchURL := "https://api.search.brave.com/res/v1/web/search"
	req, err := http.NewRequestWithContext(ctx, "GET", searchURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	// Add query parameters
	q := req.URL.Query()
	q.Add("q", query)
	q.Add("count", "5") // Top 5 results
	req.URL.RawQuery = q.Encode()

	// Add headers
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Subscription-Token", apiKey)

	// Execute request
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("search request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("search API returned status %d", resp.StatusCode)
	}

	// Parse response
	var result braveSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	// Format results
	var output strings.Builder
	output.WriteString(fmt.Sprintf("Search results for: %s\n\n", query))

	if len(result.Web.Results) == 0 {
		output.WriteString("No results found.")
		return output.String(), nil
	}

	for i, r := range result.Web.Results {
		output.WriteString(fmt.Sprintf("%d. %s\n", i+1, r.Title))
		output.WriteString(fmt.Sprintf("   URL: %s\n", r.URL))
		if r.Description != "" {
			output.WriteString(fmt.Sprintf("   %s\n", r.Description))
		}
		output.WriteString("\n")
	}

	return output.String(), nil
}

// braveSearchResponse represents the Brave Search API response
type braveSearchResponse struct {
	Web struct {
		Results []struct {
			Title       string `json:"title"`
			URL         string `json:"url"`
			Description string `json:"description"`
		} `json:"results"`
	} `json:"web"`
}

// execute runs a shell command, optionally in a project's container
func (pt *PersonaTools) execute(ctx context.Context, params map[string]any) (string, error) {
	command, _ := params["command"].(string)
	project, _ := params["project"].(string)

	if command == "" {
		return "", fmt.Errorf("command is required")
	}

	// Security: block dangerous commands
	blockedPatterns := []string{
		"rm -rf /",
		"rm -rf /*",
		"sudo",
		"su ",
		".ssh",
		".aws",
		"/etc/passwd",
		"curl.*metadata",
		"> /dev",
		"mkfs",
		"dd if=",
	}
	lowerCmd := strings.ToLower(command)
	for _, pattern := range blockedPatterns {
		if strings.Contains(lowerCmd, pattern) {
			return "", fmt.Errorf("blocked command: contains dangerous pattern %q", pattern)
		}
	}

	// If project specified and containers available, run in container
	if project != "" && pt.containers != nil && pt.containers.IsAvailable() {
		return pt.executeInContainer(ctx, project, command)
	}

	// Otherwise run on host
	return pt.executeOnHost(ctx, command, project)
}

// executeInContainer runs a command inside a project's Docker container
func (pt *PersonaTools) executeInContainer(ctx context.Context, project, command string) (string, error) {
	execCtx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()

	result, err := pt.containers.Exec(execCtx, project, []string{"bash", "-c", command}, "/workspace")
	if err != nil {
		return "", fmt.Errorf("container exec failed: %w", err)
	}

	var output strings.Builder
	if result.Stdout != "" {
		output.WriteString(result.Stdout)
	}
	if result.Stderr != "" {
		if output.Len() > 0 {
			output.WriteString("\n")
		}
		output.WriteString("stderr: ")
		output.WriteString(result.Stderr)
	}

	outputStr := output.String()
	if len(outputStr) > 50000 {
		outputStr = outputStr[:50000] + "\n... (truncated)"
	}

	if result.ExitCode != 0 {
		if outputStr == "" {
			return "", fmt.Errorf("command failed with exit code %d", result.ExitCode)
		}
		return outputStr + fmt.Sprintf("\n\nExit code: %d", result.ExitCode), nil
	}

	if outputStr == "" {
		return "(no output)", nil
	}
	return outputStr, nil
}

// executeOnHost runs a command on the host
func (pt *PersonaTools) executeOnHost(ctx context.Context, command, project string) (string, error) {
	// Determine working directory
	workDir := pt.workingDir
	if project != "" {
		workDir = filepath.Join(pt.workingDir, "vega.work", "projects", project)
		if _, err := os.Stat(workDir); os.IsNotExist(err) {
			// Try without vega.work prefix
			workDir = filepath.Join(pt.workingDir, "projects", project)
		}
	}

	// Ensure working directory exists
	if err := os.MkdirAll(workDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create working directory: %w", err)
	}

	execCtx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()

	cmd := exec.CommandContext(execCtx, "bash", "-c", command)
	cmd.Dir = workDir

	output, err := cmd.CombinedOutput()
	outputStr := string(output)

	if len(outputStr) > 50000 {
		outputStr = outputStr[:50000] + "\n... (truncated)"
	}

	if err != nil {
		if execCtx.Err() != nil {
			return "", fmt.Errorf("command timed out after 120 seconds")
		}
		if outputStr == "" {
			return "", fmt.Errorf("command failed: %v", err)
		}
		return outputStr + fmt.Sprintf("\n\nError: %v", err), nil
	}

	if outputStr == "" {
		return "(no output)", nil
	}
	return outputStr, nil
}

// getProjectStatus returns the status of a project's container
func (pt *PersonaTools) getProjectStatus(ctx context.Context, params map[string]any) (string, error) {
	project, _ := params["project"].(string)

	if project == "" {
		return "", fmt.Errorf("project name is required")
	}

	if pt.containers == nil || !pt.containers.IsAvailable() {
		return "Docker not available - projects run in direct mode", nil
	}

	status, err := pt.containers.GetProjectStatus(ctx, project)
	if err != nil {
		return "", fmt.Errorf("failed to get project status: %w", err)
	}

	var result strings.Builder
	result.WriteString(fmt.Sprintf("Project: %s\n", project))
	if status.ContainerID != "" {
		result.WriteString(fmt.Sprintf("Container ID: %s\n", status.ContainerID))
	}
	result.WriteString(fmt.Sprintf("Running: %v\n", status.Running))
	if status.Image != "" {
		result.WriteString(fmt.Sprintf("Image: %s\n", status.Image))
	}
	if !status.Created.IsZero() {
		result.WriteString(fmt.Sprintf("Created: %s\n", status.Created.Format(time.RFC3339)))
	}

	return result.String(), nil
}

// startServer starts a server process for a project
func (pt *PersonaTools) startServer(ctx context.Context, params map[string]any) (string, error) {
	project, _ := params["project"].(string)
	command, _ := params["command"].(string)

	if project == "" {
		return "", fmt.Errorf("project name is required")
	}
	if command == "" {
		return "", fmt.Errorf("command is required")
	}

	if pt.processManager == nil {
		return "", fmt.Errorf("server management not available")
	}

	// Determine working directory for the project
	workDir := filepath.Join(pt.workingDir, "vega.work", "projects", project)
	if _, err := os.Stat(workDir); os.IsNotExist(err) {
		workDir = filepath.Join(pt.workingDir, "projects", project)
		if _, err := os.Stat(workDir); os.IsNotExist(err) {
			return "", fmt.Errorf("project %q not found", project)
		}
	}

	// Prepare environment
	env := os.Environ()

	// Start the server process
	proc, err := pt.processManager.StartServer(ctx, project, command, workDir, env)
	if err != nil {
		return "", fmt.Errorf("failed to start server: %w", err)
	}

	return fmt.Sprintf("Server started for project '%s'\nURL: %s\nPort: %d\nSubdomain: %s",
		project, proc.URL, proc.Port, proc.Subdomain), nil
}

// stopServer stops a running server
func (pt *PersonaTools) stopServer(ctx context.Context, params map[string]any) (string, error) {
	project, _ := params["project"].(string)

	if project == "" {
		return "", fmt.Errorf("project name is required")
	}

	if pt.processManager == nil {
		return "", fmt.Errorf("server management not available")
	}

	if err := pt.processManager.StopServer(project); err != nil {
		return "", err
	}

	return fmt.Sprintf("Server stopped for project '%s'", project), nil
}

// getServerURL gets the URL of a running server
func (pt *PersonaTools) getServerURL(ctx context.Context, params map[string]any) (string, error) {
	project, _ := params["project"].(string)

	if project == "" {
		return "", fmt.Errorf("project name is required")
	}

	if pt.processManager == nil {
		return "", fmt.Errorf("server management not available")
	}

	proc := pt.processManager.GetServer(project)
	if proc == nil {
		return "", fmt.Errorf("no server running for project %q", project)
	}

	return fmt.Sprintf("URL: %s\nStatus: %s\nPort: %d",
		proc.URL, proc.Status, proc.Port), nil
}

// listServers lists all running servers
func (pt *PersonaTools) listServers(ctx context.Context, params map[string]any) (string, error) {
	if pt.processManager == nil {
		return "", fmt.Errorf("server management not available")
	}

	servers := pt.processManager.ListServers()
	if len(servers) == 0 {
		return "No servers currently running", nil
	}

	var result strings.Builder
	result.WriteString(fmt.Sprintf("Running servers (%d):\n\n", len(servers)))

	for _, s := range servers {
		result.WriteString(fmt.Sprintf("Project: %s\n", s.ProjectName))
		result.WriteString(fmt.Sprintf("  URL: %s\n", s.URL))
		result.WriteString(fmt.Sprintf("  Port: %d\n", s.Port))
		result.WriteString(fmt.Sprintf("  Status: %s\n", s.Status))
		result.WriteString(fmt.Sprintf("  Started: %s\n\n", s.StartedAt.Format(time.RFC3339)))
	}

	return result.String(), nil
}

// listProjects lists all projects in the work directory
func (pt *PersonaTools) listProjects(ctx context.Context, params map[string]any) (string, error) {
	var projects []string

	// Check both possible project locations
	projectDirs := []string{
		filepath.Join(pt.workingDir, "projects"),
		filepath.Join(pt.workingDir, "vega.work", "projects"),
	}

	for _, dir := range projectDirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue // Directory may not exist
		}

		for _, entry := range entries {
			if entry.IsDir() && !strings.HasPrefix(entry.Name(), ".") {
				// Read README for description if it exists
				readmePath := filepath.Join(dir, entry.Name(), "README.md")
				description := ""
				if data, err := os.ReadFile(readmePath); err == nil {
					// Extract first non-header line as description
					lines := strings.Split(string(data), "\n")
					for _, line := range lines {
						line = strings.TrimSpace(line)
						if line != "" && !strings.HasPrefix(line, "#") {
							description = line
							break
						}
					}
				}

				info := entry.Name()
				if description != "" {
					info = fmt.Sprintf("%s - %s", entry.Name(), description)
				}
				projects = append(projects, info)
			}
		}
	}

	if len(projects) == 0 {
		return "No projects found. Use create_project to start a new project.", nil
	}

	var result strings.Builder
	result.WriteString(fmt.Sprintf("Projects (%d):\n", len(projects)))
	for _, p := range projects {
		result.WriteString(fmt.Sprintf("- %s\n", p))
	}

	return result.String(), nil
}

// shareKnowledge shares a discovery, insight, or decision with the team
func (pt *PersonaTools) shareKnowledge(ctx context.Context, params map[string]any) (string, error) {
	if pt.knowledgeStore == nil {
		return "", fmt.Errorf("knowledge store not available")
	}

	entryType, _ := params["type"].(string)
	title, _ := params["title"].(string)
	content, _ := params["content"].(string)
	domain, _ := params["domain"].(string)
	tagsStr, _ := params["tags"].(string)

	if title == "" {
		return "", fmt.Errorf("title is required")
	}
	if content == "" {
		return "", fmt.Errorf("content is required")
	}

	// Determine author from process context
	author := "Unknown"
	if proc := vega.ProcessFromContext(ctx); proc != nil && proc.Agent != nil {
		author = proc.Agent.Name
	}

	// Parse tags
	var tags []string
	if tagsStr != "" {
		for _, t := range strings.Split(tagsStr, ",") {
			t = strings.TrimSpace(t)
			if t != "" {
				tags = append(tags, t)
			}
		}
	}

	// Map type string to EntryType
	var kt knowledge.EntryType
	switch strings.ToLower(entryType) {
	case "discovery":
		kt = knowledge.TypeDiscovery
	case "insight":
		kt = knowledge.TypeInsight
	case "decision":
		kt = knowledge.TypeDecision
	case "task_result":
		kt = knowledge.TypeTaskResult
	case "resource":
		kt = knowledge.TypeResource
	default:
		kt = knowledge.TypeDiscovery
	}

	// Map domain string to Domain
	var kd knowledge.Domain
	switch strings.ToLower(domain) {
	case "tech":
		kd = knowledge.DomainTech
	case "marketing":
		kd = knowledge.DomainMarketing
	case "finance":
		kd = knowledge.DomainFinance
	case "ops":
		kd = knowledge.DomainOps
	case "product":
		kd = knowledge.DomainProduct
	default:
		// Default to author's domain
		kd = knowledge.DomainFromPersona(author)
	}

	// Build source from context
	var source *knowledge.Source
	if proc := vega.ProcessFromContext(ctx); proc != nil {
		source = &knowledge.Source{
			ProcessID: proc.ID,
		}
	}

	entry := knowledge.Entry{
		Type:    kt,
		Domain:  kd,
		Author:  author,
		Title:   title,
		Content: content,
		Tags:    tags,
		Source:  source,
	}

	if err := pt.knowledgeStore.Add(entry); err != nil {
		return "", fmt.Errorf("failed to save knowledge: %w", err)
	}

	return fmt.Sprintf("Knowledge shared: [%s] %s\nThis will appear in the team's knowledge feed.", kt, title), nil
}

// queryKnowledge searches the shared knowledge base
func (pt *PersonaTools) queryKnowledge(ctx context.Context, params map[string]any) (string, error) {
	if pt.knowledgeStore == nil {
		return "", fmt.Errorf("knowledge store not available")
	}

	domain, _ := params["domain"].(string)
	author, _ := params["author"].(string)
	entryType, _ := params["type"].(string)
	tagsStr, _ := params["tags"].(string)
	limitFloat, _ := params["limit"].(float64)

	limit := int(limitFloat)
	if limit == 0 {
		limit = 10
	}

	// Parse tags
	var tags []string
	if tagsStr != "" {
		for _, t := range strings.Split(tagsStr, ",") {
			t = strings.TrimSpace(t)
			if t != "" {
				tags = append(tags, t)
			}
		}
	}

	// Build query options
	opts := knowledge.QueryOptions{
		Limit: limit,
		Tags:  tags,
	}

	if domain != "" {
		opts.Domain = knowledge.Domain(strings.ToLower(domain))
	}
	if author != "" {
		opts.Author = author
	}
	if entryType != "" {
		opts.Type = knowledge.EntryType(strings.ToLower(entryType))
	}

	entries := pt.knowledgeStore.Query(opts)
	return knowledge.FormatEntriesForQuery(entries), nil
}

// getKnowledgeFeed returns the recent activity feed
func (pt *PersonaTools) getKnowledgeFeed(ctx context.Context, params map[string]any) (string, error) {
	if pt.knowledgeStore == nil {
		return "", fmt.Errorf("knowledge store not available")
	}

	feedSection := knowledge.GetFeedPromptSection(pt.knowledgeStore)
	if feedSection == "" {
		return "No recent team activity in the last 24 hours.", nil
	}

	return feedSection, nil
}

// GetKnowledgeStore returns the knowledge store for external use
func (pt *PersonaTools) GetKnowledgeStore() *knowledge.Store {
	return pt.knowledgeStore
}

// ListServersForDisplay returns a formatted string of running servers for Slack display
func (pt *PersonaTools) ListServersForDisplay() string {
	if pt.processManager == nil {
		return "Server management not available"
	}

	servers := pt.processManager.ListServers()
	if len(servers) == 0 {
		return "No servers currently running"
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("*Running Servers (%d):*\n", len(servers)))
	for _, s := range servers {
		sb.WriteString(fmt.Sprintf("• *%s*: %s (port %d, %s)\n", s.ProjectName, s.URL, s.Port, s.Status))
	}
	return sb.String()
}
