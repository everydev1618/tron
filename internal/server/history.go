package server

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	// MaxHistoryAge is how old history entries can be before being pruned
	MaxHistoryAge = 30 * 24 * time.Hour
	// historyFileName is the file where history is persisted
	historyFileName = "history.json"
)

// HistoryEntryType represents the type of history event
type HistoryEntryType string

const (
	HistoryProcessStart  HistoryEntryType = "process_start"
	HistoryProcessEnd    HistoryEntryType = "process_end"
	HistorySessionStart  HistoryEntryType = "session_start"
	HistorySessionEnd    HistoryEntryType = "session_end"
	HistoryError         HistoryEntryType = "error"
)

// HistoryEntry represents a single historical event
type HistoryEntry struct {
	ID         string            `json:"id"`
	Type       HistoryEntryType  `json:"type"`
	Timestamp  time.Time         `json:"timestamp"`
	Agent      string            `json:"agent"`
	ProcessID  string            `json:"process_id,omitempty"`
	Task       string            `json:"task,omitempty"`
	Status     string            `json:"status,omitempty"`
	DurationMs int64             `json:"duration_ms,omitempty"`
	Metrics    *HistoryMetrics   `json:"metrics,omitempty"`
	Error      string            `json:"error,omitempty"`
}

// HistoryMetrics contains metrics for a completed process
type HistoryMetrics struct {
	InputTokens   int     `json:"input_tokens,omitempty"`
	OutputTokens  int     `json:"output_tokens,omitempty"`
	TotalTokens   int     `json:"total_tokens,omitempty"`
	LLMCalls      int     `json:"llm_calls,omitempty"`
	ToolCalls     int     `json:"tool_calls,omitempty"`
	EstimatedCost float64 `json:"estimated_cost,omitempty"`
}

// HistorySummary contains aggregate statistics
type HistorySummary struct {
	TotalEntries    int                       `json:"total_entries"`
	TotalProcesses  int                       `json:"total_processes"`
	TotalSessions   int                       `json:"total_sessions"`
	TotalErrors     int                       `json:"total_errors"`
	ByAgent         map[string]int            `json:"by_agent"`
	ByDay           map[string]int            `json:"by_day"`
	ByStatus        map[string]int            `json:"by_status"`
	AvgDurationMs   int64                     `json:"avg_duration_ms"`
	TotalCost       float64                   `json:"total_cost"`
}

// HistoryResponse is the API response for /api/history
type HistoryResponse struct {
	Entries []HistoryEntry `json:"entries"`
	Summary HistorySummary `json:"summary"`
}

// HistoryStore manages historical event data
type HistoryStore struct {
	entries []HistoryEntry
	mu      sync.RWMutex
	baseDir string
}

// NewHistoryStore creates a new history store
func NewHistoryStore(baseDir string) *HistoryStore {
	store := &HistoryStore{
		entries: make([]HistoryEntry, 0),
		baseDir: baseDir,
	}
	store.load()
	return store
}

// Record adds a new history entry
func (h *HistoryStore) Record(entry HistoryEntry) {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Generate ID if not set
	if entry.ID == "" {
		entry.ID = generateHistoryID()
	}

	// Set timestamp if not set
	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now()
	}

	h.entries = append(h.entries, entry)
	h.prune()
	h.save()
}

// Query returns entries within the specified number of days
func (h *HistoryStore) Query(days int) HistoryResponse {
	h.mu.RLock()
	defer h.mu.RUnlock()

	cutoff := time.Now().Add(-time.Duration(days) * 24 * time.Hour)
	filtered := make([]HistoryEntry, 0)

	for _, entry := range h.entries {
		if entry.Timestamp.After(cutoff) {
			filtered = append(filtered, entry)
		}
	}

	// Sort by timestamp descending (newest first)
	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].Timestamp.After(filtered[j].Timestamp)
	})

	return HistoryResponse{
		Entries: filtered,
		Summary: h.buildSummary(filtered),
	}
}

// buildSummary creates aggregate statistics from entries
func (h *HistoryStore) buildSummary(entries []HistoryEntry) HistorySummary {
	summary := HistorySummary{
		TotalEntries: len(entries),
		ByAgent:      make(map[string]int),
		ByDay:        make(map[string]int),
		ByStatus:     make(map[string]int),
	}

	var totalDuration int64
	var durationCount int

	for _, entry := range entries {
		// Count by agent
		if entry.Agent != "" {
			summary.ByAgent[entry.Agent]++
		}

		// Count by day
		day := entry.Timestamp.Format("2006-01-02")
		summary.ByDay[day]++

		// Count by status
		if entry.Status != "" {
			summary.ByStatus[entry.Status]++
		}

		// Count by type
		switch entry.Type {
		case HistoryProcessStart, HistoryProcessEnd:
			summary.TotalProcesses++
		case HistorySessionStart, HistorySessionEnd:
			summary.TotalSessions++
		case HistoryError:
			summary.TotalErrors++
		}

		// Track duration
		if entry.DurationMs > 0 {
			totalDuration += entry.DurationMs
			durationCount++
		}

		// Track cost
		if entry.Metrics != nil {
			summary.TotalCost += entry.Metrics.EstimatedCost
		}
	}

	// Calculate average duration
	if durationCount > 0 {
		summary.AvgDurationMs = totalDuration / int64(durationCount)
	}

	// Dedupe process/session counts (start and end are separate events)
	summary.TotalProcesses = summary.TotalProcesses / 2
	summary.TotalSessions = summary.TotalSessions / 2

	return summary
}

