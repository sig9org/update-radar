package monitor

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sig9org/update-radar/internal/githubapi"
	"github.com/sig9org/update-radar/internal/githubstate"
	"github.com/sig9org/update-radar/internal/repoconfig"
)

func newTestClient(t *testing.T, releaseTag, tagName string, calls *int) *githubapi.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls != nil {
			*calls++
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/releases/latest"):
			fmt.Fprintf(w, `{"tag_name":%q,"name":%q,"html_url":"https://example.invalid/release"}`, releaseTag, releaseTag)
		case strings.HasSuffix(r.URL.Path, "/tags"):
			fmt.Fprintf(w, `[{"name":%q,"commit":{"sha":"abc"}}]`, tagName)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)

	oldBase := githubapi.BaseURL
	githubapi.BaseURL = srv.URL
	t.Cleanup(func() { githubapi.BaseURL = oldBase })

	return githubapi.NewClient("")
}

func testConfig(checkReleases, checkTags bool) repoconfig.Config {
	return repoconfig.Config{
		Repositories: []repoconfig.Repository{
			{
				Name:          "Example",
				URL:           "https://github.com/example/repo",
				CheckReleases: checkReleases,
				CheckTags:     checkTags,
			},
		},
	}
}

func TestCheckRepositoriesFirstRunSeedsWithoutEvent(t *testing.T) {
	client := newTestClient(t, "v1.0.0", "v1.0.0", nil)
	cfg := testConfig(true, false)
	st, _ := state.Load("/nonexistent")

	events, newState := CheckRepositories(context.Background(), client, cfg, st)

	if len(events) != 0 {
		t.Fatalf("events = %+v, want none on first run", events)
	}
	if got := newState.LatestRelease("Example"); got != "v1.0.0" {
		t.Errorf("LatestRelease(Example) = %q, want %q", got, "v1.0.0")
	}
}

func TestCheckRepositoriesDetectsNewRelease(t *testing.T) {
	client := newTestClient(t, "v2.0.0", "", nil)
	cfg := testConfig(true, false)
	st, _ := state.Load("/nonexistent")
	st.SetLatestRelease("Example", "v1.0.0")

	events, newState := CheckRepositories(context.Background(), client, cfg, st)

	if len(events) != 1 {
		t.Fatalf("len(events) = %d, want 1", len(events))
	}
	ev := events[0]
	if ev.Kind != KindRelease || ev.Previous != "v1.0.0" || ev.Current != "v2.0.0" {
		t.Errorf("event = %+v, want release v1.0.0 -> v2.0.0", ev)
	}
	if got := newState.LatestRelease("Example"); got != "v2.0.0" {
		t.Errorf("LatestRelease(Example) = %q, want %q", got, "v2.0.0")
	}
}

func TestCheckRepositoriesNoChangeNoEvent(t *testing.T) {
	client := newTestClient(t, "v1.0.0", "", nil)
	cfg := testConfig(true, false)
	st, _ := state.Load("/nonexistent")
	st.SetLatestRelease("Example", "v1.0.0")

	events, _ := CheckRepositories(context.Background(), client, cfg, st)

	if len(events) != 0 {
		t.Fatalf("events = %+v, want none when release is unchanged", events)
	}
}

func TestCheckRepositoriesDetectsNewTag(t *testing.T) {
	client := newTestClient(t, "", "v3.0.0", nil)
	cfg := testConfig(false, true)
	st, _ := state.Load("/nonexistent")
	st.SetLatestTag("Example", "v2.0.0")

	events, newState := CheckRepositories(context.Background(), client, cfg, st)

	if len(events) != 1 {
		t.Fatalf("len(events) = %d, want 1", len(events))
	}
	ev := events[0]
	if ev.Kind != KindTag || ev.Previous != "v2.0.0" || ev.Current != "v3.0.0" {
		t.Errorf("event = %+v, want tag v2.0.0 -> v3.0.0", ev)
	}
	if got := newState.LatestTag("Example"); got != "v3.0.0" {
		t.Errorf("LatestTag(Example) = %q, want %q", got, "v3.0.0")
	}
}

func TestCheckRepositoriesSkipsDisabledChecks(t *testing.T) {
	var calls int
	client := newTestClient(t, "v1.0.0", "v1.0.0", &calls)
	cfg := testConfig(false, false)
	st, _ := state.Load("/nonexistent")

	events, _ := CheckRepositories(context.Background(), client, cfg, st)

	if len(events) != 0 {
		t.Fatalf("events = %+v, want none when both checks are disabled", events)
	}
	if calls != 0 {
		t.Errorf("calls = %d, want 0 when both checks are disabled", calls)
	}
}

func TestCheckRepositoriesSkipsInvalidURL(t *testing.T) {
	client := newTestClient(t, "v1.0.0", "", nil)
	cfg := repoconfig.Config{
		Repositories: []repoconfig.Repository{
			{Name: "Broken", URL: "not-a-github-url", CheckReleases: true},
		},
	}
	st, _ := state.Load("/nonexistent")

	events, _ := CheckRepositories(context.Background(), client, cfg, st)
	if len(events) != 0 {
		t.Fatalf("events = %+v, want none for a repository with an invalid URL", events)
	}
}

