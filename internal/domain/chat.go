package domain

import (
	"context"
	"encoding/json"
)

type ProviderName string

const (
	ProviderAnthropic ProviderName = "anthropic"
	ProviderOpenAI    ProviderName = "openai_compatible"
)

type MessageRole string

const (
	RoleSystem    MessageRole = "system"
	RoleUser      MessageRole = "user"
	RoleAssistant MessageRole = "assistant"
	RoleTool      MessageRole = "tool"
)

type Message struct {
	Role       MessageRole     `json:"role"`
	Content    string          `json:"content,omitempty"`
	ToolCallID string          `json:"tool_call_id,omitempty"`
	ToolName   string          `json:"tool_name,omitempty"`
	ToolInput  json.RawMessage `json:"tool_input,omitempty"`
}

type ToolDefinition struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Schema      json.RawMessage `json:"schema"`
}

type TurnRequest struct {
	SessionID   string
	Profile     ProviderProfile
	Model       string
	System      string
	Messages    []Message
	Tools       []ToolDefinition
	Temperature float64
}

type ToolCall struct {
	ID    string          `json:"id"`
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
}

type StreamEventType string

const (
	EventTextDelta StreamEventType = "text_delta"
	EventToolCall  StreamEventType = "tool_call"
	EventDone      StreamEventType = "done"
)

type StreamEvent struct {
	Type  StreamEventType
	Text  string
	Call  *ToolCall
	Usage *Usage
}

type Usage struct {
	InputTokens  int `json:"input_tokens,omitempty"`
	OutputTokens int `json:"output_tokens,omitempty"`
}

type Provider interface {
	StreamTurn(context.Context, TurnRequest) (<-chan StreamEvent, <-chan error)
	ListModels(context.Context, ProviderProfile) ([]Model, error)
	ValidateProfile(context.Context, ProviderProfile) error
}

type Model struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name,omitempty"`
}
