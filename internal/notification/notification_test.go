package notification

import (
	"strings"
	"testing"
	"time"

	"github.com/sig9org/chatxgo/notify"
	"github.com/sig9org/update-radar/internal/model"
)

func TestMessageContainsOnlyProvidedChanges(t *testing.T) {
	location := time.FixedZone("JST", 9*60*60)
	message := Message(model.SiteDiff{
		Site:      model.Site{Name: "Test Product", URL: "https://software.cisco.com/test"},
		Suggested: model.SectionDiff{Added: []string{"2.0"}, Removed: []string{"1.0"}},
	}, time.Date(2026, 8, 6, 8, 42, 0, 0, location))
	if want := "Cisco Software Update (1 update)"; message.Subject != want {
		t.Errorf("subject = %q, want %q", message.Subject, want)
	}
	wantBody := "**Test Product**\n" +
		"- Suggested Release:\n" +
		"  - Added: 2.0\n" +
		"  - Removed: 1.0\n" +
		"- Download page:\n" +
		"  - https://software.cisco.com/test"
	if message.Body != wantBody {
		t.Errorf("body = %q, want %q", message.Body, wantBody)
	}
	if strings.Contains(message.Body, "Latest Release") {
		t.Errorf("unchanged section was included: %s", message.Body)
	}
}

func TestMessageFormatsTeamsFriendlyNestedList(t *testing.T) {
	message := Message(model.SiteDiff{
		Site: model.Site{
			Name: "Cisco Modeling Labs",
			URL:  "https://software.cisco.com/download/home/286193282/type/286326381/release/",
		},
		Suggested: model.SectionDiff{Added: []string{"2.10.0"}, Removed: []string{"2.9.0"}},
		Latest:    model.SectionDiff{Added: []string{"2.10.0"}},
		Deferred:  model.SectionDiff{Added: []string{"2.8.0"}},
	}, time.Time{})
	want := "**Cisco Modeling Labs**\n" +
		"- Suggested Release:\n" +
		"  - Added: 2.10.0\n" +
		"  - Removed: 2.9.0\n" +
		"- Latest Release:\n" +
		"  - Added: 2.10.0\n" +
		"- Deferred Release:\n" +
		"  - Added: 2.8.0\n" +
		"- Download page:\n" +
		"  - https://software.cisco.com/download/home/286193282/type/286326381/release/"
	if message.Body != want {
		t.Errorf("body = %q, want %q", message.Body, want)
	}
}

func TestMessagesGroupsChangesByDefault(t *testing.T) {
	changes := []model.SiteDiff{
		{Site: model.Site{Name: "Software A", URL: "https://example.com/a"}, Latest: model.SectionDiff{Added: []string{"2.0"}}},
		{Site: model.Site{Name: "Software B", URL: "https://example.com/b"}, Suggested: model.SectionDiff{Removed: []string{"1.0"}}},
	}
	messages := Messages(changes, time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC), false)
	if len(messages) != 1 {
		t.Fatalf("len(Messages) = %d, want 1", len(messages))
	}
	if messages[0].Subject != "Cisco Software Update (2 updates)" {
		t.Errorf("Subject = %q", messages[0].Subject)
	}
	for _, name := range []string{"Software A", "Software B"} {
		if !strings.Contains(messages[0].Body, "**"+name+"**") {
			t.Errorf("grouped body does not contain %q: %s", name, messages[0].Body)
		}
	}
	if strings.Contains(messages[0].Body, "## ") {
		t.Errorf("grouped body still uses large headings: %s", messages[0].Body)
	}
}

func TestMessagesIncludesMentions(t *testing.T) {
	change := model.SiteDiff{
		Site:   model.Site{Name: "Software A", URL: "https://example.com/a"},
		Latest: model.SectionDiff{Added: []string{"2.0"}},
	}
	mention := notify.Mention{ID: "user@example.com"}
	for _, separate := range []bool{false, true} {
		messages := Messages([]model.SiteDiff{change}, time.Time{}, separate, mention)
		if len(messages) != 1 || len(messages[0].Mentions) != 1 || messages[0].Mentions[0].ID != mention.ID {
			t.Errorf("separate=%t messages=%#v", separate, messages)
		}
	}
}

func TestMessagesSeparatesChangesWithSoftwareNameSubjects(t *testing.T) {
	changes := []model.SiteDiff{
		{Site: model.Site{Name: "Software A", URL: "https://example.com/a"}, Latest: model.SectionDiff{Added: []string{"2.0"}}},
		{Site: model.Site{Name: "Software B", URL: "https://example.com/b"}, Suggested: model.SectionDiff{Removed: []string{"1.0"}}},
	}
	messages := Messages(changes, time.Now(), true)
	if len(messages) != 2 {
		t.Fatalf("len(Messages) = %d, want 2", len(messages))
	}
	for i, want := range []string{"Software A", "Software B"} {
		if messages[i].Subject != want {
			t.Errorf("messages[%d].Subject = %q, want %q", i, messages[i].Subject, want)
		}
	}
}
