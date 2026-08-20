// Package repoconfig loads config.yml, describing GitHub repositories and
// chat destinations for github-rader.
package repoconfig

import (
	"fmt"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Repository is a single GitHub repository to watch.
type Repository struct {
	// Name is a human-readable label for the repository, used in
	// notifications and debug output.
	Name string `yaml:"name"`
	// URL is the GitHub repository URL, e.g. "https://github.com/owner/repo".
	URL string `yaml:"url"`
	// CheckReleases enables checking for new GitHub releases.
	CheckReleases bool `yaml:"check_releases"`
	// CheckTags enables checking for new GitHub tags.
	CheckTags bool `yaml:"check_tags"`
}

// Settings holds the top-level "settings" block of config.yml.
type Settings struct {
	// GitHubToken authenticates GitHub API requests, raising the low
	// unauthenticated rate limit. It is required. The YAML key is spelled
	// "github_tokoen" to match the field name used across github-rader's
	// configuration files.
	GitHubToken string `yaml:"github_tokoen"`
	// Mention lists who to mention in the chat notification. It is kept as a
	// comma-separated string internally for compatibility with the notifier;
	// config.yml accepts either a string or a YAML list of strings.
	Mention string        `yaml:"mention"`
	Timeout time.Duration `yaml:"-"`
}

// UnmarshalYAML accepts human-readable durations such as "15s".
func (s *Settings) UnmarshalYAML(value *yaml.Node) error {
	var raw struct {
		GitHubToken string    `yaml:"github_tokoen"`
		Mention     yaml.Node `yaml:"mention"`
		Timeout     string    `yaml:"timeout"`
	}
	if err := value.Decode(&raw); err != nil {
		return err
	}
	mention, err := parseMentionValue(raw.Mention)
	if err != nil {
		return err
	}
	*s = Settings{GitHubToken: raw.GitHubToken, Mention: mention}
	if strings.TrimSpace(raw.Timeout) != "" {
		d, err := time.ParseDuration(raw.Timeout)
		if err != nil {
			return fmt.Errorf("settings.timeout: %w", err)
		}
		if d <= 0 {
			return fmt.Errorf("settings.timeout must be greater than zero")
		}
		s.Timeout = d
	}
	return nil
}

func parseMentionValue(node yaml.Node) (string, error) {
	if node.Kind == 0 {
		return "", nil
	}
	if node.Kind == yaml.ScalarNode {
		var value string
		if err := node.Decode(&value); err != nil {
			return "", err
		}
		return value, nil
	}
	if node.Kind != yaml.SequenceNode {
		return "", fmt.Errorf("settings.mention must be a string or list of strings")
	}
	values := make([]string, 0, len(node.Content))
	for _, item := range node.Content {
		if item.Kind != yaml.ScalarNode {
			return "", fmt.Errorf("settings.mention must be a list of strings")
		}
		var value string
		if err := item.Decode(&value); err != nil {
			return "", err
		}
		values = append(values, value)
	}
	return strings.Join(values, ","), nil
}

// ChatSettings is the YAML representation of chatxgo's notify.Config.
type ChatSettings struct {
	Webex struct {
		Token string `yaml:"token"`
		Dest  string `yaml:"destination"`
	} `yaml:"webex"`
	Teams struct {
		Dest string `yaml:"destination"`
	} `yaml:"teams"`
	Slack struct {
		Dest    string `yaml:"destination"`
		Token   string `yaml:"token"`
		Channel string `yaml:"channel"`
	} `yaml:"slack"`
	Proxy string `yaml:"proxy"`
}

// Config is the parsed content of config.yml.
type Config struct {
	// Settings holds tool-wide configuration.
	Settings      Settings     `yaml:"settings"`
	Notifications ChatSettings `yaml:"notifications"`
	// Repositories lists the GitHub repositories to watch.
	Repositories []Repository `yaml:"sites"`
}

// Load reads and parses the config.yml file at path.
func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("repoconfig: read %s: %w", path, err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("repoconfig: parse %s: %w", path, err)
	}
	if strings.TrimSpace(cfg.Settings.GitHubToken) == "" {
		return Config{}, fmt.Errorf("repoconfig: parse %s: settings.github_tokoen is required", path)
	}
	return cfg, nil
}
