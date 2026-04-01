package app

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/oladapodev/claudecode-go/internal/config"
	"github.com/oladapodev/claudecode-go/internal/domain"
	"github.com/oladapodev/claudecode-go/internal/history"
	"github.com/oladapodev/claudecode-go/internal/paths"
	"github.com/oladapodev/claudecode-go/internal/session"
	itools "github.com/oladapodev/claudecode-go/internal/tools"
	"github.com/oladapodev/claudecode-go/internal/transcript"
)

type mockProvider struct{}

func (mockProvider) StreamTurn(_ context.Context, req domain.TurnRequest) (<-chan domain.StreamEvent, <-chan error) {
	events := make(chan domain.StreamEvent, 4)
	errs := make(chan error, 1)

	go func() {
		defer close(events)
		defer close(errs)

		hasToolResult := false
		for _, msg := range req.Messages {
			if msg.Role == domain.RoleTool {
				hasToolResult = true
				break
			}
		}

		if !hasToolResult {
			events <- domain.StreamEvent{
				Type: domain.EventToolCall,
				Call: &domain.ToolCall{
					ID:    "todo-1",
					Name:  "todo_write",
					Input: []byte(`{"items":["verify rewrite"]}`),
				},
			}
			events <- domain.StreamEvent{Type: domain.EventDone}
			return
		}

		events <- domain.StreamEvent{Type: domain.EventTextDelta, Text: "all set"}
		events <- domain.StreamEvent{Type: domain.EventDone}
	}()

	return events, errs
}

func (mockProvider) ListModels(context.Context, domain.ProviderProfile) ([]domain.Model, error) {
	return []domain.Model{{ID: "mock"}}, nil
}

func (mockProvider) ValidateProfile(context.Context, domain.ProviderProfile) error { return nil }

func TestControllerRunsToolLoop(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	p := paths.Paths{
		ConfigDir:   filepath.Join(root, "config"),
		StateDir:    filepath.Join(root, "state"),
		ConfigFile:  filepath.Join(root, "config", "config.yaml"),
		SessionDir:  filepath.Join(root, "state", "sessions"),
		HistoryFile: filepath.Join(root, "state", "history.jsonl"),
		TodoDir:     filepath.Join(root, "state", "todos"),
	}
	if err := p.Ensure(); err != nil {
		t.Fatalf("Ensure() error = %v", err)
	}

	cfg := domain.DefaultConfig()
	cfg.DefaultModel = "mock"
	cfg.DefaultProvider = domain.ProviderAnthropic
	cfg.Profiles["default"] = domain.ProviderProfile{
		Name:        "default",
		Provider:    domain.ProviderAnthropic,
		BaseURL:     "http://mock",
		Model:       "mock",
		Temperature: 0.2,
		KeychainKey: "default",
	}

	configStore := config.NewStore(p)
	historyStore := history.NewStore(p)
	transcriptStore := transcript.NewStore(p)
	permissions := itools.NewPermissionService(&cfg, configStore.Save, nil)
	controller := NewControllerWithProviders(&cfg, configStore, historyStore, transcriptStore, permissions, map[domain.ProviderName]domain.Provider{
		domain.ProviderAnthropic: mockProvider{},
	})

	state, err := session.New("session-1", domain.ProviderAnthropic, "mock")
	if err != nil {
		t.Fatalf("session.New() error = %v", err)
	}
	state.Summary.CreatedAt = time.Now().UTC()
	if err := transcriptStore.StartSession(state.Summary); err != nil {
		t.Fatalf("StartSession() error = %v", err)
	}

	updates, errs := controller.RunTurn(context.Background(), &state, "do it", "default")
	var sawTool bool
	var finalText string
	for update := range updates {
		if update.ToolCall != nil {
			sawTool = true
		}
		if update.Text != "" {
			finalText += update.Text
		}
	}
	if err := <-errs; err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}

	if !sawTool {
		t.Fatalf("expected tool call update")
	}
	if finalText != "all set" {
		t.Fatalf("expected final text %q, got %q", "all set", finalText)
	}

	events, err := transcriptStore.Load(state.Summary.SessionID)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(events) < 5 {
		t.Fatalf("expected transcript events to include tool loop, got %d", len(events))
	}
}
