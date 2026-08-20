// Package monitor checks watched GitHub repositories for new releases
// and tags, and builds the chat notification for whatever it finds.
package monitor

import (
	"context"
	"fmt"
	"strings"

	"github.com/sig9org/chatxgo/notify"
	"github.com/sig9org/update-radar/internal/debugx"
	"github.com/sig9org/update-radar/internal/githubapi"
	"github.com/sig9org/update-radar/internal/githubstate"
	"github.com/sig9org/update-radar/internal/repoconfig"
)

// Kind identifies whether an Event is about a release or a tag.
type Kind string

const (
	KindRelease Kind = "release"
	KindTag     Kind = "tag"
)

// Event describes a newly discovered release or tag for one repository.
type Event struct {
	RepoName string
	RepoURL  string
	Kind     Kind
	Previous string // never empty: an Event only exists when something changed
	Current  string
}

// CheckRepositories checks every repository in cfg against st using
// client, returning the events for anything newly seen and the updated
// state (not yet persisted by the caller).
//
// A repository's current release/tag is only reported as an Event when a
// different value was previously recorded in st; the very first time a
// repository is seen, its release/tag is recorded as a baseline without
// being treated as "new" (there is nothing to compare it to yet).
func CheckRepositories(ctx context.Context, client *githubapi.Client, cfg repoconfig.Config, st state.State) ([]Event, state.State) {
	var events []Event

	for _, r := range cfg.Repositories {
		if !r.CheckReleases && !r.CheckTags {
			continue
		}

		owner, repo, err := githubapi.ParseOwnerRepo(r.URL)
		if err != nil {
			debugx.Printf("%s: %v", r.Name, err)
			continue
		}

		if r.CheckReleases {
			if ev, ok := checkRelease(ctx, client, &st, r, owner, repo); ok {
				events = append(events, ev)
			}
		}
		if r.CheckTags {
			if ev, ok := checkTag(ctx, client, &st, r, owner, repo); ok {
				events = append(events, ev)
			}
		}
	}

	return events, st
}

func checkRelease(ctx context.Context, client *githubapi.Client, st *state.State, r repoconfig.Repository, owner, repo string) (Event, bool) {
	release, ok, err := client.LatestRelease(ctx, owner, repo)
	if err != nil {
		debugx.Printf("%s: check latest release failed: %v", r.Name, err)
		return Event{}, false
	}
	if !ok {
		debugx.Printf("%s: no releases found", r.Name)
		return Event{}, false
	}

	previous := st.LatestRelease(r.Name)
	isNew := previous != "" && previous != release.TagName
	debugx.Printf("%s: latest release = %s (previous = %q, updated = %v)", r.Name, release.TagName, previous, isNew)

	st.SetLatestRelease(r.Name, release.TagName)
	if !isNew {
		return Event{}, false
	}
	return Event{
		RepoName: r.Name,
		RepoURL:  r.URL,
		Kind:     KindRelease,
		Previous: previous,
		Current:  release.TagName,
	}, true
}

func checkTag(ctx context.Context, client *githubapi.Client, st *state.State, r repoconfig.Repository, owner, repo string) (Event, bool) {
	tag, ok, err := client.LatestTag(ctx, owner, repo)
	if err != nil {
		debugx.Printf("%s: check latest tag failed: %v", r.Name, err)
		return Event{}, false
	}
	if !ok {
		debugx.Printf("%s: no tags found", r.Name)
		return Event{}, false
	}

	previous := st.LatestTag(r.Name)
	isNew := previous != "" && previous != tag.Name
	debugx.Printf("%s: latest tag = %s (previous = %q, updated = %v)", r.Name, tag.Name, previous, isNew)

	st.SetLatestTag(r.Name, tag.Name)
	if !isNew {
		return Event{}, false
	}
	return Event{
		RepoName: r.Name,
		RepoURL:  r.URL,
		Kind:     KindTag,
		Previous: previous,
		Current:  tag.Name,
	}, true
}

// BuildMessage renders events as a single Markdown notification. The
// caller should not call this with an empty events slice. settings.Mention
// is parsed into one mention per comma-separated "id" or "id:label" entry
// (chatxgo's own -mention flag format), supporting more than one recipient
// when chatxgo's target chat tool allows it.
func BuildMessage(events []Event, settings repoconfig.Settings) (notify.Message, error) {
	mentions, err := parseMentions(settings.Mention)
	if err != nil {
		return notify.Message{}, err
	}

	return notify.Message{
		Subject:  groupedSubject(len(events)),
		Body:     buildBody(events),
		Mentions: mentions,
	}, nil
}

func groupedSubject(updateCount int) string {
	label := "updates"
	if updateCount == 1 {
		label = "update"
	}
	return fmt.Sprintf("GitHub Update (%d %s)", updateCount, label)
}

// BuildSeparateMessages renders one notification per event. Each message
// uses the corresponding sites.yaml name as its entire subject.
func BuildSeparateMessages(events []Event, settings repoconfig.Settings) ([]notify.Message, error) {
	mentions, err := parseMentions(settings.Mention)
	if err != nil {
		return nil, err
	}

	messages := make([]notify.Message, 0, len(events))
	for _, event := range events {
		messages = append(messages, notify.Message{
			Subject:  event.RepoName,
			Body:     buildBody([]Event{event}),
			Mentions: mentions,
		})
	}
	return messages, nil
}

func buildBody(events []Event) string {
	var b strings.Builder
	for _, ev := range events {
		fmt.Fprintf(&b, "- [%s](%s) new %s: [%s](%s) -> [%s](%s)\n",
			ev.RepoName, ev.RepoURL, ev.Kind,
			ev.Previous, releaseTagURL(ev.RepoURL, ev.Previous),
			ev.Current, releaseTagURL(ev.RepoURL, ev.Current),
		)
	}
	return strings.TrimRight(b.String(), "\n")
}

// releaseTagURL builds the GitHub releases page URL for a given tag/version
// of repoURL, e.g. ("https://github.com/owner/repo", "v1.0.0") ->
// "https://github.com/owner/repo/releases/tag/v1.0.0". Used for both
// releases and tags, since a repository's tags are shown on the same
// releases page even when it has no release notes for that tag.
func releaseTagURL(repoURL, version string) string {
	return strings.TrimRight(repoURL, "/") + "/releases/tag/" + version
}

// parseMentions splits the normalized mention list on commas and parses each part with
// notify.ParseMention. An empty raw yields no mentions.
func parseMentions(raw string) ([]notify.Mention, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	var mentions []notify.Mention
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		m, err := notify.ParseMention(part)
		if err != nil {
			return nil, fmt.Errorf("monitor: settings.mention: %w", err)
		}
		mentions = append(mentions, m)
	}
	return mentions, nil
}
