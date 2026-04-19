package agent

import (
	"encoding/json"
	"time"
)

type Session struct {
	ID           string          `json:"id"`
	UserID       string          `json:"user_id"`
	Title        string          `json:"title"`
	SystemPrompt *string         `json:"system_prompt,omitempty"`
	Model        *string         `json:"model,omitempty"`
	Metadata     json.RawMessage `json:"metadata"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
}

type Message struct {
	ID           string          `json:"id"`
	SessionID    string          `json:"session_id"`
	Role         string          `json:"role"`
	Content      json.RawMessage `json:"content"`
	StopReason   *string         `json:"stop_reason,omitempty"`
	InputTokens  *int            `json:"input_tokens,omitempty"`
	OutputTokens *int            `json:"output_tokens,omitempty"`
	CreatedAt    time.Time       `json:"created_at"`
}

// Event is what we stream to clients during an agent run.
type Event struct {
	Type    string          `json:"type"`              // text | tool_use | tool_result | usage | done | error
	Text    string          `json:"text,omitempty"`
	Tool    string          `json:"tool,omitempty"`
	ToolID  string          `json:"tool_id,omitempty"`
	Input   json.RawMessage `json:"input,omitempty"`
	Output  json.RawMessage `json:"output,omitempty"`
	IsError bool            `json:"is_error,omitempty"`
	Usage   *Usage          `json:"usage,omitempty"`
	Error   string          `json:"error,omitempty"`
}

type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}
