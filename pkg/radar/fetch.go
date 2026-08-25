package radar

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/sig9org/update-radar/internal/githubapi"
	"github.com/sig9org/update-radar/internal/model"
	"github.com/sig9org/update-radar/internal/scraper"
	"github.com/sig9org/update-radar/internal/webclient"
)

// CiscoSite identifies a Cisco software download page.
type CiscoSite struct {
	Name string
	URL  string
}

// CiscoOptions controls Cisco page rendering.
type CiscoOptions struct {
	Timeout   time.Duration
	Headless  bool
	UserAgent string
}

// CiscoSnapshot is the current release information found on a Cisco page.
// Suggested, Latest, and Deferred contain both parent and expanded child
// release versions.
type CiscoSnapshot struct {
	ProductName string
	Suggested   []string
	Latest      []string
	Deferred    []string
	FetchedAt   time.Time
}

// FetchCisco retrieves Suggested Release, Latest Release, and Deferred
// Release information from one Cisco software page.
func FetchCisco(ctx context.Context, site CiscoSite, opts CiscoOptions) (CiscoSnapshot, error) {
	if strings.TrimSpace(site.URL) == "" {
		return CiscoSnapshot{}, fmt.Errorf("radar: Cisco URL is required")
	}
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = 45 * time.Second
	}
	pool, err := scraper.OpenPool(ctx, scraper.Options{
		Timeout: timeout, Headless: opts.Headless, UserAgent: opts.UserAgent,
	}, 1)
	if err != nil {
		return CiscoSnapshot{}, fmt.Errorf("radar: open Cisco browser: %w", err)
	}
	defer pool.Close()
	snapshot, err := pool.Fetch(ctx, model.Site{Name: site.Name, URL: site.URL})
	if err != nil {
		return CiscoSnapshot{}, fmt.Errorf("radar: fetch Cisco page %q: %w", site.URL, err)
	}
	return CiscoSnapshot{
		ProductName: snapshot.ProductName,
		Suggested:   append([]string(nil), snapshot.Suggested...),
		Latest:      append([]string(nil), snapshot.Latest...),
		Deferred:    append([]string(nil), snapshot.Deferred...),
		FetchedAt:   snapshot.FetchedAt,
	}, nil
}

// GitHubOptions controls GitHub API access. If both CheckReleases and
// CheckTags are false, both kinds of information are fetched.
type GitHubOptions struct {
	Token         string
	HTTPClient    *http.Client
	CheckReleases bool
	CheckTags     bool
}

// GitHubRelease is the latest published release of a repository.
type GitHubRelease struct {
	TagName string
	Name    string
	HTMLURL string
}

// GitHubTag is the most recently created tag of a repository.
type GitHubTag struct {
	Name      string
	CommitSHA string
}

// GitHubSnapshot is the current release and tag information for a repository.
// A nil Release or Tag means that the requested kind was not found or was not
// requested.
type GitHubSnapshot struct {
	Repository string
	Release    *GitHubRelease
	Tag        *GitHubTag
}

// FetchGitHub retrieves the latest release and/or tag information for a
// GitHub repository URL.
func FetchGitHub(ctx context.Context, repositoryURL string, opts GitHubOptions) (GitHubSnapshot, error) {
	owner, repo, err := githubapi.ParseOwnerRepo(repositoryURL)
	if err != nil {
		return GitHubSnapshot{}, err
	}
	checkReleases, checkTags := opts.CheckReleases, opts.CheckTags
	if !checkReleases && !checkTags {
		checkReleases, checkTags = true, true
	}
	client := &githubapi.Client{Token: opts.Token, HTTPClient: opts.HTTPClient}
	result := GitHubSnapshot{Repository: owner + "/" + repo}
	if checkReleases {
		release, ok, err := client.LatestRelease(ctx, owner, repo)
		if err != nil {
			return GitHubSnapshot{}, fmt.Errorf("radar: fetch GitHub release: %w", err)
		}
		if ok {
			result.Release = &GitHubRelease{TagName: release.TagName, Name: release.Name, HTMLURL: release.HTMLURL}
		}
	}
	if checkTags {
		tag, ok, err := client.LatestTag(ctx, owner, repo)
		if err != nil {
			return GitHubSnapshot{}, fmt.Errorf("radar: fetch GitHub tag: %w", err)
		}
		if ok {
			result.Tag = &GitHubTag{Name: tag.Name, CommitSHA: tag.Commit.SHA}
		}
	}
	return result, nil
}

// WebOptions controls web page retrieval.
type WebOptions struct {
	Timeout    time.Duration
	HTTPClient *http.Client
}

// WebSnapshot is the SHA-256 observation of a web page.
type WebSnapshot struct {
	URL    string
	SHA256 string
}

// FetchWeb retrieves a web page and returns its SHA-256 content digest.
func FetchWeb(ctx context.Context, url string, opts WebOptions) (WebSnapshot, error) {
	if strings.TrimSpace(url) == "" {
		return WebSnapshot{}, fmt.Errorf("radar: web URL is required")
	}
	client := webclient.NewClient(opts.Timeout)
	if opts.HTTPClient != nil {
		client.HTTPClient = opts.HTTPClient
	}
	digest, err := client.Fetch(ctx, url)
	if err != nil {
		return WebSnapshot{}, fmt.Errorf("radar: fetch web page %q: %w", url, err)
	}
	return WebSnapshot{URL: url, SHA256: digest}, nil
}