// prune removes entries older than MaxHistoryAge
func (h *HistoryStore) prune() {
	cutoff := time.Now().Add(-MaxHistoryAge)
	filtered := make([]HistoryEntry, 0, len(h.entries))

	for _, entry := range h.entries {
		if entry.Timestamp.After(cutoff) {
			filtered = append(filtered, entry)
		}
	}

	h.entries = filtered
}

// load reads history from disk
func (h *HistoryStore) load() {
	path := h.filePath()
	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			// Log error but continue with empty history
		}
		return
	}

	var entries []HistoryEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return
	}

	h.entries = entries
	h.prune()
}

// save writes history to disk
func (h *HistoryStore) save() {
	path := h.filePath()

	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return
	}

	data, err := json.MarshalIndent(h.entries, "", "  ")
	if err != nil {
		return
	}

	os.WriteFile(path, data, 0644)
}

// filePath returns the path to the history file
func (h *HistoryStore) filePath() string {
	if h.baseDir != "" {
		return filepath.Join(h.baseDir, ".tronvega", historyFileName)
	}

	// Fall back to home directory
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, ".tronvega", historyFileName)
}

// generateHistoryID creates a unique ID for a history entry
func generateHistoryID() string {
	return time.Now().Format("20060102150405.000000")
}

// SpawnPattern represents a common parent→child spawn pattern
type SpawnPattern struct {
	Parent string `json:"parent"`
	Child  string `json:"child"`
	Count  int    `json:"count"`
}

// SpawnPatternSummary contains aggregate spawn statistics
type SpawnPatternSummary struct {
	TotalSpawns    int            `json:"total_spawns"`
	MaxDepth       int            `json:"max_depth"`
	SpawnsByAgent  map[string]int `json:"spawns_by_agent"`  // Who spawns most
	SpawnedByAgent map[string]int `json:"spawned_by_agent"` // Who gets spawned most
	CommonPatterns []SpawnPattern `json:"common_patterns"`  // Parent→Child frequencies
}

// BuildSpawnPatterns analyzes spawn history and returns pattern summary
// Note: This is a simplified implementation that tracks spawn events.
// For full accuracy, spawn events should be recorded in history.
func (h *HistoryStore) BuildSpawnPatterns(days int) SpawnPatternSummary {
	h.mu.RLock()
	defer h.mu.RUnlock()

	cutoff := time.Now().Add(-time.Duration(days) * 24 * time.Hour)
	summary := SpawnPatternSummary{
		SpawnsByAgent:  make(map[string]int),
		SpawnedByAgent: make(map[string]int),
		CommonPatterns: make([]SpawnPattern, 0),
	}

	// Track spawn patterns from process start events
	// We infer spawns from process_start events - the spawner is typically
	// a persona (Tony, Maya, etc.) and the spawned is a team member
	patternCounts := make(map[string]int)

	// Known personas (spawners)
	personas := map[string]bool{
		"Tony": true, "Maya": true, "Alex": true, "Jordan": true, "Riley": true,
	}

	for _, entry := range h.entries {
		if entry.Timestamp.Before(cutoff) {
			continue
		}

		if entry.Type == HistoryProcessStart && entry.Agent != "" {
			// Track who gets spawned
			summary.SpawnedByAgent[entry.Agent]++
			summary.TotalSpawns++

			// If this is a team member being spawned, attribute to their manager
			// This is a heuristic - ideally we'd track parent in history
			if !personas[entry.Agent] {
				// Find which persona likely spawned this agent
				// For now, count all spawned agents
				// In a full implementation, we'd track the parent ID
			}
		}
	}

	// Convert pattern counts to sorted list
	for pattern, count := range patternCounts {
		parts := splitPattern(pattern)
		if len(parts) == 2 {
			summary.CommonPatterns = append(summary.CommonPatterns, SpawnPattern{
				Parent: parts[0],
				Child:  parts[1],
				Count:  count,
			})
		}
	}

	// Sort patterns by count (descending)
	sortPatterns(summary.CommonPatterns)

	// Limit to top 10 patterns
	if len(summary.CommonPatterns) > 10 {
		summary.CommonPatterns = summary.CommonPatterns[:10]
	}

	return summary
}

// splitPattern splits a "parent→child" pattern string
func splitPattern(pattern string) []string {
	return strings.Split(pattern, "→")
}

// sortPatterns sorts patterns by count descending
func sortPatterns(patterns []SpawnPattern) {
	sort.Slice(patterns, func(i, j int) bool {
		return patterns[i].Count > patterns[j].Count
	})
}
