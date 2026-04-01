package paths

import (
	"os"
	"path/filepath"
)

type Paths struct {
	ConfigDir   string
	StateDir    string
	ConfigFile  string
	SessionDir  string
	HistoryFile string
	TodoDir     string
}

func Resolve() (Paths, error) {
	configRoot, err := os.UserConfigDir()
	if err != nil {
		return Paths{}, err
	}

	stateRoot, err := userStateDir()
	if err != nil {
		return Paths{}, err
	}

	configDir := filepath.Join(configRoot, "claudecode-go")
	stateDir := filepath.Join(stateRoot, "claudecode-go")

	return Paths{
		ConfigDir:   configDir,
		StateDir:    stateDir,
		ConfigFile:  filepath.Join(configDir, "config.yaml"),
		SessionDir:  filepath.Join(stateDir, "sessions"),
		HistoryFile: filepath.Join(stateDir, "history.jsonl"),
		TodoDir:     filepath.Join(stateDir, "todos"),
	}, nil
}

func userStateDir() (string, error) {
	if value := os.Getenv("XDG_STATE_HOME"); value != "" {
		return value, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "state"), nil
}

func (p Paths) Ensure() error {
	for _, dir := range []string{p.ConfigDir, p.StateDir, p.SessionDir, p.TodoDir} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
	}
	return nil
}
