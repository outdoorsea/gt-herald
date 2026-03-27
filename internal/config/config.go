// Package config provides YAML configuration loading for gt-herald.
package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Config is the top-level gt-herald configuration.
type Config struct {
	Slack     SlackConfig     `yaml:"slack"`
	GasTown   GasTownConfig  `yaml:"gastown"`
	Channels  ChannelConfig   `yaml:"channels"`
	Filters   FilterConfig    `yaml:"filters"`
	RateLimit RateLimitConfig `yaml:"rate_limit"`
	StateFile string          `yaml:"state_file"`
}

// SlackConfig holds Slack connection settings.
type SlackConfig struct {
	TokenEnv      string `yaml:"token_env"`
	WebhookURLEnv string `yaml:"webhook_url_env"`
}

// GasTownConfig holds Gas Town workspace settings.
type GasTownConfig struct {
	Root     string `yaml:"root"`
	DoltPort int    `yaml:"dolt_port"`
}

// ChannelConfig holds channel routing configuration.
type ChannelConfig struct {
	Alerts  string            `yaml:"alerts"`
	Default string            `yaml:"default"`
	Rigs    map[string]string `yaml:"rigs"`
}

// FilterConfig holds event filter configuration.
type FilterConfig struct {
	Townlog TownlogFilter `yaml:"townlog"`
	Beads   BeadsFilter   `yaml:"beads"`
}

// BeadsFilter specifies which bead state transitions to include.
type BeadsFilter struct {
	Include []string `yaml:"include"` // bead_closed, bead_claimed, bead_blocked, bead_opened
}

// TownlogFilter specifies which townlog event types to include.
type TownlogFilter struct {
	Include []string `yaml:"include"`
}

// RateLimitConfig holds rate limiting settings.
type RateLimitConfig struct {
	PerChannel  int    `yaml:"per_channel"`
	Burst       int    `yaml:"burst"`
	BatchWindow string `yaml:"batch_window"`
}

// BatchWindowDuration returns the parsed batch window duration.
func (r RateLimitConfig) BatchWindowDuration() time.Duration {
	d, _ := time.ParseDuration(r.BatchWindow)
	if d == 0 {
		return 30 * time.Second
	}
	return d
}

// Load reads and parses a YAML config file.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	cfg.applyDefaults()

	if err := cfg.validate(); err != nil {
		return nil, err
	}

	// Expand ~ in paths.
	cfg.GasTown.Root = expandHome(cfg.GasTown.Root)
	cfg.StateFile = expandHome(cfg.StateFile)

	return &cfg, nil
}

// WebhookURL resolves the Slack webhook URL from environment.
func (c *Config) WebhookURL() string {
	if c.Slack.WebhookURLEnv != "" {
		return os.Getenv(c.Slack.WebhookURLEnv)
	}
	return ""
}

// BotToken resolves the Slack bot token from environment.
func (c *Config) BotToken() string {
	if c.Slack.TokenEnv != "" {
		return os.Getenv(c.Slack.TokenEnv)
	}
	return ""
}

// TownlogPath returns the path to the townlog file.
func (c *Config) TownlogPath() string {
	return c.GasTown.Root + "/logs/town.log"
}

// ChannelForRig returns the Slack channel for a given rig name.
func (c *Config) ChannelForRig(rig string) string {
	if ch, ok := c.Channels.Rigs[rig]; ok {
		return ch
	}
	return c.Channels.Default
}

// ShouldInclude returns whether an event type should be posted.
func (c *Config) ShouldInclude(eventType string) bool {
	if len(c.Filters.Townlog.Include) == 0 {
		return true // no filter = include all
	}
	for _, t := range c.Filters.Townlog.Include {
		if t == eventType {
			return true
		}
	}
	return false
}

func (c *Config) applyDefaults() {
	if c.Channels.Alerts == "" {
		c.Channels.Alerts = "#gt-alerts"
	}
	if c.Channels.Default == "" {
		c.Channels.Default = "#gt-ops"
	}
	if c.RateLimit.PerChannel == 0 {
		c.RateLimit.PerChannel = 10
	}
	if c.RateLimit.Burst == 0 {
		c.RateLimit.Burst = 5
	}
	if c.RateLimit.BatchWindow == "" {
		c.RateLimit.BatchWindow = "30s"
	}
	if c.StateFile == "" {
		c.StateFile = "state.json"
	}
}

func (c *Config) validate() error {
	if c.WebhookURL() == "" && c.BotToken() == "" {
		return fmt.Errorf("no Slack target: set %s or %s environment variable",
			c.Slack.WebhookURLEnv, c.Slack.TokenEnv)
	}
	if c.GasTown.Root == "" {
		return fmt.Errorf("gastown.root is required")
	}
	return nil
}

func expandHome(path string) string {
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return home + path[1:]
		}
	}
	return path
}
