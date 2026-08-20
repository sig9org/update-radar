// Package config loads and validates monitored Cisco sites.
package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sig9org/chatxgo/notify"
	"github.com/sig9org/update-radar/internal/model"
	"gopkg.in/yaml.v3"
)

const defaultTimeout = 45 * time.Second

// File is the application configuration document.
type File struct {
	Settings      Settings             `yaml:"settings"`
	Sites         []model.Site         `yaml:"sites"`
	Notifications NotificationSettings `yaml:"notifications"`
}

// Settings controls runtime behavior shared by all monitored sites.
type Settings struct {
	Mention    string        `yaml:"mention"`
	Separate   bool          `yaml:"separate"`
	Threads    int           `yaml:"threads"`
	Headless   bool          `yaml:"headless"`
	UserAgent  string        `yaml:"user-agent"`
	Timeout    time.Duration `yaml:"timeout"`
	Silent     bool          `yaml:"silent"`
	Debug      bool          `yaml:"debug"`
	threadsSet bool
}

// UnmarshalYAML accepts human-readable durations such as "45s".
func (s *Settings) UnmarshalYAML(value *yaml.Node) error {
	var raw struct {
		Mention   string `yaml:"mention"`
		Separate  bool   `yaml:"separate"`
		Threads   *int   `yaml:"threads"`
		Headless  bool   `yaml:"headless"`
		UserAgent string `yaml:"user-agent"`
		Timeout   string `yaml:"timeout"`
		Silent    bool   `yaml:"silent"`
		Debug     bool   `yaml:"debug"`
	}
	if err := value.Decode(&raw); err != nil {
		return err
	}
	s.Mention = raw.Mention
	s.Separate = raw.Separate
	s.Threads = 1
	if raw.Threads != nil {
		s.Threads = *raw.Threads
		s.threadsSet = true
	}
	s.Headless = raw.Headless
	s.UserAgent = raw.UserAgent
	s.Silent = raw.Silent
	s.Debug = raw.Debug
	s.Timeout = defaultTimeout
	if strings.TrimSpace(raw.Timeout) != "" {
		timeout, err := time.ParseDuration(strings.TrimSpace(raw.Timeout))
		if err != nil {
			return fmt.Errorf("settings.timeout must be a valid duration: %w", err)
		}
		s.Timeout = timeout
	}
	return nil
}

// NotificationSettings contains the former config.ini notification values.
type NotificationSettings struct {
	Proxy string `yaml:"proxy"`
	Teams struct {
		Destination string `yaml:"destination"`
	} `yaml:"teams"`
	Webex struct {
		Token       string `yaml:"token"`
		Destination string `yaml:"destination"`
	} `yaml:"webex"`
	Slack struct {
		Destination string `yaml:"destination"`
		Token       string `yaml:"token"`
		Channel     string `yaml:"channel"`
	} `yaml:"slack"`
}

// ChatConfig converts the YAML notification settings to chatxgo's config.
func (s NotificationSettings) ChatConfig() notify.Config {
	return notify.Config{
		Proxy: s.Proxy,
		Teams: notify.TeamsConfig{Dest: s.Teams.Destination},
		Webex: notify.WebexConfig{Token: s.Webex.Token, Dest: s.Webex.Destination},
		Slack: notify.SlackConfig{Dest: s.Slack.Destination, Token: s.Slack.Token, Channel: s.Slack.Channel},
	}
}

// ResolveConfigPath returns an explicitly requested path, or config.yml.
func ResolveConfigPath(explicit string) (string, error) {
	if strings.TrimSpace(explicit) != "" {
		return explicit, nil
	}
	if _, err := os.Stat("config.yml"); err == nil {
		return "config.yml", nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("inspect config file %q: %w", "config.yml", err)
	}
	return "", errors.New("no config file found (looked for config.yml)")
}

// StatePath derives the state file path from the config file path.
func StatePath(configPath string) string {
	ext := filepath.Ext(configPath)
	base := strings.TrimSuffix(filepath.Base(configPath), ext)
	return filepath.Join(filepath.Dir(configPath), base+"_state"+ext)
}

// Load reads and validates a config file.
func Load(path string) (File, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return File{}, fmt.Errorf("read config file %q: %w", path, err)
	}
	var cfg File
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return File{}, fmt.Errorf("parse config file %q: %w", path, err)
	}
	if cfg.Settings.Timeout == 0 {
		cfg.Settings.Timeout = defaultTimeout
	}
	if cfg.Settings.Threads == 0 && !cfg.Settings.threadsSet {
		cfg.Settings.Threads = 1
	}
	if cfg.Settings.Threads <= 0 {
		return File{}, errors.New("settings.threads must be greater than zero")
	}
	cfg.Settings.Mention = strings.TrimSpace(cfg.Settings.Mention)
	cfg.Settings.UserAgent = strings.TrimSpace(cfg.Settings.UserAgent)
	cfg.Notifications.Proxy = strings.TrimSpace(cfg.Notifications.Proxy)
	cfg.Notifications.Teams.Destination = strings.TrimSpace(cfg.Notifications.Teams.Destination)
	cfg.Notifications.Webex.Token = strings.TrimSpace(cfg.Notifications.Webex.Token)
	cfg.Notifications.Webex.Destination = strings.TrimSpace(cfg.Notifications.Webex.Destination)
	cfg.Notifications.Slack.Destination = strings.TrimSpace(cfg.Notifications.Slack.Destination)
	cfg.Notifications.Slack.Token = strings.TrimSpace(cfg.Notifications.Slack.Token)
	cfg.Notifications.Slack.Channel = strings.TrimSpace(cfg.Notifications.Slack.Channel)
	if !cfg.Notifications.HasDestination() {
		return File{}, errors.New("config file contains no notification destination")
	}
	if len(cfg.Sites) == 0 {
		return File{}, errors.New("config file contains no sites")
	}
	seen := make(map[string]struct{}, len(cfg.Sites))
	for i := range cfg.Sites {
		site := &cfg.Sites[i]
		site.Name = strings.TrimSpace(site.Name)
		site.URL = strings.TrimSpace(site.URL)
		parsed, parseErr := url.ParseRequestURI(site.URL)
		if parseErr != nil || parsed.Scheme != "https" || parsed.Host == "" {
			return File{}, fmt.Errorf("sites[%d].url must be a valid HTTPS URL", i)
		}
		if site.Name == "" {
			site.Name = site.URL
		}
		if _, exists := seen[site.URL]; exists {
			return File{}, fmt.Errorf("duplicate site URL %q", site.URL)
		}
		seen[site.URL] = struct{}{}
	}
	return cfg, nil
}

// HasDestination reports whether at least one notification destination is
// configured. Credentials are validated by the notification sender itself.
func (s NotificationSettings) HasDestination() bool {
	return s.Teams.Destination != "" || s.Webex.Destination != "" || s.Slack.Destination != ""
}
