package githubapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestParseOwnerRepo(t *testing.T) {
	tests := []struct {
		url       string
		wantOwner string
		wantRepo  string
		wantErr   bool
	}{
		{"https://github.com/owner/repo", "owner", "repo", false},
		{"https://github.com/owner/repo/", "owner", "repo", false},
		{"https://github.com/owner/repo.git", "owner", "repo", false},
		{"https://github.com/owner", "", "", true},
		{"not a url", "", "", true},
		{"", "", "", true},
	}
	for _, tt := range tests {
		owner, repo, err := ParseOwnerRepo(tt.url)
		if tt.wantErr {
			if err == nil {
				t.Errorf("ParseOwnerRepo(%q) error = nil, want error", tt.url)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseOwnerRepo(%q) unexpected error: %v", tt.url, err)
			continue
		}
		if owner != tt.wantOwner || repo != tt.wantRepo {
			t.Errorf("ParseOwnerRepo(%q) = (%q, %q), want (%q, %q)", tt.url, owner, repo, tt.wantOwner, tt.wantRepo)
		}
	}
}

func withTestServer(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	oldBase := BaseURL
	BaseURL = srv.URL
	t.Cleanup(func() { BaseURL = oldBase })

	return NewClient("")
}

func TestLatestRelease(t *testing.T) {
	client := withTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/owner/repo/releases/latest" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"tag_name":"v1.2.3","name":"v1.2.3","html_url":"https://github.com/owner/repo/releases/tag/v1.2.3"}`))
	})

	release, ok, err := client.LatestRelease(context.Background(), "owner", "repo")
	if err != nil {
		t.Fatalf("LatestRelease() error = %v", err)
	}
	if !ok {
		t.Fatal("LatestRelease() ok = false, want true")
	}
	if release.TagName != "v1.2.3" {
		t.Errorf("TagName = %q, want %q", release.TagName, "v1.2.3")
	}
}

func TestLatestReleaseNotFound(t *testing.T) {
	client := withTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	_, ok, err := client.LatestRelease(context.Background(), "owner", "repo")
	if err != nil {
		t.Fatalf("LatestRelease() error = %v", err)
	}
	if ok {
		t.Error("LatestRelease() ok = true, want false for a repository with no releases")
	}
}

func TestLatestTag(t *testing.T) {
	client := withTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/owner/repo/tags" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[{"name":"v2.0.0","commit":{"sha":"abc123"}}]`))
	})

	tag, ok, err := client.LatestTag(context.Background(), "owner", "repo")
	if err != nil {
		t.Fatalf("LatestTag() error = %v", err)
	}
	if !ok {
		t.Fatal("LatestTag() ok = false, want true")
	}
	if tag.Name != "v2.0.0" {
		t.Errorf("Name = %q, want %q", tag.Name, "v2.0.0")
	}
}

func TestLatestTagEmpty(t *testing.T) {
	client := withTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[]`))
	})

	_, ok, err := client.LatestTag(context.Background(), "owner", "repo")
	if err != nil {
		t.Fatalf("LatestTag() error = %v", err)
	}
	if ok {
		t.Error("LatestTag() ok = true, want false for a repository with no tags")
	}
}

func TestAuthorizationHeader(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	oldBase := BaseURL
	BaseURL = srv.URL
	defer func() { BaseURL = oldBase }()

	client := NewClient("my-token")
	_, _, _ = client.LatestRelease(context.Background(), "owner", "repo")

	if gotAuth != "Bearer my-token" {
		t.Errorf("Authorization header = %q, want %q", gotAuth, "Bearer my-token")
	}
}

func TestUnexpectedStatus(t *testing.T) {
	client := withTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	_, _, err := client.LatestRelease(context.Background(), "owner", "repo")
	if err == nil {
		t.Fatal("LatestRelease() error = nil, want error for 500 status")
	}
}
