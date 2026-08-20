// Package state persists the most recently seen release tag and tag name
// for each watched repository, so github-rader can tell whether a fresh
// check turned up something new.
package state

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// RepoState is what was last seen for a single repository.
type RepoState struct {
	LatestRelease string `yaml:"latest_release,omitempty"`
	LatestTag     string `yaml:"latest_tag,omitempty"`
}

// State maps a repository name (repoconfig.Repository.Name) to what was
// last seen for it.
type State struct {
	Repositories map[string]RepoState `yaml:"sites"`
}

// MarshalYAML writes Repositories with its keys sorted alphabetically
// (case-insensitively), so the state file stays in a stable, readable
// order across saves regardless of map iteration order.
func (s State) MarshalYAML() (any, error) {
	names := make([]string, 0, len(s.Repositories))
	for name := range s.Repositories {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool {
		return strings.ToLower(names[i]) < strings.ToLower(names[j])
	})

	root := &yaml.Node{Kind: yaml.MappingNode}
	root.Content = append(root.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: "sites"})
	sites := &yaml.Node{Kind: yaml.MappingNode}
	for _, name := range names {
		value := &yaml.Node{}
		if err := value.Encode(s.Repositories[name]); err != nil {
			return nil, err
		}
		sites.Content = append(sites.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Value: name}, value)
	}
	root.Content = append(root.Content, sites)
	return root, nil
}

// Load reads the state file at path. A missing file is not an error: it
// yields an empty State, as on first run.
func Load(path string) (State, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return State{Repositories: map[string]RepoState{}}, nil
		}
		return State{}, fmt.Errorf("state: read %s: %w", path, err)
	}
	var s State
	if err := yaml.Unmarshal(data, &s); err != nil {
		return State{}, fmt.Errorf("state: parse %s: %w", path, err)
	}
	if s.Repositories == nil {
		s.Repositories = map[string]RepoState{}
	}
	return s, nil
}

// Save writes the state file at path.
func (s State) Save(path string) error {
	data, err := yaml.Marshal(s)
	if err != nil {
		return fmt.Errorf("state: encode: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("state: write %s: %w", path, err)
	}
	return nil
}

// LatestRelease returns the last-seen release tag for repo, or "" if none
// has been recorded.
func (s State) LatestRelease(repo string) string {
	return s.Repositories[repo].LatestRelease
}

// LatestTag returns the last-seen tag name for repo, or "" if none has
// been recorded.
func (s State) LatestTag(repo string) string {
	return s.Repositories[repo].LatestTag
}

// SetLatestRelease records tag as the last-seen release for repo.
func (s *State) SetLatestRelease(repo, tag string) {
	entry := s.Repositories[repo]
	entry.LatestRelease = tag
	s.Repositories[repo] = entry
}

// SetLatestTag records name as the last-seen tag for repo.
func (s *State) SetLatestTag(repo, name string) {
	entry := s.Repositories[repo]
	entry.LatestTag = name
	s.Repositories[repo] = entry
}
