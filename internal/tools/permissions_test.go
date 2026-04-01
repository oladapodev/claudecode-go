package tools

import (
	"context"
	"testing"

	"github.com/oladapodev/claudecode-go/internal/domain"
)

type stubPrompter struct {
	response PromptResponse
	calls    int
}

func (s *stubPrompter) PromptPermission(context.Context, PromptRequest) (PromptResponse, error) {
	s.calls++
	return s.response, nil
}

func TestPermissionServiceCachesSessionDecision(t *testing.T) {
	t.Parallel()

	cfg := domain.DefaultConfig()
	prompter := &stubPrompter{
		response: PromptResponse{Action: domain.PermissionAllow, Scope: "session"},
	}
	service := NewPermissionService(&cfg, nil, prompter)

	decision, err := service.Resolve(context.Background(), "shell", "/tmp/demo", "run shell", domain.PermissionAsk)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if decision.Action != domain.PermissionAllow || decision.Scope != "session" {
		t.Fatalf("unexpected first decision: %#v", decision)
	}

	decision, err = service.Resolve(context.Background(), "shell", "/tmp/demo", "run shell", domain.PermissionAsk)
	if err != nil {
		t.Fatalf("Resolve(second) error = %v", err)
	}
	if decision.Scope != "session" {
		t.Fatalf("expected cached session decision, got %#v", decision)
	}
	if prompter.calls != 1 {
		t.Fatalf("expected prompter to be called once, got %d", prompter.calls)
	}
}

func TestPermissionServicePersistsConfigRule(t *testing.T) {
	t.Parallel()

	cfg := domain.DefaultConfig()
	prompter := &stubPrompter{
		response: PromptResponse{Action: domain.PermissionDeny, Scope: "config"},
	}
	service := NewPermissionService(&cfg, func(updated domain.Config) error {
		cfg = updated
		return nil
	}, prompter)

	_, err := service.Resolve(context.Background(), "write_file", "/tmp/out", "write file", domain.PermissionAsk)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	if len(cfg.Permissions.Rules) != 1 {
		t.Fatalf("expected persisted config rule, got %d", len(cfg.Permissions.Rules))
	}
	if cfg.Permissions.Rules[0].Action != domain.PermissionDeny {
		t.Fatalf("expected deny rule, got %#v", cfg.Permissions.Rules[0])
	}
}
