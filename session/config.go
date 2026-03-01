package session

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Config holds persistent user preferences.
type Config struct {
	Location string `json:"location,omitempty"`
}

func configPath() string {
	base := os.Getenv("XDG_DATA_HOME")
	if base == "" {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(base, "localfreshllm", "config.json")
}

// LoadConfig reads config from disk. Returns empty config if file doesn't exist.
func LoadConfig() *Config {
	data, err := os.ReadFile(configPath())
	if err != nil {
		return &Config{}
	}
	var cfg Config
	if json.Unmarshal(data, &cfg) != nil {
		return &Config{}
	}
	return &cfg
}

// Save writes config to disk.
func (c *Config) Save() error {
	path := configPath()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}
