// Package version holds build-time version information for update-radar.
package version

import "runtime/debug"

// Name is the tool's display name.
const Name = "update-radar"

// Version is set at build time via -ldflags "-X .../version.Version=vX.Y.Z".
// It is derived from the latest git tag (see Taskfile.yml VERSION/RELEASE_VERSION).
var Version = "dev"

// CommitHash returns the short VCS revision the running binary was built
// from. The go command embeds this automatically (see
// runtime/debug.BuildInfo) for binaries built with "go build" or "go run"
// inside a git checkout; it returns "" if that information is unavailable
// (e.g. a "go test" binary, or a binary built with -trimpath).
func CommitHash() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}
	var revision string
	for _, s := range info.Settings {
		if s.Key == "vcs.revision" {
			revision = s.Value
			break
		}
	}
	if len(revision) > 7 {
		revision = revision[:7]
	}
	return revision
}

// String returns the name and release tag. Build commit IDs are not shown.
func String() string {
	return Name + " " + Version
}
