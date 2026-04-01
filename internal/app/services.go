package app

import (
	"github.com/oladapodev/claudecode-go/internal/config"
	"github.com/oladapodev/claudecode-go/internal/domain"
	"github.com/oladapodev/claudecode-go/internal/history"
	"github.com/oladapodev/claudecode-go/internal/paths"
	"github.com/oladapodev/claudecode-go/internal/secret"
	itools "github.com/oladapodev/claudecode-go/internal/tools"
	"github.com/oladapodev/claudecode-go/internal/transcript"
)

type Services struct {
	Paths       paths.Paths
	Config      *domain.Config
	ConfigStore config.Store
	Secrets     secret.Store
	History     history.Store
	Transcript  transcript.Store
	Permissions *itools.PermissionService
	Controller  *Controller
}

func Bootstrap(prompter itools.Prompter) (*Services, error) {
	p, err := paths.Resolve()
	if err != nil {
		return nil, err
	}
	if err := p.Ensure(); err != nil {
		return nil, err
	}

	configStore := config.NewStore(p)
	cfg, err := configStore.Load()
	if err != nil {
		return nil, err
	}

	secrets := secret.New()
	historyStore := history.NewStore(p)
	transcriptStore := transcript.NewStore(p)
	permissions := itools.NewPermissionService(&cfg, configStore.Save, prompter)
	controller := NewController(&cfg, configStore, historyStore, transcriptStore, permissions, secrets)

	return &Services{
		Paths:       p,
		Config:      &cfg,
		ConfigStore: configStore,
		Secrets:     secrets,
		History:     historyStore,
		Transcript:  transcriptStore,
		Permissions: permissions,
		Controller:  controller,
	}, nil
}
