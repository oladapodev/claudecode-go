package domain

import (
	"context"
	"encoding/json"
)

type ToolAuthorization struct {
	Target   string
	Decision PermissionDecision
}

type ToolResult struct {
	Content string          `json:"content"`
	Data    json.RawMessage `json:"data,omitempty"`
}

type Tool interface {
	Name() string
	Schema() json.RawMessage
	Description() string
	Authorize(context.Context, ToolInvocation, PermissionResolver) (ToolAuthorization, error)
	Run(context.Context, ToolInvocation) (ToolResult, error)
}

type ToolInvocation struct {
	Name      string
	Input     json.RawMessage
	CWD       string
	SessionID string
}

type PermissionResolver interface {
	Resolve(context.Context, string, string, string, PermissionAction) (PermissionDecision, error)
}

type PermissionDecision struct {
	Action PermissionAction
	Scope  string
	Reason string
}
