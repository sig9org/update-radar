// Package diff compares current Cisco release data with saved state.
package diff

import (
	"slices"

	"github.com/sig9org/update-radar/internal/model"
)

// Compute returns a stable, sorted release difference.
func Compute(site model.Site, previous *model.Snapshot, current model.Snapshot) model.SiteDiff {
	result := model.SiteDiff{Site: site, Snapshot: current, FirstRun: previous == nil}
	if previous == nil || stateIdentityChanged(*previous, current) {
		// A state generated for a different product must not be compared with
		// the current release lists, otherwise every version appears changed.
		result.FirstRun = true
		return result
	}
	result.Suggested = section(previous.Suggested, current.Suggested)
	result.Latest = section(previous.Latest, current.Latest)
	result.Deferred = section(previous.Deferred, current.Deferred)
	return result
}

func stateIdentityChanged(previous, current model.Snapshot) bool {
	return previous.ProductName != "" && current.ProductName != "" && previous.ProductName != current.ProductName
}

func section(oldVersions, newVersions []string) model.SectionDiff {
	oldSet := asSet(oldVersions)
	newSet := asSet(newVersions)
	var result model.SectionDiff
	for version := range newSet {
		if _, ok := oldSet[version]; !ok {
			result.Added = append(result.Added, version)
		}
	}
	for version := range oldSet {
		if _, ok := newSet[version]; !ok {
			result.Removed = append(result.Removed, version)
		}
	}
	slices.Sort(result.Added)
	slices.Sort(result.Removed)
	return result
}

func asSet(values []string) map[string]struct{} {
	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		result[value] = struct{}{}
	}
	return result
}
