package tools

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/oladapodev/claudecode-go/internal/domain"
	"github.com/oladapodev/claudecode-go/internal/paths"
	"github.com/oladapodev/claudecode-go/internal/tools/filetool"
	"github.com/oladapodev/claudecode-go/internal/tools/searchtool"
	"github.com/oladapodev/claudecode-go/internal/tools/shelltool"
	"github.com/oladapodev/claudecode-go/internal/tools/todotool"
)

var ErrToolNotFound = errors.New("tool not found")
var ErrPermissionDenied = errors.New("tool permission denied")

type Manager struct {
	registry    map[string]domain.Tool
	permissions *PermissionService
}

func NewManager(p paths.Paths, permissions *PermissionService) *Manager {
	tools := []domain.Tool{
		filetool.NewReadTool(),
		filetool.NewWriteTool(),
		filetool.NewEditTool(),
		searchtool.NewGlobTool(),
		searchtool.NewGrepTool(),
		shelltool.New(),
		todotool.New(p.TodoDir),
	}

	registry := make(map[string]domain.Tool, len(tools))
	for _, tool := range tools {
		registry[tool.Name()] = tool
	}

	return &Manager{
		registry:    registry,
		permissions: permissions,
	}
}

func (m *Manager) Definitions() []domain.ToolDefinition {
	defs := make([]domain.ToolDefinition, 0, len(m.registry))
	for _, tool := range m.registry {
		defs = append(defs, domain.ToolDefinition{
			Name:        tool.Name(),
			Description: tool.Description(),
			Schema:      tool.Schema(),
		})
	}
	return defs
}

func (m *Manager) Invoke(ctx context.Context, invocation domain.ToolInvocation) (domain.ToolResult, *domain.PermissionLog, error) {
	tool, ok := m.registry[invocation.Name]
	if !ok {
		return domain.ToolResult{}, nil, ErrToolNotFound
	}

	auth, err := tool.Authorize(ctx, invocation, m.permissions)
	if err != nil {
		return domain.ToolResult{}, nil, err
	}

	log := &domain.PermissionLog{
		Tool:    invocation.Name,
		Target:  auth.Target,
		Action:  auth.Decision.Action,
		Scope:   auth.Decision.Scope,
		Allowed: auth.Decision.Action == domain.PermissionAllow,
		Reason:  auth.Decision.Reason,
	}

	if auth.Decision.Action != domain.PermissionAllow {
		return domain.ToolResult{}, log, ErrPermissionDenied
	}

	result, err := tool.Run(ctx, invocation)
	return result, log, err
}

func MustJSON(v any) json.RawMessage {
	raw, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return raw
}
