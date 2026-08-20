package siteconfig

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

func TestLoad(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "sites.yml", `
settings:
  subject: "Website Update Notice"
  mention: "foobar@example.com"
sites:
  - name: "Example"
    check: "https://example.invalid/"
    link: "https://example.invalid/updates"
  - name: "Unnamed URL"
    check: "https://example.invalid/other"
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Settings.Subject != "Website Update Notice" {
		t.Errorf("Settings.Subject = %q, want %q", cfg.Settings.Subject, "Website Update Notice")
	}
	if cfg.Settings.Mention != "foobar@example.com" {
		t.Errorf("Settings.Mention = %q, want %q", cfg.Settings.Mention, "foobar@example.com")
	}
	if len(cfg.Sites) != 2 {
		t.Fatalf("len(Sites) = %d, want 2", len(cfg.Sites))
	}

	first := cfg.Sites[0]
	if first.Name != "Example" || first.Check != "https://example.invalid/" || first.Link != "https://example.invalid/updates" {
		t.Errorf("first site = %+v, want name/check/link to match", first)
	}
	if cfg.Sites[1].Link != cfg.Sites[1].Check {
		t.Errorf("second site Link = %q, want it to default to Check %q", cfg.Sites[1].Link, cfg.Sites[1].Check)
	}
}

func TestLoadDefaultsNameToCheck(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "sites.yml", `
sites:
  - check: "https://example.invalid/"
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Sites[0].Name != "https://example.invalid/" {
		t.Errorf("Sites[0].Name = %q, want it to default to the check URL", cfg.Sites[0].Name)
	}
}

func TestLoadOptionalSettingsDefaultEmpty(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "sites.yml", `
sites:
  - name: "Example"
    check: "https://example.invalid/"
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Settings.Subject != "" {
		t.Errorf("Settings.Subject = %q, want empty when omitted", cfg.Settings.Subject)
	}
	if cfg.Settings.Mention != "" {
		t.Errorf("Settings.Mention = %q, want empty when omitted", cfg.Settings.Mention)
	}
}

func TestLoadSettingsTimeout(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "config.yml", `
settings:
  timeout: 5s
sites:
  - check: "https://example.invalid/"
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Settings.Timeout != 5*time.Second {
		t.Errorf("Settings.Timeout = %s, want 5s", cfg.Settings.Timeout)
	}
}

func TestLoadMissingFile(t *testing.T) {
	_, err := Load(filepath.Join(t.TempDir(), "does-not-exist.yml"))
	if err == nil {
		t.Fatal("Load() error = nil, want error for missing file")
	}
}

func TestLoadInvalidYAML(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "sites.yml", "sites: [this is not valid: yaml:")

	_, err := Load(path)
	if err == nil {
		t.Fatal("Load() error = nil, want error for invalid YAML")
	}
}

func TestLoadEmptyFileNoSites(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "sites.yml", "")

	_, err := Load(path)
	if err == nil {
		t.Fatal("Load() error = nil, want error for no sites defined")
	}
}

func TestLoadMissingCheck(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "sites.yml", `
sites:
  - name: "Example"
`)

	_, err := Load(path)
	if err == nil {
		t.Fatal("Load() error = nil, want error for a site missing check")
	}
}
