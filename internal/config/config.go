package config

import (
	"os"
	"path/filepath"
	"time"
)

// Config holds all configuration for Wellspring.
type Config struct {
	General  GeneralConfig            `mapstructure:"general"`
	Keys     map[string]string        `mapstructure:"keys"`
	Sources  map[string]SourceConfig  `mapstructure:"sources"`
}

// GeneralConfig holds general settings.
type GeneralConfig struct {
	DefaultFormat string        `mapstructure:"default_format"`
	DefaultLimit  int           `mapstructure:"default_limit"`
	CacheDir      string        `mapstructure:"cache_dir"`
	CacheTTL      time.Duration `mapstructure:"cache_ttl"`
}

// SourceConfig holds per-source overrides.
type SourceConfig struct {
	DefaultSubreddit string `mapstructure:"default_subreddit"`
}

// DefaultConfig returns the default configuration.
func DefaultConfig() *Config {
	return &Config{
		General: GeneralConfig{
			DefaultFormat: "table",
			DefaultLimit:  10,
			CacheDir:      DefaultCacheDir(),
			CacheTTL:      5 * time.Minute,
		},
		Keys:    make(map[string]string),
		Sources: make(map[string]SourceConfig),
	}
}

// ConfigDir returns the XDG config directory for Wellspring.
func ConfigDir() string {
	if dir := os.Getenv("WSP_CONFIG_DIR"); dir != "" {
		return dir
	}
	if dir := os.Getenv("XDG_CONFIG_HOME"); dir != "" {
		return filepath.Join(dir, "wellspring")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "wellspring")
}

// DefaultCacheDir returns the XDG cache directory for Wellspring.
func DefaultCacheDir() string {
	if dir := os.Getenv("WSP_CACHE_DIR"); dir != "" {
		return dir
	}
	if dir := os.Getenv("XDG_CACHE_HOME"); dir != "" {
		return filepath.Join(dir, "wellspring")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".cache", "wellspring")
}

// UserSourcesDir returns the path where users can place custom source YAML files.
func UserSourcesDir() string {
	return filepath.Join(ConfigDir(), "sources")
}

// ConfigFilePath returns the path to the main config file.
func ConfigFilePath() string {
	return filepath.Join(ConfigDir(), "config.toml")
}

// GetAPIKey returns an API key, checking env vars first, then config.
func (c *Config) GetAPIKey(name string) string {
	// Check environment variable first (e.g., WSP_ALPHA_VANTAGE_KEY)
	envKey := os.Getenv("WSP_" + name + "_KEY")
	if envKey != "" {
		return envKey
	}
	// Fall back to config file
	if c.Keys != nil {
		return c.Keys[name]
	}
	return ""
}
