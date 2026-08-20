// Package state persists the most recently seen content hash for each
// watched site, so web-rader can tell whether a fresh check turned up a
// change.
package state

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// SiteState is what was last seen for a single site.
type SiteState struct {
	Hash string `yaml:"hash"`
}

// State maps a site name (siteconfig.Site.Name) to what was last seen for
// it.
type State struct {
	Sites map[string]SiteState `yaml:"sites"`
}

// MarshalYAML writes Sites with its keys sorted alphabetically
// (case-insensitively), so the state file stays in a stable, readable
// order across saves regardless of map iteration order (yaml.v3's default
// map encoding sorts by raw byte order, which would put every uppercase
// name before any lowercase one).
func (s State) MarshalYAML() (interface{}, error) {
	names := make([]string, 0, len(s.Sites))
	for name := range s.Sites {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool {
		return strings.ToLower(names[i]) < strings.ToLower(names[j])
	})

	sitesNode := &yaml.Node{Kind: yaml.MappingNode}
	for _, name := range names {
		var keyNode, valueNode yaml.Node
		if err := keyNode.Encode(name); err != nil {
			return nil, err
		}
		if err := valueNode.Encode(s.Sites[name]); err != nil {
			return nil, err
		}
		sitesNode.Content = append(sitesNode.Content, &keyNode, &valueNode)
	}

	var sitesKey yaml.Node
	if err := sitesKey.Encode("sites"); err != nil {
		return nil, err
	}
	root := &yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{&sitesKey, sitesNode}}
	return root, nil
}

// Load reads the state file at path. A missing file is not an error: it
// yields an empty State, as on first run.
func Load(path string) (State, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return State{Sites: map[string]SiteState{}}, nil
		}
		return State{}, fmt.Errorf("state: read %s: %w", path, err)
	}
	var s State
	if err := yaml.Unmarshal(data, &s); err != nil {
		return State{}, fmt.Errorf("state: parse %s: %w", path, err)
	}
	if s.Sites == nil {
		s.Sites = map[string]SiteState{}
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

// Hash returns the last-seen content hash for site, or "" if none has been
// recorded.
func (s State) Hash(site string) string {
	return s.Sites[site].Hash
}

// SetHash records hash as the last-seen content hash for site.
func (s *State) SetHash(site, hash string) {
	entry := s.Sites[site]
	entry.Hash = hash
	s.Sites[site] = entry
}
