package governance

import (
	"os"
	"path/filepath"
	"strings"
)

// CLevel contains the list of C-level executives who get governance context
var CLevel = []string{"Tony", "Maya", "Alex", "Jordan", "Riley"}

// IsCLevel returns true if the agent is a C-level executive
func IsCLevel(agentName string) bool {
	for _, name := range CLevel {
		if strings.EqualFold(name, agentName) {
			return true
		}
	}
	return false
}

// Load reads the operating framework document
func Load(knowledgeDir string) (string, error) {
	path := filepath.Join(knowledgeDir, "governance", "operating-framework.md")
	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return string(content), nil
}

// GetPromptSection formats governance content for injection into system prompt
// Only injects for C-level executives
func GetPromptSection(content string, agentName string) string {
	if content == "" || !IsCLevel(agentName) {
		return ""
	}
	return "\n\n## Company Operating Framework\n" + content
}
