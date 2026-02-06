package callback

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/everydev1618/tron/internal/email"
	"github.com/everydev1618/tron/internal/vapi"
)

// Callback represents a pending callback request
type Callback struct {
	ID            string    `json:"id"`
	AgentID       string    `json:"agent_id"`
	AgentName     string    `json:"agent_name"`
	TaskSummary   string    `json:"task_summary"`
	ProjectName   string    `json:"project_name"`
	Method        string    `json:"method"` // "call", "email", or "both"
	CustomerPhone string    `json:"customer_phone,omitempty"`
	CustomerEmail string    `json:"customer_email,omitempty"`
	CustomerName  string    `json:"customer_name,omitempty"`
	PersonaName   string    `json:"persona_name"`
	RequestedAt   time.Time `json:"requested_at"`
	CompletedAt   time.Time `json:"completed_at,omitempty"`
	Status        string    `json:"status"` // "pending", "completed", "failed", "orphaned"
	Error         string    `json:"error,omitempty"`
	GroupID       string    `json:"group_id,omitempty"`
}

// CallbackGroup represents a batch of callbacks that complete together
type CallbackGroup struct {
	ID            string                    `json:"id"`
	AgentIDs      []string                  `json:"agent_ids"`
	Results       map[string]CompletionInfo `json:"results"`
	Method        string                    `json:"method"`
	CustomerPhone string                    `json:"customer_phone,omitempty"`
	CustomerEmail string                    `json:"customer_email,omitempty"`
	CustomerName  string                    `json:"customer_name,omitempty"`
	PersonaName   string                    `json:"persona_name"`
	RequestedAt   time.Time                 `json:"requested_at"`
	CompletedAt   time.Time                 `json:"completed_at,omitempty"`
	Status        string                    `json:"status"`
	Error         string                    `json:"error,omitempty"`
}

// CompletionInfo contains the result of a completed agent
type CompletionInfo struct {
	AgentID     string `json:"agent_id"`
	AgentName   string `json:"agent_name"`
	Result      string `json:"result"`
	ProjectName string `json:"project_name"`
	Error       string `json:"error,omitempty"`
}

// AgentInfo contains info for batch registration
type AgentInfo struct {
	ID          string
	Name        string
	TaskSummary string
	ProjectName string
}

// Registry manages callback requests
type Registry struct {
	mu           sync.RWMutex
	callbacks    map[string]*Callback      // agentID -> Callback
	groups       map[string]*CallbackGroup // groupID -> Group
	history      []*Callback               // completed callbacks (last 100)
	groupHistory []*CallbackGroup          // completed groups (last 50)

	vapiClient     *vapi.Client
	emailClient    *email.Client
	getServerURL   func(projectName string) string
	agentValidator func(agentID string) bool
	baseDir        string
	personaName    string
	personaEmail   string
}

// NewRegistry creates a new callback registry
func NewRegistry(vapiClient *vapi.Client, emailClient *email.Client, baseDir, personaName, personaEmail string) *Registry {
	r := &Registry{
		callbacks:    make(map[string]*Callback),
		groups:       make(map[string]*CallbackGroup),
		history:      make([]*Callback, 0, 100),
		groupHistory: make([]*CallbackGroup, 0, 50),
		vapiClient:   vapiClient,
		emailClient:  emailClient,
		baseDir:      baseDir,
		personaName:  personaName,
		personaEmail: personaEmail,
	}

	// Load persisted callbacks
	r.load()

	return r
}

// SetServerURLFunc sets the function to get server URLs for projects
func (r *Registry) SetServerURLFunc(fn func(projectName string) string) {
	r.getServerURL = fn
}

// SetAgentValidator sets the function to validate agent existence
func (r *Registry) SetAgentValidator(fn func(agentID string) bool) {
	r.agentValidator = fn
	r.cleanupOrphaned()
}

