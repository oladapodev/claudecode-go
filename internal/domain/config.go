package domain

type Config struct {
	DefaultProvider ProviderName               `yaml:"default_provider" json:"default_provider"`
	DefaultModel    string                     `yaml:"default_model" json:"default_model"`
	DefaultProfile  string                     `yaml:"default_profile" json:"default_profile"`
	Profiles        map[string]ProviderProfile `yaml:"profiles" json:"profiles"`
	UI              UIConfig                   `yaml:"ui" json:"ui"`
	Permissions     PermissionSettings         `yaml:"permissions" json:"permissions"`
}

type UIConfig struct {
	ShowTimestamps bool `yaml:"show_timestamps" json:"show_timestamps"`
}

type ProviderProfile struct {
	Name         string       `yaml:"name" json:"name"`
	Provider     ProviderName `yaml:"provider" json:"provider"`
	BaseURL      string       `yaml:"base_url" json:"base_url"`
	Model        string       `yaml:"model" json:"model"`
	Temperature  float64      `yaml:"temperature" json:"temperature"`
	KeychainKey  string       `yaml:"keychain_key" json:"keychain_key"`
	APIKeyEnv    string       `yaml:"api_key_env" json:"api_key_env"`
	Organization string       `yaml:"organization,omitempty" json:"organization,omitempty"`
}

type PermissionSettings struct {
	DefaultAction PermissionAction `yaml:"default_action" json:"default_action"`
	Rules         []PermissionRule `yaml:"rules" json:"rules"`
}

type PermissionAction string

const (
	PermissionAsk   PermissionAction = "ask"
	PermissionAllow PermissionAction = "allow"
	PermissionDeny  PermissionAction = "deny"
)

type PermissionRule struct {
	Tool       string           `yaml:"tool" json:"tool"`
	Pattern    string           `yaml:"pattern" json:"pattern"`
	Action     PermissionAction `yaml:"action" json:"action"`
	Persistent bool             `yaml:"persistent" json:"persistent"`
}

func DefaultConfig() Config {
	return Config{
		DefaultProvider: ProviderAnthropic,
		DefaultModel:    "claude-3-7-sonnet-latest",
		DefaultProfile:  "default",
		Profiles: map[string]ProviderProfile{
			"default": {
				Name:        "default",
				Provider:    ProviderAnthropic,
				BaseURL:     "https://api.anthropic.com",
				Model:       "claude-3-7-sonnet-latest",
				Temperature: 0.2,
				KeychainKey: "default",
				APIKeyEnv:   "ANTHROPIC_API_KEY",
			},
		},
		UI: UIConfig{
			ShowTimestamps: true,
		},
		Permissions: PermissionSettings{
			DefaultAction: PermissionAsk,
			Rules:         []PermissionRule{},
		},
	}
}
