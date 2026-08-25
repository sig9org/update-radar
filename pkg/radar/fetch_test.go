package radar

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sig9org/update-radar/internal/githubapi"
)

func TestFetchGitHub(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/repos/owner/repo/releases/latest":
			_, _ = w.Write([]byte(`{"tag_name":"v1.2.3","name":"Release 1.2.3","html_url":"https://github.com/owner/repo/releases/tag/v1.2.3"}`))
		case "/repos/owner/repo/tags":
			_, _ = w.Write([]byte(`[{"name":"v1.2.4","commit":{"sha":"abc123"}}]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	previous := githubapi.BaseURL
	githubapi.BaseURL = server.URL
	t.Cleanup(func() { githubapi.BaseURL = previous })

	got, err := FetchGitHub(context.Background(), "https://github.com/owner/repo", GitHubOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got.Repository != "owner/repo" || got.Release == nil || got.Release.TagName != "v1.2.3" {
		t.Fatalf("unexpected release result: %#v", got)
	}
	if got.Tag == nil || got.Tag.Name != "v1.2.4" || got.Tag.CommitSHA != "abc123" {
		t.Fatalf("unexpected tag result: %#v", got)
	}
}

func TestFetchWeb(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("version 1"))
	}))
	defer server.Close()

	got, err := FetchWeb(context.Background(), server.URL, WebOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got.URL != server.URL || got.SHA256 == "" {
		t.Fatalf("unexpected web result: %#v", got)
	}
}

func TestFetchGitHubRejectsInvalidURL(t *testing.T) {
	if _, err := FetchGitHub(context.Background(), "not-a-github-url", GitHubOptions{}); err == nil {
		t.Fatal("FetchGitHub() accepted an invalid repository URL")
	}
}
