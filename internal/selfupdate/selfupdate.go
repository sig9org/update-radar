// Package selfupdate updates github-rader from its GitHub release assets.
package selfupdate

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/Masterminds/semver/v3"
	"github.com/sig9org/update-radar/internal/debugx"
)

// Repository is "owner/repo", where release binaries are published.
const Repository = "sig9org/update-radar"

type release struct {
	TagName string  `json:"tag_name"`
	Assets  []asset `json:"assets"`
}

type asset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
}

// Update checks GitHub for a newer release and replaces the running binary.
func Update(ctx context.Context, currentVersion string) (string, error) {
	debugx.Printf("checking %s for a release newer than %s", Repository, currentVersion)

	release, err := latestRelease(ctx)
	if err != nil {
		return "", fmt.Errorf("selfupdate: detect latest release: %w", err)
	}
	asset, err := release.asset()
	if err != nil {
		return "", err
	}
	debugx.Printf("latest release found: %s (%s)", release.TagName, asset.Name)

	if strings.TrimSpace(currentVersion) != "" && currentVersion != "dev" {
		latestVersion, latestErr := semver.NewVersion(release.TagName)
		current, currentErr := semver.NewVersion(currentVersion)
		if latestErr == nil && currentErr == nil && !latestVersion.GreaterThan(current) {
			return fmt.Sprintf("already up to date (current %s, latest %s)", currentVersion, release.TagName), nil
		}
	}

	executable, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("selfupdate: locate running executable: %w", err)
	}
	debugx.Printf("updating executable at %s", executable)
	if err := replaceExecutable(ctx, executable, asset.URL); err != nil {
		return "", fmt.Errorf("selfupdate: update failed: %w", err)
	}
	return fmt.Sprintf("updated to version %s", release.TagName), nil
}

func latestRelease(ctx context.Context) (release, error) {
	url := "https://api.github.com/repos/" + Repository + "/releases/latest"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return release{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return release{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return release{}, fmt.Errorf("GitHub returned status %s", resp.Status)
	}
	var result release
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return release{}, err
	}
	return result, nil
}

func (r release) asset() (asset, error) {
	platform := "_" + runtime.GOOS + "_" + runtime.GOARCH
	for _, candidate := range r.Assets {
		if strings.Contains(candidate.Name, platform) {
			return candidate, nil
		}
	}
	return asset{}, fmt.Errorf("selfupdate: no release asset for %s/%s", runtime.GOOS, runtime.GOARCH)
}

func replaceExecutable(ctx context.Context, executable, assetURL string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, assetURL, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download returned status %s", resp.Status)
	}

	tmp, err := os.CreateTemp(filepath.Dir(executable), ".github-rader-update-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := io.Copy(tmp, resp.Body); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o755); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, executable)
}
