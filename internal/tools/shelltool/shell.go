package shelltool

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os/exec"
	"strings"

	"github.com/oladapodev/claudecode-go/internal/domain"
)

type Input struct {
	Command string `json:"command"`
}

type Tool struct{}

func New() domain.Tool {
	return Tool{}
}

func (Tool) Name() string { return "shell" }

func (Tool) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"command":{"type":"string"}},"required":["command"]}`)
}

func (Tool) Description() string { return "Run a shell command in the current workspace." }

func (Tool) Authorize(ctx context.Context, invocation domain.ToolInvocation, resolver domain.PermissionResolver) (domain.ToolAuthorization, error) {
	var input Input
	if err := json.Unmarshal(invocation.Input, &input); err != nil {
		return domain.ToolAuthorization{}, err
	}
	if strings.TrimSpace(input.Command) == "" {
		return domain.ToolAuthorization{}, errors.New("command is required")
	}
	reason := "shell command execution requires confirmation"
	if isDangerous(input.Command) {
		reason = "dangerous shell command requires explicit confirmation"
	}
	decision, err := resolver.Resolve(ctx, "shell", input.Command, reason, domain.PermissionAsk)
	if err != nil {
		return domain.ToolAuthorization{}, err
	}
	return domain.ToolAuthorization{Target: input.Command, Decision: decision}, nil
}

func (Tool) Run(ctx context.Context, invocation domain.ToolInvocation) (domain.ToolResult, error) {
	var input Input
	if err := json.Unmarshal(invocation.Input, &input); err != nil {
		return domain.ToolResult{}, err
	}

	cmd := exec.CommandContext(ctx, "bash", "-lc", input.Command)
	cmd.Dir = invocation.CWD

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	out := strings.TrimSpace(stdout.String())
	errOut := strings.TrimSpace(stderr.String())

	combined := out
	if errOut != "" {
		if combined != "" {
			combined += "\n"
		}
		combined += errOut
	}

	if combined == "" && err == nil {
		combined = "(no output)"
	}

	if err != nil {
		if combined == "" {
			combined = err.Error()
		}
		return domain.ToolResult{Content: combined}, err
	}

	return domain.ToolResult{Content: combined}, nil
}

func isDangerous(command string) bool {
	needles := []string{"rm -rf", "sudo ", "mkfs", "shutdown", "reboot", "poweroff", ":(){", "dd if="}
	for _, needle := range needles {
		if strings.Contains(command, needle) {
			return true
		}
	}
	return false
}
