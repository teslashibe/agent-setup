package agent

import "time"

type Session struct {
	ID                 string    `json:"id"`
	UserID             string    `json:"user_id"`
	Title              string    `json:"title"`
	AnthropicSessionID string    `json:"anthropic_session_id,omitempty"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

// Message is a reconstructed conversation turn for history replay.
type Message struct {
	Role    string  `json:"role"` // "user" | "assistant"
	Content []Block `json:"content"`
}

type Block struct {
	Type   string `json:"type"`
	Text   string `json:"text,omitempty"`
	Name   string `json:"name,omitempty"`
	ID     string `json:"id,omitempty"`
	ToolID string `json:"tool_use_id,omitempty"`
}

// Event is streamed to clients over SSE during an agent run.
type Event struct {
	Type    string `json:"type"`
	Text    string `json:"text,omitempty"`
	Tool    string `json:"tool,omitempty"`
	ToolID  string `json:"tool_id,omitempty"`
	IsError bool   `json:"is_error,omitempty"`
	Error   string `json:"error,omitempty"`
}
