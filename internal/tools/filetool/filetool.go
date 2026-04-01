package filetool

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/oladapodev/claudecode-go/internal/domain"
)

type readInput struct {
	Path string `json:"path"`
}

type writeInput struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

type editInput struct {
	Path string `json:"path"`
	Old  string `json:"old"`
	New  string `json:"new"`
	All  bool   `json:"all"`
}

type tool struct {
	name        string
	description string
	schema      json.RawMessage
	run         func(context.Context, domain.ToolInvocation) (domain.ToolResult, error)
	authorize   func(context.Context, domain.ToolInvocation, domain.PermissionResolver) (domain.ToolAuthorization, error)
}

func NewReadTool() domain.Tool {
	return tool{
		name:        "read_file",
		description: "Read a local file from disk.",
		schema:      mustSchema(`{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}`),
		run: func(_ context.Context, invocation domain.ToolInvocation) (domain.ToolResult, error) {
			var input readInput
			if err := json.Unmarshal(invocation.Input, &input); err != nil {
				return domain.ToolResult{}, err
			}
			path, err := resolvePath(invocation.CWD, input.Path)
			if err != nil {
				return domain.ToolResult{}, err
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				return domain.ToolResult{}, err
			}
			return domain.ToolResult{Content: string(raw)}, nil
		},
		authorize: func(ctx context.Context, invocation domain.ToolInvocation, resolver domain.PermissionResolver) (domain.ToolAuthorization, error) {
			var input readInput
			if err := json.Unmarshal(invocation.Input, &input); err != nil {
				return domain.ToolAuthorization{}, err
			}
			path, err := resolvePath(invocation.CWD, input.Path)
			if err != nil {
				return domain.ToolAuthorization{}, err
			}
			fallback := domain.PermissionAsk
			reason := "file read requires confirmation"
			if withinWorkspace(invocation.CWD, path) {
				fallback = domain.PermissionAllow
				reason = "reading within workspace"
			}
			decision, err := resolver.Resolve(ctx, "read_file", path, reason, fallback)
			if err != nil {
				return domain.ToolAuthorization{}, err
			}
			return domain.ToolAuthorization{Target: path, Decision: decision}, nil
		},
	}
}

func NewWriteTool() domain.Tool {
	return tool{
		name:        "write_file",
		description: "Write a local file to disk, creating parent directories if needed.",
		schema:      mustSchema(`{"type":"object","properties":{"path":{"type":"string"},"content":{"type":"string"}},"required":["path","content"]}`),
		run: func(_ context.Context, invocation domain.ToolInvocation) (domain.ToolResult, error) {
			var input writeInput
			if err := json.Unmarshal(invocation.Input, &input); err != nil {
				return domain.ToolResult{}, err
			}
			path, err := resolvePath(invocation.CWD, input.Path)
			if err != nil {
				return domain.ToolResult{}, err
			}
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				return domain.ToolResult{}, err
			}
			if err := os.WriteFile(path, []byte(input.Content), 0o644); err != nil {
				return domain.ToolResult{}, err
			}
			return domain.ToolResult{Content: "wrote " + path}, nil
		},
		authorize: func(ctx context.Context, invocation domain.ToolInvocation, resolver domain.PermissionResolver) (domain.ToolAuthorization, error) {
			var input writeInput
			if err := json.Unmarshal(invocation.Input, &input); err != nil {
				return domain.ToolAuthorization{}, err
			}
			path, err := resolvePath(invocation.CWD, input.Path)
			if err != nil {
				return domain.ToolAuthorization{}, err
			}
			reason := "file writes require confirmation"
			if !withinWorkspace(invocation.CWD, path) {
				reason = "writing outside the workspace requires confirmation"
			}
			decision, err := resolver.Resolve(ctx, "write_file", path, reason, domain.PermissionAsk)
			if err != nil {
				return domain.ToolAuthorization{}, err
			}
			return domain.ToolAuthorization{Target: path, Decision: decision}, nil
		},
	}
}

func NewEditTool() domain.Tool {
	return tool{
		name:        "edit_file",
		description: "Edit a local file by replacing text.",
		schema:      mustSchema(`{"type":"object","properties":{"path":{"type":"string"},"old":{"type":"string"},"new":{"type":"string"},"all":{"type":"boolean"}},"required":["path","old","new"]}`),
		run: func(_ context.Context, invocation domain.ToolInvocation) (domain.ToolResult, error) {
			var input editInput
			if err := json.Unmarshal(invocation.Input, &input); err != nil {
				return domain.ToolResult{}, err
			}
			path, err := resolvePath(invocation.CWD, input.Path)
			if err != nil {
				return domain.ToolResult{}, err
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				return domain.ToolResult{}, err
			}
			contents := string(raw)
			if !strings.Contains(contents, input.Old) {
				return domain.ToolResult{}, errors.New("target text not found")
			}
			var updated string
			if input.All {
				updated = strings.ReplaceAll(contents, input.Old, input.New)
			} else {
				updated = strings.Replace(contents, input.Old, input.New, 1)
			}
			if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
				return domain.ToolResult{}, err
			}
			return domain.ToolResult{Content: "edited " + path}, nil
		},
		authorize: func(ctx context.Context, invocation domain.ToolInvocation, resolver domain.PermissionResolver) (domain.ToolAuthorization, error) {
			var input editInput
			if err := json.Unmarshal(invocation.Input, &input); err != nil {
				return domain.ToolAuthorization{}, err
			}
			path, err := resolvePath(invocation.CWD, input.Path)
			if err != nil {
				return domain.ToolAuthorization{}, err
			}
			reason := "file edits require confirmation"
			if !withinWorkspace(invocation.CWD, path) {
				reason = "editing outside the workspace requires confirmation"
			}
			decision, err := resolver.Resolve(ctx, "edit_file", path, reason, domain.PermissionAsk)
			if err != nil {
				return domain.ToolAuthorization{}, err
			}
			return domain.ToolAuthorization{Target: path, Decision: decision}, nil
		},
	}
}

func (t tool) Name() string { return t.name }

func (t tool) Schema() json.RawMessage { return t.schema }

func (t tool) Description() string { return t.description }

func (t tool) Authorize(ctx context.Context, invocation domain.ToolInvocation, resolver domain.PermissionResolver) (domain.ToolAuthorization, error) {
	return t.authorize(ctx, invocation, resolver)
}

func (t tool) Run(ctx context.Context, invocation domain.ToolInvocation) (domain.ToolResult, error) {
	return t.run(ctx, invocation)
}

func mustSchema(value string) json.RawMessage {
	return json.RawMessage(value)
}

func resolvePath(cwd, value string) (string, error) {
	if value == "" {
		return "", errors.New("path is required")
	}
	if filepath.IsAbs(value) {
		return filepath.Clean(value), nil
	}
	return filepath.Clean(filepath.Join(cwd, value)), nil
}

func withinWorkspace(cwd, value string) bool {
	cwd = filepath.Clean(cwd)
	value = filepath.Clean(value)
	return value == cwd || strings.HasPrefix(value, cwd+string(filepath.Separator))
}