// Register creates a new callback request
func (r *Registry) Register(agentID, agentName, taskSummary, projectName, method, phone, emailAddr, customerName string) (*Callback, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Validate method requirements
	if method == "call" || method == "both" {
		if phone == "" {
			return nil, fmt.Errorf("phone number required for call callback")
		}
		if r.vapiClient == nil || !r.vapiClient.IsConfigured() {
			return nil, fmt.Errorf("VAPI not configured for call callbacks")
		}
	}
	if method == "email" || method == "both" {
		if emailAddr == "" {
			return nil, fmt.Errorf("email address required for email callback")
		}
		if r.emailClient == nil || !r.emailClient.IsConfigured() {
			return nil, fmt.Errorf("email not configured for email callbacks")
		}
	}

	cb := &Callback{
		ID:            fmt.Sprintf("cb-%s-%d", agentID, time.Now().UnixNano()),
		AgentID:       agentID,
		AgentName:     agentName,
		TaskSummary:   taskSummary,
		ProjectName:   projectName,
		Method:        method,
		CustomerPhone: phone,
		CustomerEmail: emailAddr,
		CustomerName:  customerName,
		PersonaName:   r.personaName,
		RequestedAt:   time.Now(),
		Status:        "pending",
	}

	r.callbacks[agentID] = cb
	r.persist()

	return cb, nil
}

// RegisterBatch creates a group callback for multiple agents
func (r *Registry) RegisterBatch(agents []AgentInfo, method, phone, emailAddr, customerName string) (*CallbackGroup, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Validate method requirements (same as single)
	if method == "call" || method == "both" {
		if phone == "" {
			return nil, fmt.Errorf("phone number required for call callback")
		}
	}
	if method == "email" || method == "both" {
		if emailAddr == "" {
			return nil, fmt.Errorf("email address required for email callback")
		}
	}

	groupID := fmt.Sprintf("grp-%d", time.Now().UnixNano())
	agentIDs := make([]string, len(agents))

	// Create individual callbacks pointing to group
	for i, agent := range agents {
		agentIDs[i] = agent.ID
		cb := &Callback{
			ID:            fmt.Sprintf("cb-%s-%d", agent.ID, time.Now().UnixNano()),
			AgentID:       agent.ID,
			AgentName:     agent.Name,
			TaskSummary:   agent.TaskSummary,
			ProjectName:   agent.ProjectName,
			Method:        method,
			CustomerPhone: phone,
			CustomerEmail: emailAddr,
			CustomerName:  customerName,
			PersonaName:   r.personaName,
			RequestedAt:   time.Now(),
			Status:        "pending",
			GroupID:       groupID,
		}
		r.callbacks[agent.ID] = cb
	}

	group := &CallbackGroup{
		ID:            groupID,
		AgentIDs:      agentIDs,
		Results:       make(map[string]CompletionInfo),
		Method:        method,
		CustomerPhone: phone,
		CustomerEmail: emailAddr,
		CustomerName:  customerName,
		PersonaName:   r.personaName,
		RequestedAt:   time.Now(),
		Status:        "pending",
	}

	r.groups[groupID] = group
	r.persist()

	return group, nil
}

// OnAgentComplete is called when an agent finishes
func (r *Registry) OnAgentComplete(info CompletionInfo) {
	r.mu.Lock()
	defer r.mu.Unlock()

	cb, ok := r.callbacks[info.AgentID]
	if !ok {
		return // No callback registered
	}

	if cb.GroupID != "" {
		// Part of a group - record result
		group, ok := r.groups[cb.GroupID]
		if !ok {
			return
		}

		group.Results[info.AgentID] = info

		// Check if all agents in group are done
		if len(group.Results) == len(group.AgentIDs) {
			r.executeGroupCallback(group)
		}
	} else {
		// Single callback
		r.executeCallback(cb, info)
	}

	r.persist()
}

