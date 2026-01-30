package memory

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const (
	directivesFileName = "directives.md"
	peopleDir          = "people"
	maxDirectivesSize  = 10 * 1024 // 10KB warning threshold
)

// LoadDirectives reads the permanent directives file
func LoadDirectives(baseDir string) (string, error) {
	path := filepath.Join(baseDir, "tron.persona", directivesFileName)
	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}

	if len(content) > maxDirectivesSize {
		fmt.Printf("Warning: directives.md is larger than %d bytes\n", maxDirectivesSize)
	}

	return string(content), nil
}

// SaveDirective adds a new directive to the directives file
func SaveDirective(baseDir, directive, category string) error {
	dir := filepath.Join(baseDir, "tron.persona")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	path := filepath.Join(dir, directivesFileName)
	dateStr := time.Now().Format("2006-01-02")

	// Read existing content
	content, _ := os.ReadFile(path)
	existingContent := string(content)

	entry := fmt.Sprintf("- %s [%s] (%s)\n", directive, category, dateStr)

	var newContent string
	if existingContent == "" {
		newContent = fmt.Sprintf("# Permanent Directives\n\nThese are things Tony should always do.\n\n%s", entry)
	} else {
		newContent = existingContent + entry
	}

	return os.WriteFile(path, []byte(newContent), 0644)
}

// LoadPersonMemory reads the memory file for a specific person
func LoadPersonMemory(baseDir, personName string) (string, error) {
	slug := slugifyName(personName)
	path := filepath.Join(baseDir, "tron.persona", peopleDir, slug+".md")

	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}

	return string(content), nil
}

// SavePersonMemory adds a memory entry for a specific person
func SavePersonMemory(baseDir, personName, memory, category string) error {
	dir := filepath.Join(baseDir, "tron.persona", peopleDir)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	slug := slugifyName(personName)
	path := filepath.Join(dir, slug+".md")
	dateStr := time.Now().Format("2006-01-02")

	// Read existing content
	content, _ := os.ReadFile(path)
	existingContent := string(content)

	entry := fmt.Sprintf("- %s [%s] (%s)\n", memory, category, dateStr)

	var newContent string
	if existingContent == "" {
		newContent = fmt.Sprintf("# %s\n\nPermanent memories about %s.\n\n%s", personName, personName, entry)
	} else {
		newContent = existingContent + entry
	}

	return os.WriteFile(path, []byte(newContent), 0644)
}

// ListPeopleMemories returns a list of all people with saved memories
func ListPeopleMemories(baseDir string) ([]string, error) {
	dir := filepath.Join(baseDir, "tron.persona", peopleDir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var people []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".md") {
			// Convert slug back to name (rough approximation)
			name := strings.TrimSuffix(entry.Name(), ".md")
			name = strings.ReplaceAll(name, "-", " ")
			name = strings.Title(name)
			people = append(people, name)
		}
	}

	return people, nil
}

// GetDirectivesPromptSection formats directives for injection into system prompt
func GetDirectivesPromptSection(content string) string {
	if content == "" {
		return ""
	}
	return fmt.Sprintf("\n## Permanent Directives\n%s", content)
}

// GetPersonPromptSection formats person memory for injection into system prompt
func GetPersonPromptSection(personName, content string) string {
	if content == "" {
		return ""
	}
	return fmt.Sprintf("\n## Permanent Memory: %s\n%s", personName, content)
}

// slugifyName converts a name to a filesystem-safe slug
func slugifyName(name string) string {
	// Convert to lowercase
	slug := strings.ToLower(name)
	// Replace spaces with hyphens
	slug = strings.ReplaceAll(slug, " ", "-")
	// Remove non-alphanumeric characters except hyphens
	reg := regexp.MustCompile(`[^a-z0-9-]`)
	slug = reg.ReplaceAllString(slug, "")
	// Remove multiple consecutive hyphens
	reg = regexp.MustCompile(`-+`)
	slug = reg.ReplaceAllString(slug, "-")
	// Trim hyphens from ends
	slug = strings.Trim(slug, "-")
	return slug
}