func TestBuildMessage(t *testing.T) {
	events := []Event{
		{RepoName: "Terraform Provider for Cisco CML2", RepoURL: "https://github.com/CiscoDevNet/terraform-provider-cml2", Kind: KindRelease, Previous: "v0.9.0", Current: "v0.9.1"},
		{RepoName: "WordPress", RepoURL: "https://github.com/WordPress/WordPress", Kind: KindTag, Previous: "7.0.1", Current: "7.0.2"},
	}

	msg, err := BuildMessage(events, repoconfig.Settings{})
	if err != nil {
		t.Fatalf("BuildMessage() error = %v", err)
	}

	if msg.Subject != "GitHub Update (2 updates)" {
		t.Errorf("Subject = %q, want %q", msg.Subject, "GitHub Update (2 updates)")
	}

	wantLines := []string{
		"- [Terraform Provider for Cisco CML2](https://github.com/CiscoDevNet/terraform-provider-cml2) new release: " +
			"[v0.9.0](https://github.com/CiscoDevNet/terraform-provider-cml2/releases/tag/v0.9.0) -> " +
			"[v0.9.1](https://github.com/CiscoDevNet/terraform-provider-cml2/releases/tag/v0.9.1)",
		"- [WordPress](https://github.com/WordPress/WordPress) new tag: " +
			"[7.0.1](https://github.com/WordPress/WordPress/releases/tag/7.0.1) -> " +
			"[7.0.2](https://github.com/WordPress/WordPress/releases/tag/7.0.2)",
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

func TestReleaseTagURL(t *testing.T) {
	tests := []struct {
		repoURL string
		version string
		want    string
	}{
		{"https://github.com/owner/repo", "v1.0.0", "https://github.com/owner/repo/releases/tag/v1.0.0"},
		{"https://github.com/owner/repo/", "v1.0.0", "https://github.com/owner/repo/releases/tag/v1.0.0"},
	}
	for _, tt := range tests {
		if got := releaseTagURL(tt.repoURL, tt.version); got != tt.want {
			t.Errorf("releaseTagURL(%q, %q) = %q, want %q", tt.repoURL, tt.version, got, tt.want)
		}
	}
}

func TestBuildSeparateMessages(t *testing.T) {
	events := []Event{
		{RepoName: "Terraform Provider for Cisco CML2", RepoURL: "https://github.com/CiscoDevNet/terraform-provider-cml2", Kind: KindRelease, Previous: "v0.9.0", Current: "v0.9.1"},
		{RepoName: "WordPress", RepoURL: "https://github.com/WordPress/WordPress", Kind: KindTag, Previous: "7.0.1", Current: "7.0.2"},
	}

	messages, err := BuildSeparateMessages(events, repoconfig.Settings{Mention: "foobar@example.com"})
	if err != nil {
		t.Fatalf("BuildSeparateMessages() error = %v", err)
	}
	if len(messages) != 2 {
		t.Fatalf("len(messages) = %d, want 2", len(messages))
	}
	for i, wantSubject := range []string{"Terraform Provider for Cisco CML2", "WordPress"} {
		if messages[i].Subject != wantSubject {
			t.Errorf("messages[%d].Subject = %q, want %q", i, messages[i].Subject, wantSubject)
		}
		if !strings.Contains(messages[i].Body, events[i].Current) {
			t.Errorf("messages[%d].Body = %q, want the corresponding update", i, messages[i].Body)
		}
		if len(messages[i].Mentions) != 1 {
			t.Errorf("len(messages[%d].Mentions) = %d, want 1", i, len(messages[i].Mentions))
		}
	}
}

func TestBuildMessageUsesSingularUpdateSubject(t *testing.T) {
	events := []Event{{RepoName: "Example", RepoURL: "https://github.com/example/repo", Kind: KindRelease, Previous: "v1.0.0", Current: "v2.0.0"}}
	msg, err := BuildMessage(events, repoconfig.Settings{})
	if err != nil {
		t.Fatalf("BuildMessage() error = %v", err)
	}
	if msg.Subject != "GitHub Update (1 update)" {
		t.Errorf("Subject = %q, want %q", msg.Subject, "GitHub Update (1 update)")
	}
}

func TestBuildMessageMentions(t *testing.T) {
	events := []Event{{RepoName: "Example", RepoURL: "https://github.com/example/repo", Kind: KindRelease, Previous: "v1.0.0", Current: "v2.0.0"}}

	msg, err := BuildMessage(events, repoconfig.Settings{Mention: "foobar@example.com, U0123456:Someone"})
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
	events := []Event{{RepoName: "Example", RepoURL: "https://github.com/example/repo", Kind: KindRelease, Previous: "v1.0.0", Current: "v2.0.0"}}

	msg, err := BuildMessage(events, repoconfig.Settings{Mention: "  "})
	if err != nil {
		t.Fatalf("BuildMessage() error = %v", err)
	}
	if len(msg.Mentions) != 0 {
		t.Errorf("Mentions = %+v, want none for a blank mention setting", msg.Mentions)
	}
}
