package sentinel

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sig9org/update-radar/internal/siteconfig"
	"github.com/sig9org/update-radar/internal/webclient"
	"github.com/sig9org/update-radar/internal/webstate"
)

func newTestServer(t *testing.T, body *string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(*body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func testConfig(name, url string) siteconfig.Config {
	return siteconfig.Config{
		Sites: []siteconfig.Site{{Name: name, Check: url, Link: url}},
	}
}

func TestCheckSitesFirstRunSeedsWithoutEvent(t *testing.T) {
	body := "v1"
	srv := newTestServer(t, &body)
	cfg := testConfig("Example", srv.URL)
	st, _ := state.Load("/nonexistent")

	events, newState := CheckSites(context.Background(), webclient.NewClient(), cfg, st)

	if len(events) != 0 {
		t.Fatalf("events = %+v, want none on first run", events)
	}
	if got := newState.Hash("Example"); got == "" {
		t.Errorf("Hash(Example) = %q, want a recorded baseline hash", got)
	}
}

func TestCheckSitesDetectsChange(t *testing.T) {
	body := "v2"
	srv := newTestServer(t, &body)
	cfg := testConfig("Example", srv.URL)
	st, _ := state.Load("/nonexistent")
	st.SetHash("Example", "stale-hash")

	events, newState := CheckSites(context.Background(), webclient.NewClient(), cfg, st)

	if len(events) != 1 {
		t.Fatalf("len(events) = %d, want 1", len(events))
	}
	ev := events[0]
	if ev.SiteName != "Example" || ev.Previous != "stale-hash" || ev.Current == "" || ev.Current == ev.Previous {
		t.Errorf("event = %+v, want a changed hash from stale-hash", ev)
	}
	if got := newState.Hash("Example"); got != ev.Current {
		t.Errorf("Hash(Example) = %q, want %q", got, ev.Current)
	}
}

func TestCheckSitesNoChangeNoEvent(t *testing.T) {
	body := "same"
	srv := newTestServer(t, &body)
	cfg := testConfig("Example", srv.URL)
	st, _ := state.Load("/nonexistent")

	// Seed the baseline first, mirroring a prior run.
	_, seeded := CheckSites(context.Background(), webclient.NewClient(), cfg, st)

	events, _ := CheckSites(context.Background(), webclient.NewClient(), cfg, seeded)
	if len(events) != 0 {
		t.Fatalf("events = %+v, want none when content is unchanged", events)
	}
}

func TestCheckSitesSkipsFetchFailure(t *testing.T) {
	cfg := testConfig("Broken", "http://127.0.0.1:0/does-not-exist")
	st, _ := state.Load("/nonexistent")

	events, newState := CheckSites(context.Background(), webclient.NewClient(), cfg, st)
	if len(events) != 0 {
		t.Fatalf("events = %+v, want none for a site that fails to fetch", events)
	}
	if got := newState.Hash("Broken"); got != "" {
		t.Errorf("Hash(Broken) = %q, want empty when fetch failed", got)
	}
}

func TestBuildMessage(t *testing.T) {
	events := []Event{
		{SiteName: "Example One", SiteLink: "https://example.invalid/one", Previous: "aaaaaaaaaaaa", Current: "bbbbbbbbbbbb"},
		{SiteName: "Example Two", SiteLink: "https://example.invalid/two", Previous: "cccccccccccc", Current: "dddddddddddd"},
	}

	msg, err := BuildMessage(events, siteconfig.Settings{})
	if err != nil {
		t.Fatalf("BuildMessage() error = %v", err)
	}

	if msg.Subject != "Web Site Update (2 updates)" {
		t.Errorf("Subject = %q, want %q", msg.Subject, "Web Site Update (2 updates)")
	}

	wantLines := []string{
		"- [Example One](https://example.invalid/one) updated: `aaaaaaaa` -> `bbbbbbbb`",
		"- [Example Two](https://example.invalid/two) updated: `cccccccc` -> `dddddddd`",
	}
	for _, want := range wantLines {
		if !strings.Contains(msg.Body, want) {
			t.Errorf("Body = %q, want it to contain %q", msg.Body, want)
		}
	}
	if len(msg.Mentions) != 0 {
		t.Errorf("Mentions = %+v, want none when settings.mention is empty", msg.Mentions)
	}
}

func TestBuildMessageSubjectOverride(t *testing.T) {
	events := []Event{{SiteName: "Example", SiteLink: "https://example.invalid/", Previous: "aaaaaaaaaaaa", Current: "bbbbbbbbbbbb"}}

	msg, err := BuildMessage(events, siteconfig.Settings{Subject: "Custom Title"})
	if err != nil {
		t.Fatalf("BuildMessage() error = %v", err)
	}
	if msg.Subject != "Custom Title" {
		t.Errorf("Subject = %q, want %q", msg.Subject, "Custom Title")
	}
}

func TestBuildMessagesCombinedByDefault(t *testing.T) {
	events := []Event{
		{SiteName: "Example One", SiteLink: "https://example.invalid/one", Previous: "aaaaaaaaaaaa", Current: "bbbbbbbbbbbb"},
		{SiteName: "Example Two", SiteLink: "https://example.invalid/two", Previous: "cccccccccccc", Current: "dddddddddddd"},
	}

	messages, err := BuildMessages(events, siteconfig.Settings{Subject: "Custom Title"}, false)
	if err != nil {
		t.Fatalf("BuildMessages() error = %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("len(messages) = %d, want 1", len(messages))
	}
	if messages[0].Subject != "Custom Title" {
		t.Errorf("Subject = %q, want %q", messages[0].Subject, "Custom Title")
	}
	if !strings.Contains(messages[0].Body, "Example One") || !strings.Contains(messages[0].Body, "Example Two") {
		t.Errorf("Body = %q, want both updates", messages[0].Body)
	}
}

func TestBuildMessagesSeparateUsesSiteNamesAsSubjects(t *testing.T) {
	events := []Event{
		{SiteName: "Example One", SiteLink: "https://example.invalid/one", Previous: "aaaaaaaaaaaa", Current: "bbbbbbbbbbbb"},
		{SiteName: "Example Two", SiteLink: "https://example.invalid/two", Previous: "cccccccccccc", Current: "dddddddddddd"},
	}

	messages, err := BuildMessages(events, siteconfig.Settings{Subject: "Ignored", Mention: "foobar@example.com"}, true)
	if err != nil {
		t.Fatalf("BuildMessages() error = %v", err)
	}
	if len(messages) != 2 {
		t.Fatalf("len(messages) = %d, want 2", len(messages))
	}
	for i, want := range []string{"Example One", "Example Two"} {
		if messages[i].Subject != want {
			t.Errorf("messages[%d].Subject = %q, want %q", i, messages[i].Subject, want)
		}
		if !strings.Contains(messages[i].Body, want) {
			t.Errorf("messages[%d].Body = %q, want only the %q update", i, messages[i].Body, want)
		}
		if len(messages[i].Mentions) != 1 {
			t.Errorf("len(messages[%d].Mentions) = %d, want 1", i, len(messages[i].Mentions))
		}
	}
	if strings.Contains(messages[0].Body, "Example Two") || strings.Contains(messages[1].Body, "Example One") {
		t.Errorf("separate message bodies contain another site's update: %+v", messages)
	}
}

func TestBuildMessageMentions(t *testing.T) {
	events := []Event{{SiteName: "Example", SiteLink: "https://example.invalid/", Previous: "aaaaaaaaaaaa", Current: "bbbbbbbbbbbb"}}

	msg, err := BuildMessage(events, siteconfig.Settings{Mention: "foobar@example.com, U0123456:Someone"})
	if err != nil {
		t.Fatalf("BuildMessage() error = %v", err)
	}
	if len(msg.Mentions) != 2 {
		t.Fatalf("len(Mentions) = %d, want 2", len(msg.Mentions))
	}
	if msg.Mentions[0].ID != "foobar@example.com" {
		t.Errorf("Mentions[0].ID = %q, want %q", msg.Mentions[0].ID, "foobar@example.com")
	}
	if msg.Mentions[1].ID != "U0123456" || msg.Mentions[1].Label != "Someone" {
		t.Errorf("Mentions[1] = %+v, want ID=U0123456 Label=Someone", msg.Mentions[1])
	}
}

func TestBuildMessageNoMentionsWhenEmpty(t *testing.T) {
	events := []Event{{SiteName: "Example", SiteLink: "https://example.invalid/", Previous: "aaaaaaaaaaaa", Current: "bbbbbbbbbbbb"}}

	msg, err := BuildMessage(events, siteconfig.Settings{Mention: "  "})
	if err != nil {
		t.Fatalf("BuildMessage() error = %v", err)
	}
	if len(msg.Mentions) != 0 {
		t.Errorf("Mentions = %+v, want none for a blank mention setting", msg.Mentions)
	}
}

func TestShortHash(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"abcdefgh12345", "abcdefgh"},
		{"short", "short"},
		{"", ""},
	}
	for _, tt := range tests {
		if got := shortHash(tt.in); got != tt.want {
			t.Errorf("shortHash(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
