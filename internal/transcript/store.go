package transcript

import (
	"bufio"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/oladapodev/claudecode-go/internal/domain"
	"github.com/oladapodev/claudecode-go/internal/paths"
)

type Store struct {
	Paths paths.Paths
}

func NewStore(p paths.Paths) Store {
	return Store{Paths: p}
}

func NewSessionID() string {
	return uuid.NewString()
}

func (s Store) StartSession(summary domain.SessionSummary) error {
	return s.Append(summary.SessionID, domain.TranscriptEvent{
		Type:      "session_started",
		Timestamp: summary.CreatedAt,
		SessionID: summary.SessionID,
		Title:     summary.Title,
		CWD:       summary.CWD,
		Provider:  summary.Provider,
		Model:     summary.Model,
	})
}

func (s Store) Append(sessionID string, event domain.TranscriptEvent) error {
	if err := s.Paths.Ensure(); err != nil {
		return err
	}

	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now().UTC()
	}

	path := filepath.Join(s.Paths.SessionDir, sessionID+".jsonl")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()

	enc := json.NewEncoder(file)
	return enc.Encode(event)
}

func (s Store) Load(sessionID string) ([]domain.TranscriptEvent, error) {
	path := filepath.Join(s.Paths.SessionDir, sessionID+".jsonl")
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var events []domain.TranscriptEvent
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var event domain.TranscriptEvent
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			return nil, err
		}
		events = append(events, event)
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return events, nil
}

func (s Store) List(query string) ([]domain.SessionSummary, error) {
	if err := s.Paths.Ensure(); err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(s.Paths.SessionDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []domain.SessionSummary{}, nil
		}
		return nil, err
	}

	query = strings.ToLower(strings.TrimSpace(query))
	summaries := make([]domain.SessionSummary, 0, len(entries))

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".jsonl" {
			continue
		}

		summary, err := readSummary(filepath.Join(s.Paths.SessionDir, entry.Name()))
		if err != nil {
			continue
		}

		if query != "" {
			haystack := strings.ToLower(summary.Title + " " + summary.SessionID + " " + summary.CWD + " " + summary.Model)
			if !strings.Contains(haystack, query) {
				continue
			}
		}

		summaries = append(summaries, summary)
	}

	sort.Slice(summaries, func(i, j int) bool {
		return summaries[i].LastUpdated.After(summaries[j].LastUpdated)
	})

	return summaries, nil
}

func readSummary(path string) (domain.SessionSummary, error) {
	file, err := os.Open(path)
	if err != nil {
		return domain.SessionSummary{}, err
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return domain.SessionSummary{}, err
	}

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var event domain.TranscriptEvent
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			return domain.SessionSummary{}, err
		}
		if event.Type == "session_started" {
			return domain.SessionSummary{
				SessionID:   event.SessionID,
				Title:       event.Title,
				CWD:         event.CWD,
				Provider:    event.Provider,
				Model:       event.Model,
				CreatedAt:   event.Timestamp,
				LastUpdated: info.ModTime(),
			}, nil
		}
	}

	if err := scanner.Err(); err != nil {
		return domain.SessionSummary{}, err
	}

	return domain.SessionSummary{}, os.ErrNotExist
}
