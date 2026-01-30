package voice

import (
	"context"
	"time"
)

// Provider is the interface for voice providers
type Provider interface {
	// Name returns the provider name
	Name() string

	// IsConfigured returns true if the provider has required credentials
	IsConfigured() bool

	// SupportsInbound returns true if the provider can receive voice
	SupportsInbound() bool

	// SupportsOutbound returns true if the provider can make outbound calls
	SupportsOutbound() bool
}

// OutboundProvider is for providers that can make outbound calls
type OutboundProvider interface {
	Provider

	// Call initiates an outbound call
	Call(ctx context.Context, req OutboundRequest) (*CallResponse, error)
}

// OutboundRequest contains call details
type OutboundRequest struct {
	Phone        string
	Name         string
	FirstMessage string
	Context      map[string]string
}

// CallResponse contains call result
type CallResponse struct {
	ID        string
	Status    string
	CreatedAt time.Time
}

// ConversationHandler handles LLM integration for voice conversations
type ConversationHandler interface {
	// HandleMessage processes a message and returns a response
	HandleMessage(ctx context.Context, conversationID, userID, message string) (string, error)

	// HandleMessageStream processes a message and streams the response
	HandleMessageStream(ctx context.Context, conversationID, userID, message string) (<-chan string, error)
}
