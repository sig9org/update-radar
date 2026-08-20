// Package notification formats and sends release changes through chatxgo.
package notification

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/sig9org/chatxgo/notify"
	"github.com/sig9org/update-radar/internal/model"
)

// Message builds the default portable Markdown notification for one changed
// site. Messages should normally be used so multiple changes can be grouped.
func Message(change model.SiteDiff, now time.Time) notify.Message {
	return notify.Message{
		Subject: aggregateSubject(1),
		Body:    messageBodyWithTitle(change),
	}
}

// Messages builds either one grouped notification or one notification per
// changed software page. Separate notification subjects contain only the name
// configured for the site.
func Messages(changes []model.SiteDiff, now time.Time, separate bool, mentions ...notify.Mention) []notify.Message {
	if len(changes) == 0 {
		return nil
	}
	if separate {
		messages := make([]notify.Message, 0, len(changes))
		for _, change := range changes {
			messages = append(messages, notify.Message{
				Subject:  change.Site.Name,
				Body:     messageBody(change),
				Mentions: cloneMentions(mentions),
			})
		}
		return messages
	}
	if len(changes) == 1 {
		message := Message(changes[0], now)
		message.Mentions = cloneMentions(mentions)
		return []notify.Message{message}
	}

	var body strings.Builder
	for i, change := range changes {
		if i != 0 {
			body.WriteString("\n\n")
		}
		fmt.Fprintf(&body, "**%s**\n%s", change.Site.Name, messageBody(change))
	}
	return []notify.Message{{
		Subject:  aggregateSubject(len(changes)),
		Body:     body.String(),
		Mentions: cloneMentions(mentions),
	}}
}

func aggregateSubject(count int) string {
	unit := "updates"
	if count == 1 {
		unit = "update"
	}
	return fmt.Sprintf("Cisco Software Update (%d %s)", count, unit)
}

func cloneMentions(mentions []notify.Mention) []notify.Mention {
	return append([]notify.Mention(nil), mentions...)
}

func messageBody(change model.SiteDiff) string {
	var body strings.Builder
	writeSection(&body, "Suggested Release", change.Suggested)
	writeSection(&body, "Latest Release", change.Latest)
	fmt.Fprintf(&body, "- Download page:\n  - %s", change.Site.URL)
	return body.String()
}

func messageBodyWithTitle(change model.SiteDiff) string {
	return fmt.Sprintf("**%s**\n%s", change.Site.Name, messageBody(change))
}

func writeSection(body *strings.Builder, name string, change model.SectionDiff) {
	if !change.Changed() {
		return
	}
	fmt.Fprintf(body, "- %s:\n", name)
	for _, version := range change.Added {
		fmt.Fprintf(body, "  - Added: %s\n", version)
	}
	for _, version := range change.Removed {
		fmt.Fprintf(body, "  - Removed: %s\n", version)
	}
}

// Send dispatches a notification and returns all per-tool failures.
func Send(ctx context.Context, cfg notify.Config, message notify.Message) ([]notify.Result, error) {
	dispatcher, err := notify.NewDispatcher(cfg)
	if err != nil {
		return nil, err
	}
	return dispatcher.Send(ctx, message)
}
