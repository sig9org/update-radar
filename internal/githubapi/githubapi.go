// Package githubapi is a minimal client for the parts of the GitHub REST
// API github-rader needs: the latest release and the latest tag of a
// repository.
package githubapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// BaseURL is the root of the GitHub REST API. Overridable in tests.
var BaseURL = "https://api.github.com"

// Client talks to the GitHub REST API, optionally authenticated with a
// personal access token to raise the unauthenticated rate limit.
type Client struct {
	Token      string
	HTTPClient *http.Client
}

// NewClient builds a Client. An empty token makes unauthenticated
// requests.
func NewClient(token string) *Client {
	return &Client{Token: token, HTTPClient: http.DefaultClient}
}

// Release is the subset of a GitHub release the tool needs.
type Release struct {
	TagName string `json:"tag_name"`
	Name    string `json:"name"`
	HTMLURL string `json:"html_url"`
}

// Tag is the subset of a GitHub tag the tool needs.
type Tag struct {
	Name   string `json:"name"`
	Commit struct {
		SHA string `json:"sha"`
	} `json:"commit"`
}

// ParseOwnerRepo extracts "owner" and "repo" from a GitHub repository URL,
// e.g. "https://github.com/owner/repo" or "https://github.com/owner/repo/".
func ParseOwnerRepo(repoURL string) (owner, repo string, err error) {
	u, err := url.Parse(strings.TrimSpace(repoURL))
	if err != nil {
		return "", "", fmt.Errorf("githubapi: parse url %q: %w", repoURL, err)
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("githubapi: url %q is not a github.com/owner/repo url", repoURL)
	}
	return parts[0], strings.TrimSuffix(parts[1], ".git"), nil
}

// LatestRelease fetches the latest published (non-draft, non-prerelease)
// release of owner/repo. It returns ok=false if the repository has no
// releases (GitHub responds 404 for /releases/latest in that case).
func (c *Client) LatestRelease(ctx context.Context, owner, repo string) (release Release, ok bool, err error) {
	path := fmt.Sprintf("/repos/%s/%s/releases/latest", owner, repo)
	status, body, err := c.get(ctx, path)
	if err != nil {
		return Release{}, false, err
	}
	if status == http.StatusNotFound {
		return Release{}, false, nil
	}
	if status != http.StatusOK {
		return Release{}, false, fmt.Errorf("githubapi: get %s: unexpected status %d", path, status)
	}
	if err := json.Unmarshal(body, &release); err != nil {
		return Release{}, false, fmt.Errorf("githubapi: decode release for %s/%s: %w", owner, repo, err)
	}
	return release, true, nil
}

// LatestTag fetches the most recently created tag of owner/repo, as
// reported first by GitHub's /tags endpoint. It returns ok=false if the
// repository has no tags.
func (c *Client) LatestTag(ctx context.Context, owner, repo string) (tag Tag, ok bool, err error) {
	path := fmt.Sprintf("/repos/%s/%s/tags?per_page=1", owner, repo)
	status, body, err := c.get(ctx, path)
	if err != nil {
		return Tag{}, false, err
	}
	if status != http.StatusOK {
		return Tag{}, false, fmt.Errorf("githubapi: get %s: unexpected status %d", path, status)
	}
	var tags []Tag
	if err := json.Unmarshal(body, &tags); err != nil {
		return Tag{}, false, fmt.Errorf("githubapi: decode tags for %s/%s: %w", owner, repo, err)
	}
	if len(tags) == 0 {
		return Tag{}, false, nil
	}
	return tags[0], true, nil
}

func (c *Client) get(ctx context.Context, path string) (status int, body []byte, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, BaseURL+path, nil)
	if err != nil {
		return 0, nil, fmt.Errorf("githubapi: build request for %s: %w", path, err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}

	client := c.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("githubapi: request %s: %w", path, err)
	}
	defer resp.Body.Close()

	body, err = io.ReadAll(resp.Body)
	if err != nil {
		return 0, nil, fmt.Errorf("githubapi: read response for %s: %w", path, err)
	}
	return resp.StatusCode, body, nil
}
