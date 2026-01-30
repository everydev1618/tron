package vapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	baseURL    = "https://api.vapi.ai"
	apiTimeout = 30 * time.Second
)

// Client handles outbound VAPI calls
type Client struct {
	apiKey      string
	phoneID     string
	assistantID string
	httpClient  *http.Client
}

// NewClient creates a new VAPI client
func NewClient(apiKey, phoneID, assistantID string) *Client {
	return &Client{
		apiKey:      apiKey,
		phoneID:     phoneID,
		assistantID: assistantID,
		httpClient: &http.Client{
			Timeout: apiTimeout,
		},
	}
}

// IsConfigured returns true if the client has required credentials
func (c *Client) IsConfigured() bool {
	return c.apiKey != "" && c.phoneID != "" && c.assistantID != ""
}

// CallbackContext provides context for callback calls
type CallbackContext struct {
	AgentName   string
	TaskSummary string
	Result      string
	ProjectName string
}

// CallRequest is the request body for initiating a call
type CallRequest struct {
	PhoneNumberID      string              `json:"phoneNumberId"`
	AssistantID        string              `json:"assistantId,omitempty"`
	Customer           Customer            `json:"customer"`
	AssistantOverrides *AssistantOverrides `json:"assistantOverrides,omitempty"`
}

// Customer is the call recipient
type Customer struct {
	Number string `json:"number"`
	Name   string `json:"name,omitempty"`
}

// AssistantOverrides customizes the assistant for a specific call
type AssistantOverrides struct {
	VariableValues map[string]string `json:"variableValues,omitempty"`
	FirstMessage   string            `json:"firstMessage,omitempty"`
}

// CallResponse is the response from initiating a call
type CallResponse struct {
	ID        string `json:"id"`
	Status    string `json:"status"`
	CreatedAt string `json:"createdAt"`
}

// Call initiates an outbound phone call
func (c *Client) Call(ctx context.Context, customerPhone, customerName string, callbackCtx *CallbackContext) (*CallResponse, error) {
	if !c.IsConfigured() {
		return nil, fmt.Errorf("VAPI client not configured")
	}

	req := CallRequest{
		PhoneNumberID: c.phoneID,
		AssistantID:   c.assistantID,
		Customer: Customer{
			Number: customerPhone,
			Name:   customerName,
		},
	}

	// Add context variables if provided
	if callbackCtx != nil {
		req.AssistantOverrides = &AssistantOverrides{
			VariableValues: map[string]string{
				"agentName":   callbackCtx.AgentName,
				"taskSummary": summarize(callbackCtx.TaskSummary, 100),
				"result":      summarize(callbackCtx.Result, 200),
				"projectName": callbackCtx.ProjectName,
			},
			FirstMessage: buildFirstMessage(callbackCtx),
		}
	}

	return c.createCall(ctx, req)
}

func (c *Client) createCall(ctx context.Context, callReq CallRequest) (*CallResponse, error) {
	body, err := json.Marshal(callReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/call", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("VAPI API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var callResp CallResponse
	if err := json.Unmarshal(respBody, &callResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &callResp, nil
}

func buildFirstMessage(ctx *CallbackContext) string {
	if ctx == nil {
		return ""
	}
	return fmt.Sprintf("Hey, this is Tony. I'm calling to let you know that %s has finished working on %s.",
		ctx.AgentName, summarize(ctx.TaskSummary, 50))
}

func summarize(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
