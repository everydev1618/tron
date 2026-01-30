package memory

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const (
	// MaxMemoryAge is how old memory entries can be before being filtered out
	MaxMemoryAge = 7 * 24 * time.Hour
	// memoryFileName is the file where recent memories are stored
	memoryFileName = "memory.md"
)

// Load reads the memory file and returns content filtered to the last 7 days
func Load(baseDir string) (string, error) {
	path := filepath.Join(baseDir, "tron.persona", memoryFileName)
	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}

	return filterRecentEntries(string(content)), nil
}

// Append adds a new memory entry with proper date sections
func Append(baseDir, callerName, summary string) error {
	dir := filepath.Join(baseDir, "tron.persona")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	path := filepath.Join(dir, memoryFileName)
	now := time.Now()
	dateStr := now.Format("2006-01-02")
	timeStr := now.Format("3:04 PM")

	// Read existing content
	content, _ := os.ReadFile(path)
	existingContent := string(content)

	// Check if today's date section exists
	dateHeader := fmt.Sprintf("## %s", dateStr)
	hasDateSection := strings.Contains(existingContent, dateHeader)

	var newContent string
	if existingContent == "" {
		// Create new file with header
		newContent = fmt.Sprintf("# Recent Memory\n\n%s\n### Call with %s at %s\n%s\n",
			dateHeader, callerName, timeStr, summary)
	} else if hasDateSection {
		// Add entry under existing date section
		entry := fmt.Sprintf("### Call with %s at %s\n%s\n", callerName, timeStr, summary)
		// Insert after the date header
		idx := strings.Index(existingContent, dateHeader)
		endOfLine := strings.Index(existingContent[idx:], "\n")
		if endOfLine == -1 {
			newContent = existingContent + "\n" + entry
		} else {
			insertPoint := idx + endOfLine + 1
			newContent = existingContent[:insertPoint] + entry + existingContent[insertPoint:]
		}
	} else {
		// Add new date section at the top (after header)
		entry := fmt.Sprintf("%s\n### Call with %s at %s\n%s\n", dateHeader, callerName, timeStr, summary)
		// Find end of header
		headerEnd := strings.Index(existingContent, "\n\n")
		if headerEnd == -1 {
			newContent = existingContent + "\n\n" + entry
		} else {
			newContent = existingContent[:headerEnd+2] + entry + existingContent[headerEnd+2:]
		}
	}

	return os.WriteFile(path, []byte(newContent), 0644)
}

// filterRecentEntries removes entries older than MaxMemoryAge
func filterRecentEntries(content string) string {
	if content == "" {
		return ""
	}

	// Match date headers like "## 2024-01-30"
	dateRegex := regexp.MustCompile(`^## (\d{4}-\d{2}-\d{2})`)
	cutoff := time.Now().Add(-MaxMemoryAge)

	var result strings.Builder
	var currentDateValid bool
	scanner := bufio.NewScanner(strings.NewReader(content))

	for scanner.Scan() {
		line := scanner.Text()

		if matches := dateRegex.FindStringSubmatch(line); matches != nil {
			entryDate, err := time.Parse("2006-01-02", matches[1])
			if err != nil {
				currentDateValid = false
				continue
			}
			currentDateValid = entryDate.After(cutoff) || entryDate.Equal(cutoff.Truncate(24*time.Hour))
		}

		// Keep header lines that don't have dates
		if strings.HasPrefix(line, "# ") && !strings.HasPrefix(line, "## ") {
			result.WriteString(line)
			result.WriteString("\n")
			continue
		}

		if currentDateValid {
			result.WriteString(line)
			result.WriteString("\n")
		}
	}

	return strings.TrimSpace(result.String())
}

// GetPromptSection formats memory content for injection into system prompt
func GetPromptSection(content string) string {
	if content == "" {
		return ""
	}
	return fmt.Sprintf("\n## Recent Memory (this week)\n%s", content)
}

// SummarizePrompt returns the prompt template for Claude to summarize a conversation
func SummarizePrompt() string {
	return `Summarize this conversation in 2-4 concise bullet points. Focus on:
- Key topics discussed
- Any decisions made
- Follow-up items or commitments

Use first person (I/we). Start each bullet with "- ". Be concise.`
}
