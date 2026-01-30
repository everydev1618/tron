package slack

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const slackAPIBase = "https://slack.com/api"

// Client handles Slack Web API interactions
type Client struct {
	botToken   string
	httpClient *http.Client
}

// NewClient creates a new Slack client
func NewClient(botToken string) *Client {
	return &Client{
		botToken: botToken,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// IsConfigured returns true if the client has a bot token
func (c *Client) IsConfigured() bool {
	return c.botToken != ""
}

// User represents a Slack user
type User struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	RealName string `json:"real_name"`
	Email    string `json:"email"`
}

// SendMessage posts a message to a Slack channel
func (c *Client) SendMessage(channel, text string) error {
	if !c.IsConfigured() {
		return fmt.Errorf("Slack client not configured")
	}

	payload := map[string]string{
		"channel": channel,
		"text":    text,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, slackAPIBase+"/chat.postMessage", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.botToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send message: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	var result struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	if !result.OK {
		return fmt.Errorf("Slack API error: %s", result.Error)
	}

	return nil
}

// GetUserInfo retrieves information about a Slack user
func (c *Client) GetUserInfo(userID string) (*User, error) {
	if !c.IsConfigured() {
		return nil, fmt.Errorf("Slack client not configured")
	}

	req, err := http.NewRequest(http.MethodGet, slackAPIBase+"/users.info?user="+userID, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.botToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get user info: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var result struct {
		OK    bool `json:"ok"`
		User  struct {
			ID      string `json:"id"`
			Name    string `json:"name"`
			Profile struct {
				RealName string `json:"real_name"`
				Email    string `json:"email"`
			} `json:"profile"`
		} `json:"user"`
		Error string `json:"error"`
	}

	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if !result.OK {
		return nil, fmt.Errorf("Slack API error: %s", result.Error)
	}

	return &User{
		ID:       result.User.ID,
		Name:     result.User.Name,
		RealName: result.User.Profile.RealName,
		Email:    result.User.Profile.Email,
	}, nil
}

// GetChannelName retrieves the name of a Slack channel
func (c *Client) GetChannelName(channelID string) (string, error) {
	if !c.IsConfigured() {
		return "", fmt.Errorf("Slack client not configured")
	}

	req, err := http.NewRequest(http.MethodGet, slackAPIBase+"/conversations.info?channel="+channelID, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.botToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to get channel info: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	var result struct {
		OK      bool `json:"ok"`
		Channel struct {
			Name string `json:"name"`
		} `json:"channel"`
		Error string `json:"error"`
	}

	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	if !result.OK {
		return "", fmt.Errorf("Slack API error: %s", result.Error)
	}

	return result.Channel.Name, nil
}
