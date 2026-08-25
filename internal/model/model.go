// Package model contains the domain types shared by cisco-rader.
package model

import (
	"time"

	"gopkg.in/yaml.v3"
)

// Site is one Cisco Software Download page to monitor.
type Site struct {
	Name string `yaml:"name"`
	URL  string `yaml:"url"`
}

// Snapshot is the release information observed during one check.
type Snapshot struct {
	ProductName string    `yaml:"product_name"`
	Suggested   []string  `yaml:"suggested"`
	Latest      []string  `yaml:"latest"`
	Deferred    []string  `yaml:"deferred"`
	FetchedAt   time.Time `yaml:"fetched_at"`
}

// MarshalYAML writes release versions as double-quoted YAML strings while
// preserving the regular in-memory []string representation.
func (s Snapshot) MarshalYAML() (any, error) {
	type snapshotYAML struct {
		ProductName string         `yaml:"product_name"`
		Suggested   quotedVersions `yaml:"suggested"`
		Latest      quotedVersions `yaml:"latest"`
		Deferred    quotedVersions `yaml:"deferred"`
		FetchedAt   time.Time      `yaml:"fetched_at"`
	}
	return snapshotYAML{
		ProductName: s.ProductName,
		Suggested:   quotedVersions(s.Suggested),
		Latest:      quotedVersions(s.Latest),
		Deferred:    quotedVersions(s.Deferred),
		FetchedAt:   s.FetchedAt,
	}, nil
}

type quotedVersions []string

func (versions quotedVersions) MarshalYAML() (any, error) {
	node := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
	for _, version := range versions {
		node.Content = append(node.Content, &yaml.Node{
			Kind:  yaml.ScalarNode,
			Tag:   "!!str",
			Value: version,
			Style: yaml.DoubleQuotedStyle,
		})
	}
	return node, nil
}

// SectionDiff describes versions added to and removed from one section.
type SectionDiff struct {
	Added   []string
	Removed []string
}

// Changed reports whether the section differs from the saved state.
func (d SectionDiff) Changed() bool { return len(d.Added) != 0 || len(d.Removed) != 0 }

// SiteDiff describes all release changes found for one site.
type SiteDiff struct {
	Site      Site
	Snapshot  Snapshot
	Suggested SectionDiff
	Latest    SectionDiff
	Deferred  SectionDiff
	FirstRun  bool
}

// Changed reports whether either release section changed.
func (d SiteDiff) Changed() bool {
	return d.Suggested.Changed() || d.Latest.Changed() || d.Deferred.Changed()
}
