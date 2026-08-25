// Package selfupdate updates update-radar from its GitHub release assets.
package selfupdate

import (
	"context"
	"fmt"

	selfupdatego "github.com/sig9org/selfupdate-go"
	"github.com/sig9org/update-radar/internal/debugx"
)

// Repository is "owner/repo", where release binaries are published.
const Repository = "sig9org/update-radar"

// Update checks GitHub for a newer release and replaces the running binary.
// The replacement is accepted only when its SHA-256 value matches the
// checksums.txt asset published with the same release.
func Update(ctx context.Context, currentVersion string) (string, error) {
	debugx.Printf("checking %s for a release newer than %s", Repository, currentVersion)

	updater, err := selfupdatego.New(selfupdatego.Config{
		Repository: Repository,
		Validator:  selfupdatego.SHA256Validator{AssetName: "checksums.txt"},
	})
	if err != nil {
		return "", fmt.Errorf("selfupdate: configure updater: %w", err)
	}
	result, err := updater.Update(ctx, currentVersion)
	if err != nil {
		return "", fmt.Errorf("selfupdate: update failed: %w", err)
	}
	debugx.Printf("latest release found: %s", result.LatestVersion)
	if !result.Updated {
		return fmt.Sprintf("already up to date (current %s, latest %s)", currentVersion, result.LatestVersion), nil
	}
	return fmt.Sprintf("updated to version %s", result.LatestVersion), nil
}
