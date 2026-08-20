// Package model contains the domain types shared by cisco-rader.
package model

import "time"

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
	FetchedAt   time.Time `yaml:"fetched_at"`
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
	FirstRun  bool
}

// Changed reports whether either release section changed.
func (d SiteDiff) Changed() bool { return d.Suggested.Changed() || d.Latest.Changed() }
