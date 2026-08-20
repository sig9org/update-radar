// Package siteconfig loads the config.yml file describing
// which URLs web-rader should check for changes.
package siteconfig

import (
	"fmt"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Site is a single URL to check for changes and an optional notification link.
type Site struct {
	// Name is a human-readable label for the site, used in notifications
	// and debug output. Defaults to Check when left blank.
	Name string `yaml:"name"`
	// Check is the address to fetch and check for changes.
	Check string `yaml:"check"`
	// Link is the address included in notifications. Defaults to Check.
	Link string `yaml:"link"`
}

// Settings holds the top-level "settings" block of a config.yml file. All
// fields are optional.
type Settings struct {
	// Subject overrides the default chat notification title when set.
	Subject string `yaml:"subject"`
	// Mention lists who to mention in the chat notification, as one or
	// more comma-separated "id" or "id:label" values (the same format
	// chatxgo's own "-mention" flag accepts). Left empty, no one is
	// mentioned.
	Mention string `yaml:"mention"`
	// Timeout is the maximum duration of one site fetch.
	Timeout time.Duration `yaml:"-"`
}

// UnmarshalYAML parses the human-friendly duration used by settings.timeout.
func (s *Settings) UnmarshalYAML(value *yaml.Node) error {
	var raw struct {
		Subject string `yaml:"subject"`
		Mention string `yaml:"mention"`
		Timeout string `yaml:"timeout"`
	}
	if err := value.Decode(&raw); err != nil {
		return err
	}
	s.Subject, s.Mention = raw.Subject, raw.Mention
	if strings.TrimSpace(raw.Timeout) == "" {
		return nil
	}
	timeout, err := time.ParseDuration(raw.Timeout)
	if err != nil {
		return fmt.Errorf("invalid timeout %q: %w", raw.Timeout, err)
	}
	if timeout <= 0 {
		return fmt.Errorf("timeout must be positive")
	}
	s.Timeout = timeout
	return nil
}

// Notifications contains the chat destinations in config.yml.
type Notifications struct {
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

// Config is the parsed content of a config.yml file.
type Config struct {
	// Settings holds tool-wide configuration.
	Settings      Settings      `yaml:"settings"`
	Notifications Notifications `yaml:"notifications"`
	// Sites lists the URLs to check.
	Sites []Site `yaml:"sites"`
}

// Load reads and parses the config.yml file at path.
func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("siteconfig: read %s: %w", path, err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("siteconfig: parse %s: %w", path, err)
	}
	if len(cfg.Sites) == 0 {
		return Config{}, fmt.Errorf("siteconfig: parse %s: no sites defined", path)
	}
	for i, s := range cfg.Sites {
		if strings.TrimSpace(s.Check) == "" {
			return Config{}, fmt.Errorf("siteconfig: parse %s: sites[%d]: check is required", path, i)
		}
		if strings.TrimSpace(s.Name) == "" {
			cfg.Sites[i].Name = s.Check
		}
		if strings.TrimSpace(s.Link) == "" {
			cfg.Sites[i].Link = s.Check
		}
	}
	return cfg, nil
}
