package history

import (
	"bufio"
	"encoding/json"
	"errors"
	"os"
	"time"

	"github.com/oladapodev/claudecode-go/internal/paths"
)

type Entry struct {
	SessionID string    `json:"session_id"`
	Prompt    string    `json:"prompt"`
	Timestamp time.Time `json:"timestamp"`
}

type Store struct {
	Paths paths.Paths
}

func NewStore(p paths.Paths) Store {
	return Store{Paths: p}
}

func (s Store) Append(entry Entry) error {
	if err := s.Paths.Ensure(); err != nil {
		return err
	}
	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now().UTC()
	}

	file, err := os.OpenFile(s.Paths.HistoryFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()

	return json.NewEncoder(file).Encode(entry)
}

func (s Store) Recent(limit int) ([]Entry, error) {
	file, err := os.Open(s.Paths.HistoryFile)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []Entry{}, nil
		}
		return nil, err
	}
	defer file.Close()

	var entries []Entry
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var entry Entry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	if limit > 0 && len(entries) > limit {
		entries = entries[len(entries)-limit:]
	}

	return entries, nil
}
