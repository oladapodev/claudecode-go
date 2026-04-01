package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/oladapodev/claudecode-go/internal/paths"
)

func TestLoadCreatesDefaultConfig(t *testing.T) {
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
	cfg, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.DefaultProfile != "default" {
		t.Fatalf("expected default profile, got %q", cfg.DefaultProfile)
	}

	if _, err := os.Stat(p.ConfigFile); err != nil {
		t.Fatalf("expected config file to be created: %v", err)
	}
}
