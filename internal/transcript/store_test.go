package transcript

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/oladapodev/claudecode-go/internal/domain"
	"github.com/oladapodev/claudecode-go/internal/paths"
)

func TestStoreAndRebuildConversation(t *testing.T) {
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

	store := NewStore(p)
	summary := domain.SessionSummary{
		SessionID: "session-1",
		Title:     "demo",
		CWD:       "/tmp/demo",
		Provider:  domain.ProviderAnthropic,
		Model:     "claude",
		CreatedAt: time.Now().UTC(),
	}

	if err := store.StartSession(summary); err != nil {
		t.Fatalf("StartSession() error = %v", err)
	}

	call := domain.ToolCall{ID: "call-1", Name: "shell", Input: []byte(`{"command":"pwd"}`)}
	if err := store.Append(summary.SessionID, domain.TranscriptEvent{
		Type:      "tool_call",
		Timestamp: time.Now().UTC(),
		SessionID: summary.SessionID,
		ToolCall:  &call,
	}); err != nil {
		t.Fatalf("Append(tool_call) error = %v", err)
	}

	result := domain.Message{
		Role:       domain.RoleTool,
		ToolCallID: "call-1",
		ToolName:   "shell",
		Content:    "/tmp/demo",
	}
	if err := store.Append(summary.SessionID, domain.TranscriptEvent{
		Type:       "tool_result",
		Timestamp:  time.Now().UTC(),
		SessionID:  summary.SessionID,
		ToolResult: &result,
	}); err != nil {
		t.Fatalf("Append(tool_result) error = %v", err)
	}

	events, err := store.Load(summary.SessionID)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	messages, rebuilt := RebuildConversation(events)
	if rebuilt.SessionID != summary.SessionID {
		t.Fatalf("expected session ID %q, got %q", summary.SessionID, rebuilt.SessionID)
	}
	if len(messages) != 2 {
		t.Fatalf("expected 2 messages after rebuild, got %d", len(messages))
	}
	if messages[0].ToolName != "shell" || messages[1].Role != domain.RoleTool {
		t.Fatalf("unexpected rebuilt messages: %#v", messages)
	}
}
