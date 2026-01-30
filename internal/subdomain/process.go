package subdomain

import (
	"context"
	"fmt"
	"os/exec"
	"sync"
	"time"
)

// ProcessManager manages server processes for projects.
type ProcessManager struct {
	mu        sync.RWMutex
	registry  *Registry
	processes map[string]*ServerProcess
}

// ServerProcess represents a running server process.
type ServerProcess struct {
	ProjectName string
	Subdomain   string
	Port        int
	URL         string
	Command     string
	WorkDir     string
	Status      string
	StartedAt   time.Time
	cmd         *exec.Cmd
	cancel      context.CancelFunc
}

// NewProcessManager creates a new process manager.
func NewProcessManager(registry *Registry) *ProcessManager {
	return &ProcessManager{
		registry:  registry,
		processes: make(map[string]*ServerProcess),
	}
}

// StartServer starts a server process for a project.
func (pm *ProcessManager) StartServer(ctx context.Context, projectName, command, workDir string, env []string) (*ServerProcess, error) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	// Check if already running
	if proc, exists := pm.processes[projectName]; exists {
		if proc.Status == "running" {
			return proc, nil
		}
	}

	// Allocate subdomain and port
	alloc, err := pm.registry.Allocate(projectName)
	if err != nil {
		return nil, fmt.Errorf("failed to allocate subdomain: %w", err)
	}

	// Create process context
	procCtx, cancel := context.WithCancel(ctx)

	// Prepare command
	cmd := exec.CommandContext(procCtx, "sh", "-c", command)
	cmd.Dir = workDir

	// Set environment with PORT
	cmdEnv := append(env, fmt.Sprintf("PORT=%d", alloc.Port))
	cmd.Env = cmdEnv

	// Start the process
	if err := cmd.Start(); err != nil {
		cancel()
		pm.registry.Release(projectName)
		return nil, fmt.Errorf("failed to start server: %w", err)
	}

	proc := &ServerProcess{
		ProjectName: projectName,
		Subdomain:   alloc.Subdomain,
		Port:        alloc.Port,
		URL:         alloc.URL,
		Command:     command,
		WorkDir:     workDir,
		Status:      "running",
		StartedAt:   time.Now(),
		cmd:         cmd,
		cancel:      cancel,
	}

	pm.processes[projectName] = proc

	// Monitor process in background
	go pm.monitorProcess(proc)

	return proc, nil
}

// StopServer stops a server process.
func (pm *ProcessManager) StopServer(projectName string) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	proc, exists := pm.processes[projectName]
	if !exists {
		return fmt.Errorf("server not found: %s", projectName)
	}

	proc.cancel()
	proc.Status = "stopped"

	pm.registry.Release(projectName)
	delete(pm.processes, projectName)

	return nil
}

// GetServer returns the server process for a project.
func (pm *ProcessManager) GetServer(projectName string) *ServerProcess {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return pm.processes[projectName]
}

// ListServers returns all running servers.
func (pm *ProcessManager) ListServers() []*ServerProcess {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	servers := make([]*ServerProcess, 0, len(pm.processes))
	for _, proc := range pm.processes {
		servers = append(servers, proc)
	}
	return servers
}

// monitorProcess watches a process and updates status when it exits.
func (pm *ProcessManager) monitorProcess(proc *ServerProcess) {
	if proc.cmd == nil {
		return
	}

	err := proc.cmd.Wait()

	pm.mu.Lock()
	defer pm.mu.Unlock()

	if err != nil {
		proc.Status = "failed"
	} else {
		proc.Status = "stopped"
	}

	pm.registry.Release(proc.ProjectName)
	delete(pm.processes, proc.ProjectName)
}

// Shutdown stops all running servers.
func (pm *ProcessManager) Shutdown() {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	for _, proc := range pm.processes {
		proc.cancel()
		pm.registry.Release(proc.ProjectName)
	}

	pm.processes = make(map[string]*ServerProcess)
}
