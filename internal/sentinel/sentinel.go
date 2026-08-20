// Package sentinel checks watched sites for content changes, and builds
// the chat notification for whatever it finds.
package sentinel

import (
	"context"
	"fmt"
	"strings"

	"github.com/sig9org/chatxgo/notify"
	"github.com/sig9org/update-radar/internal/debugx"
	"github.com/sig9org/update-radar/internal/siteconfig"
	"github.com/sig9org/update-radar/internal/webclient"
	"github.com/sig9org/update-radar/internal/webstate"
)

// Event describes a newly detected content change for one site.
type Event struct {
	SiteName string
	SiteLink string
	Previous string // never empty: an Event only exists when something changed
	Current  string
}

// CheckSites fetches every site in cfg and compares its content hash
// against st, returning the events for anything newly changed and the
// updated state (not yet persisted by the caller).
//
// A site's current content hash is only reported as an Event when a
// different hash was previously recorded in st; the very first time a
// site is checked, its hash is recorded as a baseline without being
// treated as "changed" (there is nothing to compare it to yet).
func CheckSites(ctx context.Context, client *webclient.Client, cfg siteconfig.Config, st state.State) ([]Event, state.State) {
	var events []Event

	for _, s := range cfg.Sites {
		hash, err := client.Fetch(ctx, s.Check)
		if err != nil {
			debugx.Printf("%s: fetch failed: %v", s.Name, err)
			continue
		}

		previous := st.Hash(s.Name)
		isNew := previous != "" && previous != hash
		debugx.Printf("%s: hash = %s (previous = %q, changed = %v)", s.Name, hash, previous, isNew)

		st.SetHash(s.Name, hash)
		if !isNew {
			continue
		}
		events = append(events, Event{
			SiteName: s.Name,
			SiteLink: s.Link,
			Previous: previous,
			Current:  hash,
		})
	}

	return events, st
}

// BuildMessage renders events as a single Markdown notification. The
// caller should not call this with an empty events slice. settings.Subject
// overrides the default subject when set, and settings.Mention is parsed
// into one mention per comma-separated "id" or "id:label" entry (chatxgo's
// own -mention flag format), supporting more than one recipient when
// chatxgo's target chat tool allows it.
func BuildMessage(events []Event, settings siteconfig.Settings) (notify.Message, error) {
	subject := fmt.Sprintf("Web Site Update (%d updates)", len(events))
	if strings.TrimSpace(settings.Subject) != "" {
		subject = settings.Subject
	}

	var b strings.Builder
	for _, ev := range events {
		fmt.Fprintf(&b, "- [%s](%s) updated: `%s` -> `%s`\n",
			ev.SiteName, ev.SiteLink, shortHash(ev.Previous), shortHash(ev.Current))
	}

	mentions, err := parseMentions(settings.Mention)
	if err != nil {
		return notify.Message{}, err
	}

	return notify.Message{
		Subject:  subject,
		Body:     strings.TrimRight(b.String(), "\n"),
		Mentions: mentions,
	}, nil
}

// BuildMessages renders events for dispatch. By default it preserves the
// combined notification produced by BuildMessage. When separate is true, it
// returns one notification per event and uses only that event's site name as
// the subject; settings.Subject is intentionally ignored in that mode.
func BuildMessages(events []Event, settings siteconfig.Settings, separate bool) ([]notify.Message, error) {
	if !separate {
		msg, err := BuildMessage(events, settings)
		if err != nil {
			return nil, err
		}
		return []notify.Message{msg}, nil
	}

	messages := make([]notify.Message, 0, len(events))
	for _, event := range events {
		separateSettings := settings
		separateSettings.Subject = event.SiteName
		msg, err := BuildMessage([]Event{event}, separateSettings)
		if err != nil {
			return nil, err
		}
		messages = append(messages, msg)
	}
	return messages, nil
}

// shortHash truncates a hex digest to its first 8 characters for compact
// display in a chat notification.
func shortHash(hash string) string {
	if len(hash) > 8 {
		return hash[:8]
	}
	return hash
}

// parseMentions splits raw on commas and parses each part with
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
			return nil, fmt.Errorf("sentinel: settings.mention: %w", err)
		}
		mentions = append(mentions, m)
	}
	return mentions, nil
}
