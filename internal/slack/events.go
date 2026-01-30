package slack

import "strings"

// EventPayload is the top-level webhook structure from Slack
type EventPayload struct {
	Type      string      `json:"type"`       // "url_verification" or "event_callback"
	Challenge string      `json:"challenge"`  // For URL verification
	Event     *SlackEvent `json:"event"`      // Actual event
	EventID   string      `json:"event_id"`   // For deduplication
	EventTime int64       `json:"event_time"` // Unix timestamp
}

// SlackEvent represents an individual Slack event
type SlackEvent struct {
	Type    string `json:"type"`    // "message", "app_mention", etc.
	Channel string `json:"channel"` // Channel ID
	User    string `json:"user"`    // User ID who triggered event
	Text    string `json:"text"`    // Message text
	TS      string `json:"ts"`      // Timestamp (unique ID)
	BotID   string `json:"bot_id"`  // Bot ID if from a bot
	Subtype string `json:"subtype"` // Message subtype
}

// IsFromBot returns true if the event was sent by a bot
func (e *SlackEvent) IsFromBot() bool {
	return e.BotID != "" || e.Subtype == "bot_message"
}

// IsDirectMessage returns true if this is a DM channel
func (e *SlackEvent) IsDirectMessage() bool {
	// DM channel IDs start with "D"
	return strings.HasPrefix(e.Channel, "D")
}

// IsAppMention returns true if this is an app mention event
func (e *SlackEvent) IsAppMention() bool {
	return e.Type == "app_mention"
}
