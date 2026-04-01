package ui

import (
	"testing"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/oladapodev/claudecode-go/internal/app"
	"github.com/oladapodev/claudecode-go/internal/domain"
)

func TestHelpCommandAddsMessage(t *testing.T) {
	t.Parallel()

	m := &model{
		services: &app.Services{
			Config: &domain.Config{},
		},
		viewport: viewport.New(80, 20),
	}

	_, _ = m.handleSlashCommand("/help")
	if len(m.messages) != 1 {
		t.Fatalf("expected one message after /help, got %d", len(m.messages))
	}
	if m.messages[0].Role != "meta" {
		t.Fatalf("expected meta message, got %#v", m.messages[0])
	}
}
