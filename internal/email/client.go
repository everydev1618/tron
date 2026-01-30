package email

import (
	"fmt"
	"net/smtp"
	"strings"
	"time"
)

// Client handles email sending for callback notifications
type Client struct {
	host     string
	port     int
	user     string
	password string
	from     string
}

// NewClient creates a new email client
func NewClient(host string, port int, user, password, from string) *Client {
	return &Client{
		host:     host,
		port:     port,
		user:     user,
		password: password,
		from:     from,
	}
}

// IsConfigured returns true if the client has required settings
func (c *Client) IsConfigured() bool {
	return c.host != "" && c.from != ""
}

// CallbackContext contains data for single callback emails
type CallbackContext struct {
	RecipientName string
	RecipientEmail string
	AgentID       string
	AgentName     string
	TaskSummary   string
	ProjectName   string
	Result        string
	Error         string
	ViewURL       string
	Success       bool
}

// AgentResult contains result for a single agent in batch callbacks
type AgentResult struct {
	AgentID     string
	AgentName   string
	TaskSummary string
	ProjectName string
	Result      string
	Error       string
	Success     bool
}

// BatchCallbackContext contains data for batch callback emails
type BatchCallbackContext struct {
	RecipientName  string
	RecipientEmail string
	Results        []AgentResult
	ViewURL        string
}

// SendTaskComplete sends an email notification for a completed task
func (c *Client) SendTaskComplete(ctx *CallbackContext) error {
	if !c.IsConfigured() {
		return fmt.Errorf("email client not configured")
	}

	subject := c.buildSubject(ctx)
	body := c.buildEmailBody(ctx)

	return c.send(ctx.RecipientEmail, subject, body, "")
}

// SendBatchComplete sends an email notification for multiple completed tasks
func (c *Client) SendBatchComplete(ctx *BatchCallbackContext) error {
	if !c.IsConfigured() {
		return fmt.Errorf("email client not configured")
	}

	subject := c.buildBatchSubject(ctx)
	body := c.buildBatchEmailBody(ctx)

	return c.send(ctx.RecipientEmail, subject, body, "")
}

func (c *Client) buildSubject(ctx *CallbackContext) string {
	var subject string
	if ctx.Success {
		subject = fmt.Sprintf("%s completed: %s", ctx.AgentName, ctx.TaskSummary)
	} else {
		subject = fmt.Sprintf("%s failed: %s", ctx.AgentName, ctx.TaskSummary)
	}

	// Truncate if too long
	if len(subject) > 50 {
		subject = subject[:47] + "..."
	}
	return subject
}

func (c *Client) buildBatchSubject(ctx *BatchCallbackContext) string {
	successCount := 0
	failCount := 0
	for _, r := range ctx.Results {
		if r.Success {
			successCount++
		} else {
			failCount++
		}
	}

	if failCount == 0 {
		return fmt.Sprintf("Your tasks are complete (%d finished)", successCount)
	}
	return fmt.Sprintf("Your tasks are complete (%d finished, %d failed)", successCount, failCount)
}

func (c *Client) buildEmailBody(ctx *CallbackContext) string {
	var sb strings.Builder

	// Greeting
	if ctx.RecipientName != "" {
		sb.WriteString(fmt.Sprintf("Hey %s,\n\n", ctx.RecipientName))
	} else {
		sb.WriteString("Hey,\n\n")
	}

	// Status
	if ctx.Success {
		sb.WriteString(fmt.Sprintf("%s has finished working on your task.\n\n", ctx.AgentName))
	} else {
		sb.WriteString(fmt.Sprintf("%s encountered an issue with your task.\n\n", ctx.AgentName))
	}

	// Task details
	sb.WriteString(fmt.Sprintf("**Task:** %s\n", ctx.TaskSummary))
	if ctx.ProjectName != "" {
		sb.WriteString(fmt.Sprintf("**Project:** %s\n", ctx.ProjectName))
	}

	// Result or error
	if ctx.Success && ctx.Result != "" {
		sb.WriteString(fmt.Sprintf("\n**Result:**\n%s\n", ctx.Result))
	} else if !ctx.Success && ctx.Error != "" {
		sb.WriteString(fmt.Sprintf("\n**Error:**\n%s\n", ctx.Error))
	}

	// View URL
	if ctx.ViewURL != "" {
		sb.WriteString(fmt.Sprintf("\nView the project: %s\n", ctx.ViewURL))
	}

	// Footer
	sb.WriteString(fmt.Sprintf("\n---\nAgent ID: %s\nThis is an automated notification from Tony.\n", ctx.AgentID))

	return sb.String()
}

func (c *Client) buildBatchEmailBody(ctx *BatchCallbackContext) string {
	var sb strings.Builder

	// Greeting
	if ctx.RecipientName != "" {
		sb.WriteString(fmt.Sprintf("Hey %s,\n\n", ctx.RecipientName))
	} else {
		sb.WriteString("Hey,\n\n")
	}

	sb.WriteString("Your tasks have been completed. Here's a summary:\n\n")

	// List each result
	for _, r := range ctx.Results {
		if r.Success {
			sb.WriteString(fmt.Sprintf("✓ **%s** - %s\n", r.AgentName, r.TaskSummary))
			if r.Result != "" {
				sb.WriteString(fmt.Sprintf("  Result: %s\n", truncate(r.Result, 100)))
			}
		} else {
			sb.WriteString(fmt.Sprintf("✗ **%s** - %s\n", r.AgentName, r.TaskSummary))
			if r.Error != "" {
				sb.WriteString(fmt.Sprintf("  Error: %s\n", truncate(r.Error, 100)))
			}
		}
		sb.WriteString("\n")
	}

	// View URL
	if ctx.ViewURL != "" {
		sb.WriteString(fmt.Sprintf("View the project: %s\n", ctx.ViewURL))
	}

	// Footer
	sb.WriteString("\n---\nThis is an automated notification from Tony.\n")

	return sb.String()
}

func (c *Client) send(to, subject, body, fromOverride string) error {
	from := c.from
	if fromOverride != "" {
		from = fromOverride
	}

	// Build email message
	msg := fmt.Sprintf("From: %s\r\n"+
		"To: %s\r\n"+
		"Subject: %s\r\n"+
		"Date: %s\r\n"+
		"MIME-Version: 1.0\r\n"+
		"Content-Type: text/plain; charset=utf-8\r\n"+
		"\r\n"+
		"%s",
		from, to, subject, time.Now().Format(time.RFC1123Z), body)

	addr := fmt.Sprintf("%s:%d", c.host, c.port)

	// Use auth if credentials provided
	var auth smtp.Auth
	if c.user != "" && c.password != "" {
		auth = smtp.PlainAuth("", c.user, c.password, c.host)
	}

	return smtp.SendMail(addr, auth, from, []string{to}, []byte(msg))
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
