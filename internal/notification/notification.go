package notification

import "context"

// ChannelType identifies the source channel for a request
type ChannelType string

const (
	ChannelSlack ChannelType = "slack"
	ChannelVoice ChannelType = "voice"
	ChannelAPI   ChannelType = "api"
)

// ChannelContext holds information about the channel that initiated a request
type ChannelContext struct {
	Type      ChannelType
	ChannelID string // Slack channel ID
	UserID    string // Slack user ID or phone number
	UserName  string // Display name for messages
	Email     string // Email for voice callbacks (optional)
}

type contextKey string

const channelKey contextKey = "notification.channel"

// WithChannel adds channel context to a context
func WithChannel(ctx context.Context, ch ChannelContext) context.Context {
	return context.WithValue(ctx, channelKey, ch)
}

// ChannelFromContext extracts channel context from a context
func ChannelFromContext(ctx context.Context) (ChannelContext, bool) {
	ch, ok := ctx.Value(channelKey).(ChannelContext)
	return ch, ok
}
