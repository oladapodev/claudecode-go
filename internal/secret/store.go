package secret

import (
	"context"
	"os"

	"github.com/oladapodev/claudecode-go/internal/domain"
	"github.com/zalando/go-keyring"
)

const serviceName = "claudecode-go"

type Store interface {
	Get(context.Context, domain.ProviderProfile) (string, error)
	Set(context.Context, domain.ProviderProfile, string) error
}

type KeychainStore struct{}

func New() Store {
	return KeychainStore{}
}

func (KeychainStore) Get(_ context.Context, profile domain.ProviderProfile) (string, error) {
	if profile.APIKeyEnv != "" {
		if value := os.Getenv(profile.APIKeyEnv); value != "" {
			return value, nil
		}
	}

	return keyring.Get(serviceName, profile.KeychainKey)
}

func (KeychainStore) Set(_ context.Context, profile domain.ProviderProfile, secret string) error {
	return keyring.Set(serviceName, profile.KeychainKey, secret)
}
