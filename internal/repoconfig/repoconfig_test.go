package repoconfig

import (
	"os"
	"path/filepath"
	"testing"
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
	path := writeFile(t, dir, "sites.yaml", `
settings:
  github_tokoen: "ghp_example"
  mention: "foobar@example.com,U0123456:Someone"
sites:
  - name: "Example"
    url: "https://github.com/example/repo"
    check_releases: true
    check_tags: false
  - name: "TagsOnly"
    url: "https://github.com/example/tags-only"
    check_releases: false
    check_tags: true
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Settings.GitHubToken != "ghp_example" {
		t.Errorf("Settings.GitHubToken = %q, want %q", cfg.Settings.GitHubToken, "ghp_example")
	}
	if cfg.Settings.Mention != "foobar@example.com,U0123456:Someone" {
		t.Errorf("Settings.Mention = %q, want %q", cfg.Settings.Mention, "foobar@example.com,U0123456:Someone")
	}
	if len(cfg.Repositories) != 2 {
		t.Fatalf("len(Repositories) = %d, want 2", len(cfg.Repositories))
	}

	first := cfg.Repositories[0]
	if first.Name != "Example" || first.URL != "https://github.com/example/repo" {
		t.Errorf("first repository = %+v, want name/url to match", first)
	}
	if !first.CheckReleases || first.CheckTags {
		t.Errorf("first repository flags = releases:%v tags:%v, want releases:true tags:false", first.CheckReleases, first.CheckTags)
	}

	second := cfg.Repositories[1]
	if second.CheckReleases || !second.CheckTags {
		t.Errorf("second repository flags = releases:%v tags:%v, want releases:false tags:true", second.CheckReleases, second.CheckTags)
	}
}

func TestLoadMissingFile(t *testing.T) {
	_, err := Load(filepath.Join(t.TempDir(), "does-not-exist.yaml"))
	if err == nil {
		t.Fatal("Load() error = nil, want error for missing file")
	}
}

func TestLoadInvalidYAML(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "sites.yaml", "sites: [this is not valid: yaml:")

	_, err := Load(path)
	if err == nil {
		t.Fatal("Load() error = nil, want error for invalid YAML")
	}
}

func TestLoadEmptyFileMissingToken(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "sites.yaml", "")

	_, err := Load(path)
	if err == nil {
		t.Fatal("Load() error = nil, want error for missing settings.github_tokoen")
	}
}

func TestLoadMissingGitHubToken(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "sites.yaml", `
settings:
  mention: "no-token@example.com"
sites:
  - name: "Example"
    url: "https://github.com/example/repo"
    check_releases: true
`)

	_, err := Load(path)
	if err == nil {
		t.Fatal("Load() error = nil, want error for missing settings.github_tokoen")
	}
}

func TestLoadOptionalSettingsDefaultEmpty(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "sites.yaml", `
settings:
  github_tokoen: "ghp_example"
sites:
  - name: "Example"
    url: "https://github.com/example/repo"
    check_releases: true
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Settings.Mention != "" {
		t.Errorf("Settings.Mention = %q, want empty when omitted", cfg.Settings.Mention)
	}
}
