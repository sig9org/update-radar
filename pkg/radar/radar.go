// Package radar provides the embeddable update-radar monitoring API.
package radar

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/sig9org/update-radar/internal/appconfig"
	ciscostate "github.com/sig9org/update-radar/internal/ciscostate"
	"github.com/sig9org/update-radar/internal/diff"
	"github.com/sig9org/update-radar/internal/githubapi"
	githubstate "github.com/sig9org/update-radar/internal/githubstate"
	"github.com/sig9org/update-radar/internal/model"
	"github.com/sig9org/update-radar/internal/monitor"
	"github.com/sig9org/update-radar/internal/repoconfig"
	"github.com/sig9org/update-radar/internal/scraper"
	"github.com/sig9org/update-radar/internal/sentinel"
	"github.com/sig9org/update-radar/internal/siteconfig"
	"github.com/sig9org/update-radar/internal/webclient"
	webstate "github.com/sig9org/update-radar/internal/webstate"
	"gopkg.in/yaml.v3"
)

// Target identifies one category of monitored resource.
type Target string

const (
	Cisco  Target = "cisco"
	GitHub Target = "github"
	Web    Target = "web"
)

// Options controls an embedded check. An empty Targets slice checks all
// categories configured in the selected profiles.
type Options struct {
	ConfigPath string
	Targets    []Target
	ReadOnly   bool
}

// Result summarizes the check. Updated is true when at least one monitored
// resource changed since its previous saved observation.
type Result struct {
	Updated       bool
	CiscoUpdates  int
	GitHubUpdates int
	WebUpdates    int
}

type stateFile struct {
	Cisco  ciscostate.File   `yaml:"cisco"`
	GitHub githubstate.State `yaml:"github"`
	Web    webstate.State    `yaml:"web"`
}

// Check loads the configured profiles and checks the requested categories.
// It does not send notifications. By default it persists the new observations;
// set ReadOnly to true to make the call read-only.
func Check(ctx context.Context, opts Options) (Result, error) {
	path, err := appconfig.Resolve(opts.ConfigPath)
	if err != nil {
		return Result{}, err
	}
	cfg, err := appconfig.Load(path)
	if err != nil {
		return Result{}, err
	}
	st, err := loadState(appconfig.StatePath(path))
	if err != nil {
		return Result{}, err
	}
	selected := func(t Target) bool {
		if len(opts.Targets) == 0 {
			return true
		}
		for _, x := range opts.Targets {
			if strings.EqualFold(string(x), string(t)) {
				return true
			}
		}
		return false
	}
	result := Result{}
	for _, profile := range cfg.Profiles {
		if selected(Cisco) {
			n, err := checkCisco(ctx, cfg.Settings, profile.Cisco, &st)
			if err != nil {
				return result, err
			}
			result.CiscoUpdates += n
		}
		if selected(GitHub) {
			n, next := checkGitHub(ctx, cfg.Settings, profile.GitHub, st.GitHub)
			st.GitHub = next
			result.GitHubUpdates += n
		}
		if selected(Web) {
			n, next := checkWeb(ctx, cfg.Settings, profile.Web, st.Web)
			st.Web = next
			result.WebUpdates += n
		}
	}
	result.Updated = result.CiscoUpdates+result.GitHubUpdates+result.WebUpdates > 0
	if !opts.ReadOnly {
		if err := saveState(appconfig.StatePath(path), st); err != nil {
			return result, err
		}
	}
	return result, nil
}

func loadState(path string) (stateFile, error) {
	st := stateFile{Cisco: ciscostate.File{Sites: map[string]model.Snapshot{}}, GitHub: githubstate.State{Repositories: map[string]githubstate.RepoState{}}, Web: webstate.State{Sites: map[string]webstate.SiteState{}}}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return st, nil
	}
	if err != nil {
		return st, fmt.Errorf("read state %q: %w", path, err)
	}
	if err := yaml.Unmarshal(data, &st); err != nil {
		return st, fmt.Errorf("parse state %q: %w", path, err)
	}
	if st.Cisco.Sites == nil {
		st.Cisco.Sites = map[string]model.Snapshot{}
	}
	if st.GitHub.Repositories == nil {
		st.GitHub.Repositories = map[string]githubstate.RepoState{}
	}
	if st.Web.Sites == nil {
		st.Web.Sites = map[string]webstate.SiteState{}
	}
	return st, nil
}

func saveState(path string, st stateFile) error {
	var buf bytes.Buffer
	encoder := yaml.NewEncoder(&buf)
	encoder.SetIndent(2)
	err := encoder.Encode(st)
	closeErr := encoder.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0600)
}

func checkGitHub(ctx context.Context, s appconfig.Settings, repos []repoconfig.Repository, st githubstate.State) (int, githubstate.State) {
	cfg := repoconfig.Config{Settings: repoconfig.Settings{GitHubToken: s.GitHubToken}, Repositories: repos}
	events, next := monitor.CheckRepositories(ctx, githubapi.NewClient(s.GitHubToken), cfg, st)
	return len(events), next
}

func checkWeb(ctx context.Context, s appconfig.Settings, sites []siteconfig.Site, st webstate.State) (int, webstate.State) {
	cfg := siteconfig.Config{Sites: sites}
	events, next := sentinel.CheckSites(ctx, webclient.NewClient(s.Timeout), cfg, st)
	return len(events), next
}

func checkCisco(ctx context.Context, s appconfig.Settings, sites []appconfig.CiscoSite, st *stateFile) (int, error) {
	if len(sites) == 0 {
		return 0, nil
	}
	timeout := s.Timeout
	if timeout == 0 {
		timeout = 45 * time.Second
	}
	pool, err := scraper.OpenPool(ctx, scraper.Options{Timeout: timeout, Headless: s.Headless, UserAgent: s.UserAgent}, s.Threads)
	if err != nil {
		return 0, err
	}
	defer pool.Close()
	updates := 0
	for _, item := range sites {
		site := model.Site{Name: item.Name, URL: item.URL}
		current, err := pool.Fetch(ctx, site)
		if err != nil {
			return updates, err
		}
		previous, ok := st.Cisco.Sites[site.URL]
		var old *model.Snapshot
		if ok {
			old = &previous
		}
		if diff.Compute(site, old, current).Changed() {
			updates++
		}
		st.Cisco.Sites[site.URL] = current
	}
	return updates, nil
}
