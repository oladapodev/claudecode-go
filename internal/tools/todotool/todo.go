package todotool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/oladapodev/claudecode-go/internal/domain"
)

type Tool struct {
	dir string
}

type Input struct {
	Items []string `json:"items"`
}

func New(dir string) domain.Tool {
	return Tool{dir: dir}
}

func (t Tool) Name() string { return "todo_write" }

func (t Tool) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"items":{"type":"array","items":{"type":"string"}}},"required":["items"]}`)
}

func (t Tool) Description() string { return "Persist a simple todo list for the current session." }

func (t Tool) Authorize(ctx context.Context, invocation domain.ToolInvocation, resolver domain.PermissionResolver) (domain.ToolAuthorization, error) {
	target := filepath.Join(t.dir, invocation.SessionID+".json")
	decision, err := resolver.Resolve(ctx, t.Name(), target, "updating session todos", domain.PermissionAllow)
	if err != nil {
		return domain.ToolAuthorization{}, err
	}
	return domain.ToolAuthorization{Target: target, Decision: decision}, nil
}

func (t Tool) Run(_ context.Context, invocation domain.ToolInvocation) (domain.ToolResult, error) {
	var input Input
	if err := json.Unmarshal(invocation.Input, &input); err != nil {
		return domain.ToolResult{}, err
	}

	if err := os.MkdirAll(t.dir, 0o700); err != nil {
		return domain.ToolResult{}, err
	}

	path := filepath.Join(t.dir, invocation.SessionID+".json")
	raw, err := json.MarshalIndent(input, "", "  ")
	if err != nil {
		return domain.ToolResult{}, err
	}

	if err := os.WriteFile(path, raw, 0o600); err != nil {
		return domain.ToolResult{}, err
	}

	return domain.ToolResult{Content: "updated todos"}, nil
}