func (r *Registry) executeCallback(cb *Callback, info CompletionInfo) {
	cb.CompletedAt = time.Now()

	var execErr error
	switch cb.Method {
	case "call":
		execErr = r.executeCall(cb, info)
	case "email":
		execErr = r.executeEmail(cb, info)
	case "both":
		if err := r.executeCall(cb, info); err != nil {
			execErr = err
		}
		if err := r.executeEmail(cb, info); err != nil {
			if execErr != nil {
				execErr = fmt.Errorf("call: %v; email: %v", execErr, err)
			} else {
				execErr = err
			}
		}
	}

	if execErr != nil {
		cb.Status = "failed"
		cb.Error = execErr.Error()
		log.Printf("Callback failed for agent %s: %v", cb.AgentID, execErr)
	} else {
		cb.Status = "completed"
	}

	// Move to history
	delete(r.callbacks, cb.AgentID)
	r.history = append(r.history, cb)
	if len(r.history) > 100 {
		r.history = r.history[1:]
	}
}

func (r *Registry) executeGroupCallback(group *CallbackGroup) {
	group.CompletedAt = time.Now()

	var execErr error
	switch group.Method {
	case "call":
		execErr = r.executeBatchCall(group)
	case "email":
		execErr = r.executeBatchEmail(group)
	case "both":
		if err := r.executeBatchCall(group); err != nil {
			execErr = err
		}
		if err := r.executeBatchEmail(group); err != nil {
			if execErr != nil {
				execErr = fmt.Errorf("call: %v; email: %v", execErr, err)
			} else {
				execErr = err
			}
		}
	}

	if execErr != nil {
		group.Status = "failed"
		group.Error = execErr.Error()
		log.Printf("Group callback failed: %v", execErr)
	} else {
		group.Status = "completed"
	}

	// Clean up individual callbacks
	for _, agentID := range group.AgentIDs {
		if cb, ok := r.callbacks[agentID]; ok {
			cb.Status = group.Status
			cb.CompletedAt = group.CompletedAt
			r.history = append(r.history, cb)
			delete(r.callbacks, agentID)
		}
	}
	if len(r.history) > 100 {
		r.history = r.history[len(r.history)-100:]
	}

	// Move group to history
	delete(r.groups, group.ID)
	r.groupHistory = append(r.groupHistory, group)
	if len(r.groupHistory) > 50 {
		r.groupHistory = r.groupHistory[1:]
	}
}

func (r *Registry) executeCall(cb *Callback, info CompletionInfo) error {
	if r.vapiClient == nil || !r.vapiClient.IsConfigured() {
		return fmt.Errorf("VAPI client not configured")
	}

	ctx := &vapi.CallbackContext{
		AgentName:   cb.AgentName,
		TaskSummary: cb.TaskSummary,
		Result:      info.Result,
		ProjectName: cb.ProjectName,
	}

	// Truncate phone for logging
	phone := cb.CustomerPhone
	if len(phone) > 6 {
		phone = phone[:3] + "***" + phone[len(phone)-4:]
	}
	log.Printf("Initiating callback call to %s for agent %s", phone, cb.AgentID)

	_, err := r.vapiClient.Call(nil, cb.CustomerPhone, cb.CustomerName, ctx)
	return err
}

func (r *Registry) executeEmail(cb *Callback, info CompletionInfo) error {
	if r.emailClient == nil || !r.emailClient.IsConfigured() {
		return fmt.Errorf("email client not configured")
	}

	var viewURL string
	if r.getServerURL != nil && cb.ProjectName != "" {
		viewURL = r.getServerURL(cb.ProjectName)
	}

	ctx := &email.CallbackContext{
		RecipientName:  cb.CustomerName,
		RecipientEmail: cb.CustomerEmail,
		AgentID:        cb.AgentID,
		AgentName:      cb.AgentName,
		TaskSummary:    cb.TaskSummary,
		ProjectName:    cb.ProjectName,
		Result:         info.Result,
		Error:          info.Error,
		ViewURL:        viewURL,
		Success:        info.Error == "",
	}

	return r.emailClient.SendTaskComplete(ctx)
}

func (r *Registry) executeBatchCall(group *CallbackGroup) error {
	// For batch calls, we make a single call summarizing all results
	// This is a simplification - could be expanded to make individual calls
	return fmt.Errorf("batch calls not yet implemented")
}

