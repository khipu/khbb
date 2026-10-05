// Package config reads and writes khbb's configuration file and stored credentials.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"

	"gopkg.in/yaml.v3"
)

const fileName = "config.yml"

// Config is the content of config.yml.
type Config struct {
	Email         string `yaml:"email,omitempty"`
	Username      string `yaml:"username,omitempty"`
	GitProtocol   string `yaml:"git_protocol,omitempty"`
	Editor        string `yaml:"editor,omitempty"`
	Pager         string `yaml:"pager,omitempty"`
	Browser       string `yaml:"browser,omitempty"`
	InsecureToken string `yaml:"token,omitempty"` // only with `auth login --insecure-storage`

	path string
}

// Dir returns the directory that holds config.yml.
func Dir() (string, error) {
	if d := os.Getenv("KHBB_CONFIG_DIR"); d != "" {
		return d, nil
	}
	if d := os.Getenv("XDG_CONFIG_HOME"); d != "" {
		return filepath.Join(d, "khbb"), nil
	}
	if runtime.GOOS == "windows" {
		if d := os.Getenv("AppData"); d != "" {
			return filepath.Join(d, "khbb"), nil
		}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("locating home directory: %w", err)
	}
	return filepath.Join(home, ".config", "khbb"), nil
}

// Load reads config.yml from Dir(). A missing file yields an empty Config.
func Load() (*Config, error) {
	dir, err := Dir()
	if err != nil {
		return nil, err
	}
	return LoadFile(filepath.Join(dir, fileName))
}

// LoadFile reads the config at path. A missing file yields an empty Config bound to path.
func LoadFile(path string) (*Config, error) {
	cfg := &Config{path: path}
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	return cfg, nil
}

// Path returns the file this Config is read from and saved to.
func (c *Config) Path() string { return c.path }

// Save writes the config atomically with owner-only permissions.
func (c *Config) Save() error {
	if c.path == "" {
		return errors.New("config has no file path")
	}
	if err := os.MkdirAll(filepath.Dir(c.path), 0o700); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}
	data, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	tmp := c.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}
	if err := os.Rename(tmp, c.path); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}
	return nil
}
