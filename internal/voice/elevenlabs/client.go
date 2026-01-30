package elevenlabs

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	baseWSURL          = "wss://api.elevenlabs.io/v1/convai/conversation"
	baseHTTPURL        = "https://api.elevenlabs.io/v1"
	defaultSampleRate  = 16000
	defaultFormat      = "pcm_16000"
)

// Client handles ElevenLabs conversational AI
type Client struct {
	apiKey     string
	agentID    string
	httpClient *http.Client
}

// NewClient creates a new ElevenLabs client
func NewClient(apiKey, agentID string) *Client {
	return &Client{
		apiKey:  apiKey,
		agentID: agentID,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// IsConfigured returns true if the client has required credentials
func (c *Client) IsConfigured() bool {
	return c.apiKey != "" && c.agentID != ""
}

// TranscriptEvent represents a transcript from speech recognition
type TranscriptEvent struct {
	Role      string `json:"role"` // "user" or "agent"
	Text      string `json:"text"`
	IsFinal   bool   `json:"is_final"`
	Timestamp int64  `json:"timestamp"`
}

// AgentResponse represents an agent's text response
type AgentResponse struct {
	Text      string `json:"text"`
	Timestamp int64  `json:"timestamp"`
}

// Session represents an active ElevenLabs conversation
type Session struct {
	conn           *websocket.Conn
	conversationID string
	mu             sync.Mutex

	// Channels for events
	transcripts    chan TranscriptEvent
	audioOut       chan []byte
	agentResponses chan AgentResponse
	done           chan struct{}
	closeOnce      sync.Once
}

// GetSignedURL gets a signed WebSocket URL for connecting
func (c *Client) GetSignedURL(ctx context.Context) (string, error) {
	url := fmt.Sprintf("%s/convai/conversation/get_signed_url?agent_id=%s", baseHTTPURL, c.agentID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("xi-api-key", c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to get signed URL: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("ElevenLabs API error (status %d): %s", resp.StatusCode, string(body))
	}

	var result struct {
		SignedURL string `json:"signed_url"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	return result.SignedURL, nil
}

// Connect establishes a WebSocket connection to ElevenLabs
func (c *Client) Connect(ctx context.Context) (*Session, error) {
	if !c.IsConfigured() {
		return nil, fmt.Errorf("ElevenLabs client not configured")
	}

	// Get signed URL
	signedURL, err := c.GetSignedURL(ctx)
	if err != nil {
		return nil, err
	}

	// Connect WebSocket
	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	conn, _, err := dialer.DialContext(ctx, signedURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to connect WebSocket: %w", err)
	}

	session := &Session{
		conn:           conn,
		transcripts:    make(chan TranscriptEvent, 100),
		audioOut:       make(chan []byte, 100),
		agentResponses: make(chan AgentResponse, 100),
		done:           make(chan struct{}),
	}

	// Start read loop
	go session.readLoop()

	return session, nil
}

// SendAudio sends audio data to ElevenLabs
func (s *Session) SendAudio(audio []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	msg := audioInputMessage{
		Type: "user_audio_chunk",
		Data: audio,
	}

	return s.conn.WriteJSON(msg)
}

// SendText sends text input (for text-based mode)
func (s *Session) SendText(text string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	msg := textInputMessage{
		Type: "user_text",
		Text: text,
	}

	return s.conn.WriteJSON(msg)
}

// Close closes the session
func (s *Session) Close() error {
	s.closeOnce.Do(func() {
		close(s.done)
		s.conn.Close()
	})
	return nil
}

// Transcripts returns the transcript event channel
func (s *Session) Transcripts() <-chan TranscriptEvent {
	return s.transcripts
}

// Audio returns the audio output channel
func (s *Session) Audio() <-chan []byte {
	return s.audioOut
}

// AgentResponses returns the agent response channel
func (s *Session) AgentResponses() <-chan AgentResponse {
	return s.agentResponses
}

// Done returns a channel that closes when the session ends
func (s *Session) Done() <-chan struct{} {
	return s.done
}

// ConversationID returns the conversation ID
func (s *Session) ConversationID() string {
	return s.conversationID
}

// Message types
type baseMessage struct {
	Type string `json:"type"`
}

type conversationInitMessage struct {
	Type           string `json:"type"`
	ConversationID string `json:"conversation_id"`
}

type userTranscriptMessage struct {
	Type       string `json:"type"`
	Transcript string `json:"user_transcription_event>user_transcript"`
	IsFinal    bool   `json:"is_final"`
}

type agentResponseMessage struct {
	Type     string `json:"type"`
	Response string `json:"agent_response"`
}

type audioMessage struct {
	Type  string `json:"type"`
	Audio []byte `json:"audio"`
}

type audioInputMessage struct {
	Type string `json:"type"`
	Data []byte `json:"data"`
}

type textInputMessage struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func (s *Session) readLoop() {
	defer func() {
		s.closeOnce.Do(func() {
			close(s.done)
		})
		close(s.transcripts)
		close(s.audioOut)
		close(s.agentResponses)
	}()

	for {
		select {
		case <-s.done:
			return
		default:
		}

		_, data, err := s.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				log.Printf("WebSocket error: %v", err)
			}
			return
		}

		s.handleMessage(data)
	}
}

func (s *Session) handleMessage(data []byte) {
	var base baseMessage
	if err := json.Unmarshal(data, &base); err != nil {
		log.Printf("Failed to parse message: %v", err)
		return
	}

	switch base.Type {
	case "conversation_initiation_metadata":
		var msg conversationInitMessage
		if err := json.Unmarshal(data, &msg); err == nil {
			s.conversationID = msg.ConversationID
		}

	case "user_transcript":
		var msg struct {
			Type                    string `json:"type"`
			UserTranscriptionEvent struct {
				UserTranscript string `json:"user_transcript"`
				IsFinal        bool   `json:"is_final"`
			} `json:"user_transcription_event"`
		}
		if err := json.Unmarshal(data, &msg); err == nil {
			select {
			case s.transcripts <- TranscriptEvent{
				Role:      "user",
				Text:      msg.UserTranscriptionEvent.UserTranscript,
				IsFinal:   msg.UserTranscriptionEvent.IsFinal,
				Timestamp: time.Now().UnixMilli(),
			}:
			default:
			}
		}

	case "agent_response":
		var msg struct {
			Type          string `json:"type"`
			AgentResponse string `json:"agent_response"`
		}
		if err := json.Unmarshal(data, &msg); err == nil {
			select {
			case s.agentResponses <- AgentResponse{
				Text:      msg.AgentResponse,
				Timestamp: time.Now().UnixMilli(),
			}:
			default:
			}
			// Also send as transcript
			select {
			case s.transcripts <- TranscriptEvent{
				Role:      "agent",
				Text:      msg.AgentResponse,
				IsFinal:   true,
				Timestamp: time.Now().UnixMilli(),
			}:
			default:
			}
		}

	case "audio":
		var msg struct {
			Type  string `json:"type"`
			Audio []byte `json:"audio"`
		}
		if err := json.Unmarshal(data, &msg); err == nil {
			select {
			case s.audioOut <- msg.Audio:
			default:
			}
		}

	case "ping":
		// Respond with pong
		s.mu.Lock()
		s.conn.WriteJSON(map[string]string{"type": "pong"})
		s.mu.Unlock()
	}
}
