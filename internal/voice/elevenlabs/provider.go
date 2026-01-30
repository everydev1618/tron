package elevenlabs

// Provider implements the voice provider interface for ElevenLabs
type Provider struct {
	client *Client
}

// NewProvider creates a new ElevenLabs provider
func NewProvider(apiKey, agentID string) *Provider {
	return &Provider{
		client: NewClient(apiKey, agentID),
	}
}

// Name returns the provider name
func (p *Provider) Name() string {
	return "elevenlabs"
}

// IsConfigured returns true if credentials are set
func (p *Provider) IsConfigured() bool {
	return p.client.IsConfigured()
}

// SupportsInbound returns true (WebSocket voice sessions)
func (p *Provider) SupportsInbound() bool {
	return true
}

// SupportsOutbound returns false (ElevenLabs doesn't make outbound calls)
func (p *Provider) SupportsOutbound() bool {
	return false
}

// Client returns the underlying ElevenLabs client
func (p *Provider) Client() *Client {
	return p.client
}