func (r *Registry) executeBatchEmail(group *CallbackGroup) error {
	if r.emailClient == nil || !r.emailClient.IsConfigured() {
		return fmt.Errorf("email client not configured")
	}

	var viewURL string
	if r.getServerURL != nil {
		// Use first project name for URL
		for _, info := range group.Results {
			if info.ProjectName != "" {
				viewURL = r.getServerURL(info.ProjectName)
				break
			}
		}
	}

	results := make([]email.AgentResult, 0, len(group.Results))
	for _, info := range group.Results {
		results = append(results, email.AgentResult{
			AgentID:     info.AgentID,
			AgentName:   info.AgentName,
			Result:      info.Result,
			Error:       info.Error,
			Success:     info.Error == "",
		})
	}

	ctx := &email.BatchCallbackContext{
		RecipientName:  group.CustomerName,
		RecipientEmail: group.CustomerEmail,
		Results:        results,
		ViewURL:        viewURL,
	}

	return r.emailClient.SendBatchComplete(ctx)
}

// Get returns a pending callback by agent ID
func (r *Registry) Get(agentID string) *Callback {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.callbacks[agentID]
}

// Cancel removes a pending callback
func (r *Registry) Cancel(agentID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.callbacks[agentID]; ok {
		delete(r.callbacks, agentID)
		r.persist()
		return true
	}
	return false
}

// ListPending returns all pending callbacks
func (r *Registry) ListPending() []*Callback {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*Callback, 0, len(r.callbacks))
	for _, cb := range r.callbacks {
		result = append(result, cb)
	}
	return result
}

// ListHistory returns completed callbacks
func (r *Registry) ListHistory() []*Callback {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*Callback, len(r.history))
	copy(result, r.history)
	return result
}

// CanCall returns true if call callbacks are available
func (r *Registry) CanCall() bool {
	return r.vapiClient != nil && r.vapiClient.IsConfigured()
}

// CanEmail returns true if email callbacks are available
func (r *Registry) CanEmail() bool {
	return r.emailClient != nil && r.emailClient.IsConfigured()
}

func (r *Registry) persist() {
	path := filepath.Join(r.baseDir, "tron.work", "callbacks.json")
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		log.Printf("Failed to create callbacks directory: %v", err)
		return
	}

	data := struct {
		Callbacks    map[string]*Callback      `json:"callbacks"`
		Groups       map[string]*CallbackGroup `json:"groups"`
		History      []*Callback               `json:"history"`
		GroupHistory []*CallbackGroup          `json:"group_history"`
	}{
		Callbacks:    r.callbacks,
		Groups:       r.groups,
		History:      r.history,
		GroupHistory: r.groupHistory,
	}

	content, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		log.Printf("Failed to marshal callbacks: %v", err)
		return
	}

	if err := os.WriteFile(path, content, 0644); err != nil {
		log.Printf("Failed to persist callbacks: %v", err)
	}
}

func (r *Registry) load() {
	path := filepath.Join(r.baseDir, "tron.work", "callbacks.json")
	content, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Printf("Failed to load callbacks: %v", err)
		}
		return
	}

	var data struct {
		Callbacks    map[string]*Callback      `json:"callbacks"`
		Groups       map[string]*CallbackGroup `json:"groups"`
		History      []*Callback               `json:"history"`
		GroupHistory []*CallbackGroup          `json:"group_history"`
	}

	if err := json.Unmarshal(content, &data); err != nil {
		log.Printf("Failed to parse callbacks: %v", err)
		return
	}

	if data.Callbacks != nil {
		r.callbacks = data.Callbacks
	}
	if data.Groups != nil {
		r.groups = data.Groups
	}
	if data.History != nil {
		r.history = data.History
	}
	if data.GroupHistory != nil {
		r.groupHistory = data.GroupHistory
	}
}

func (r *Registry) cleanupOrphaned() {
	if r.agentValidator == nil {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	for agentID, cb := range r.callbacks {
		if !r.agentValidator(agentID) {
			cb.Status = "orphaned"
			r.history = append(r.history, cb)
			delete(r.callbacks, agentID)
		}
	}

	r.persist()
}
