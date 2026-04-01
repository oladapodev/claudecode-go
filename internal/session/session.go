package session

import (
	"os"
	"path/filepath"
	"time"

	"github.com/oladapodev/claudecode-go/internal/domain"
	"github.com/oladapodev/claudecode-go/internal/transcript"
)

type State struct {
	Summary  domain.SessionSummary
	Messages []domain.Message
}

func New(sessionID string, provider domain.ProviderName, model string) (State, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return State{}, err
	}

	title := filepath.Base(cwd)
	if title == "" || title == "." || title == string(filepath.Separator) {
		title = "session"
	}

	return State{
		Summary: domain.SessionSummary{
			SessionID: sessionID,
			Title:     title,
			CWD:       cwd,
			Provider:  provider,
			Model:     model,
			CreatedAt: time.Now().UTC(),
		},
		Messages: []domain.Message{},
	}, nil
}

func FromEvents(events []domain.TranscriptEvent) State {
	messages, summary := transcript.RebuildConversation(events)
	return State{
		Summary:  summary,
		Messages: messages,
	}
}
