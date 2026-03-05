package config

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	"github.com/BurntSushi/toml"
)

// AccountEntry holds per-account display configuration from [accounts."***NNN"] blocks.
type AccountEntry struct {
	Name  string `toml:"name"`
	Order int    `toml:"order"`
}

// Config holds the application credentials and optional display settings.
type Config struct {
	AppKey   string
	Secret   string
	Accounts map[string]AccountEntry // keyed by masked account id, e.g. "***176"
}

// mainConfig mirrors the structure of config.toml for TOML decoding.
type mainConfig struct {
	AppConfig string                    `toml:"APP_CONFIG"`
	Accounts  map[string]AccountEntry   `toml:"accounts"`
}

// credsConfig mirrors the structure of the credentials file for TOML decoding.
type credsConfig struct {
	AppKey string `toml:"APP_KEY"`
	Secret string `toml:"SECRET"`
}

// configDir returns $XDG_CONFIG_HOME/schwab-cli or ~/.config/schwab-cli.
func configDir() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "schwab-cli")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".config", "schwab-cli")
	}
	return filepath.Join(home, ".config", "schwab-cli")
}

// ConfigDir returns the config directory path (exported for use by other packages).
func ConfigDir() string {
	return configDir()
}

// Load reads config.toml to find the credentials file path, security-checks
// that file, then loads APP_KEY, SECRET, and optional [[accounts]] entries.
func Load() (*Config, error) {
	mainPath := filepath.Join(configDir(), "config.toml")

	var mc mainConfig
	if _, err := toml.DecodeFile(mainPath, &mc); err != nil {
		return nil, fmt.Errorf("cannot read config file %s: %w", mainPath, err)
	}

	if mc.AppConfig == "" {
		return nil, fmt.Errorf("APP_CONFIG key not found in %s", mainPath)
	}

	// If APP_CONFIG is not an absolute path, resolve it relative to the
	// directory containing config.toml.
	credsPath := mc.AppConfig
	if !filepath.IsAbs(credsPath) {
		credsPath = filepath.Join(filepath.Dir(mainPath), credsPath)
	}

	if err := checkCredentialsFile(credsPath); err != nil {
		return nil, err
	}

	var cc credsConfig
	if _, err := toml.DecodeFile(credsPath, &cc); err != nil {
		return nil, fmt.Errorf("cannot read credentials file %s: %w", credsPath, err)
	}

	if cc.AppKey == "" {
		return nil, fmt.Errorf("APP_KEY not found in credentials file %s", credsPath)
	}
	if cc.Secret == "" {
		return nil, fmt.Errorf("SECRET not found in credentials file %s", credsPath)
	}

	return &Config{
		AppKey:   cc.AppKey,
		Secret:   cc.Secret,
		Accounts: mc.Accounts,
	}, nil
}

// checkCredentialsFile verifies the file is owned by the current user and has
// exactly 0600 permissions. Fails fast before any secrets are read.
func checkCredentialsFile(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("cannot stat credentials file %s: %w", path, err)
	}

	perm := info.Mode().Perm()
	if perm != 0600 {
		return fmt.Errorf("credentials file %s has unsafe permissions %04o — must be 0600", path, perm)
	}

	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("cannot read file ownership for %s", path)
	}
	if stat.Uid != uint32(os.Getuid()) {
		return fmt.Errorf("credentials file %s is not owned by the current user (owner uid: %d)", path, stat.Uid)
	}

	return nil
}
