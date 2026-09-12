package appconfig

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sig9org/update-radar/internal/feed"
	"github.com/sig9org/update-radar/internal/repoconfig"
	"github.com/sig9org/update-radar/internal/siteconfig"
	"gopkg.in/yaml.v3"
)

type Settings struct {
	Debug       bool          `yaml:"debug"`
	Silent      bool          `yaml:"silent"`
	Separate    bool          `yaml:"separate"`
	Headless    bool          `yaml:"headless"`
	Threads     int           `yaml:"threads"`
	Timeout     time.Duration `yaml:"-"`
	UserAgent   string        `yaml:"user-agent"`
	Mention     []string      `yaml:"mention"`
	GitHubToken string        `yaml:"github_tokoen"`
}

func (s *Settings) UnmarshalYAML(n *yaml.Node) error {
	var raw struct {
		Debug       bool      `yaml:"debug"`
		Silent      bool      `yaml:"silent"`
		Separate    bool      `yaml:"separate"`
		Headless    bool      `yaml:"headless"`
		Threads     int       `yaml:"threads"`
		Timeout     string    `yaml:"timeout"`
		UserAgent   string    `yaml:"user-agent"`
		GitHubToken string    `yaml:"github_tokoen"`
		Mention     yaml.Node `yaml:"mention"`
	}
	if err := n.Decode(&raw); err != nil {
		return err
	}
	s.Debug, s.Silent, s.Separate, s.Headless = raw.Debug, raw.Silent, raw.Separate, raw.Headless
	s.Threads, s.UserAgent, s.GitHubToken = raw.Threads, raw.UserAgent, raw.GitHubToken
	if s.Threads == 0 {
		s.Threads = 1
	}
	if s.Threads < 0 {
		return fmt.Errorf("settings.threads must not be negative")
	}
	if strings.TrimSpace(raw.Timeout) != "" {
		d, err := time.ParseDuration(raw.Timeout)
		if err != nil || d <= 0 {
			return fmt.Errorf("settings.timeout must be a positive duration")
		}
		s.Timeout = d
	}
	if raw.Mention.Kind == yaml.SequenceNode {
		for _, item := range raw.Mention.Content {
			var v string
			if err := item.Decode(&v); err != nil {
				return err
			}
			s.Mention = append(s.Mention, v)
		}
	} else if raw.Mention.Kind != 0 {
		var v string
		if err := raw.Mention.Decode(&v); err != nil {
			return err
		}
		if v != "" {
			s.Mention = []string{v}
		}
	}
	return nil
}

type Notification struct {
	Proxy   string `yaml:"proxy"`
	Discord struct {
		Destination string   `yaml:"destination"`
		Mention     []string `yaml:"mention"`
	} `yaml:"discord"`
	Email struct {
		SMTPHost     string   `yaml:"smtp_host"`
		SMTPPort     int      `yaml:"smtp_port"`
		SMTPUsername string   `yaml:"smtp_username"`
		SMTPPassword string   `yaml:"smtp_password"`
		From         string   `yaml:"from"`
		To           []string `yaml:"to"`
		Cc           []string `yaml:"cc"`
		Bcc          []string `yaml:"bcc"`
	} `yaml:"email"`
	Slack struct {
		Destination, Token, Channel string
		Mention                     []string `yaml:"mention"`
	} `yaml:"slack"`
	Teams struct {
		Destination string   `yaml:"destination"`
		Mention     []string `yaml:"mention"`
	} `yaml:"teams"`
	Webex struct {
		Token, Destination string
		Mention            []string `yaml:"mention"`
	} `yaml:"webex"`
}
type CiscoSite struct {
	Name string `yaml:"name"`
	URL  string `yaml:"url"`
}
type Profile struct {
	Notifications Notification            `yaml:"notifications"`
	Cisco         []CiscoSite             `yaml:"cisco"`
	GitHub        []repoconfig.Repository `yaml:"github"`
	Web           []siteconfig.Site       `yaml:"web"`
	Feed          []feed.Site             `yaml:"feed"`
}
type File struct {
	Settings Settings           `yaml:"settings"`
	Profiles map[string]Profile `yaml:"profiles"`
}

func Load(path string) (File, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return File{}, fmt.Errorf("read config %q: %w", path, err)
	}
	var c File
	if err := yaml.Unmarshal(data, &c); err != nil {
		return File{}, fmt.Errorf("parse config %q: %w", path, err)
	}
	if len(c.Profiles) == 0 {
		return File{}, fmt.Errorf("config %q contains no profiles", path)
	}
	for name, profile := range c.Profiles {
		for i, item := range profile.Feed {
			parsed, err := url.ParseRequestURI(strings.TrimSpace(item.URL))
			if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
				return File{}, fmt.Errorf("profile %q feed[%d].url must be a valid HTTPS URL", name, i)
			}
			c.Profiles[name].Feed[i].URL = strings.TrimSpace(item.URL)
			if strings.TrimSpace(item.Name) == "" {
				c.Profiles[name].Feed[i].Name = item.URL
			}
		}
	}
	return c, nil
}
func Resolve(path string) (string, error) {
	if path != "" {
		return path, nil
	}
	return "config.yml", nil
}
func StatePath(path string) string {
	ext := filepath.Ext(path)
	return filepath.Join(filepath.Dir(path), strings.TrimSuffix(filepath.Base(path), ext)+"_state"+ext)
}
func (p Profile) Mentions(s Settings) string { return strings.Join(s.Mention, ",") }
