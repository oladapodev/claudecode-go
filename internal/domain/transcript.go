package domain

import "time"

type TranscriptEvent struct {
	Type               string         `json:"type"`
	Timestamp          time.Time      `json:"timestamp"`
	SessionID          string         `json:"session_id,omitempty"`
	Title              string         `json:"title,omitempty"`
	CWD                string         `json:"cwd,omitempty"`
	Provider           ProviderName   `json:"provider,omitempty"`
	Model              string         `json:"model,omitempty"`
	Message            *Message       `json:"message,omitempty"`
	ToolCall           *ToolCall      `json:"tool_call,omitempty"`
	ToolResult         *Message       `json:"tool_result,omitempty"`
	PermissionDecision *PermissionLog `json:"permission_decision,omitempty"`
	Usage              *Usage         `json:"usage,omitempty"`
	Error              string         `json:"error,omitempty"`
}

type PermissionLog struct {
	Tool    string           `json:"tool"`
	Target  string           `json:"target"`
	Action  PermissionAction `json:"action"`
	Scope   string           `json:"scope,omitempty"`
	Allowed bool             `json:"allowed"`
	Reason  string           `json:"reason,omitempty"`
}

type SessionSummary struct {
	SessionID   string       `json:"session_id"`
	Title       string       `json:"title"`
	CWD         string       `json:"cwd"`
	Provider    ProviderName `json:"provider"`
	Model       string       `json:"model"`
	CreatedAt   time.Time    `json:"created_at"`
	LastUpdated time.Time    `json:"last_updated"`
}
