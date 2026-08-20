package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStatePath(t *testing.T) {
	tests := map[string]string{
		"config.yml":                   "config_state.yml",
		"config.yaml":                  "config_state.yaml",
		filepath.Join("etc", "my.yml"): filepath.Join("etc", "my_state.yml"),
	}
	for input, want := range tests {
		if got := StatePath(input); got != want {
			t.Errorf("StatePath(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yml")
	data := []byte("settings:\n  mention: ' user@example.com '\n  separate: true\n  threads: 2\n  headless: true\n  user-agent: Custom Agent\n  timeout: 1m30s\n  silent: true\n  debug: true\nnotifications:\n  teams:\n    destination: https://example.com/teams\nsites:\n  - name: Test Product\n    url: https://software.cisco.com/example\n")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Sites) != 1 || cfg.Sites[0].Name != "Test Product" {
		t.Fatalf("unexpected config: %#v", cfg)
	}
	if cfg.Settings.Mention != "user@example.com" {
		t.Fatalf("mention = %q", cfg.Settings.Mention)
	}
	if !cfg.Settings.Separate || cfg.Settings.Threads != 2 || !cfg.Settings.Headless || cfg.Settings.UserAgent != "Custom Agent" || cfg.Settings.Timeout != 90*time.Second || !cfg.Settings.Silent || !cfg.Settings.Debug {
		t.Fatalf("unexpected runtime settings: %#v", cfg.Settings)
	}
	if cfg.Notifications.ChatConfig().Teams.Dest != "https://example.com/teams" {
		t.Fatalf("unexpected notification settings: %#v", cfg.Notifications)
	}
}

func TestLoadRejectsNonHTTPSURL(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yml")
	if err := os.WriteFile(path, []byte("sites:\n  - url: http://example.com\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("Load accepted a non-HTTPS URL")
	}
}

func TestLoadRejectsMissingNotificationDestination(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yml")
	data := []byte("sites:\n  - url: https://software.cisco.com/example\n")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("Load accepted a config without notification destinations")
	}
}

func TestLoadRejectsMissingSites(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yml")
	data := []byte("notifications:\n  teams:\n    destination: https://example.com/teams\n")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("Load accepted a config without sites")
	}
}

func TestLoadDefaultsThreadsToOne(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yml")
	data := []byte("notifications:\n  teams:\n    destination: https://example.com/teams\nsites:\n  - url: https://software.cisco.com/example\n")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Settings.Threads != 1 {
		t.Fatalf("threads = %d, want 1", cfg.Settings.Threads)
	}
}

func TestLoadRejectsZeroThreads(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yml")
	data := []byte("settings:\n  threads: 0\nnotifications:\n  teams:\n    destination: https://example.com/teams\nsites:\n  - url: https://software.cisco.com/example\n")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("Load accepted zero threads")
	}
}
