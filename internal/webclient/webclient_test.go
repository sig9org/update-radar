package webclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchReturnsStableHash(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("hello world"))
	}))
	defer srv.Close()

	client := NewClient()
	got, err := client.Fetch(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	// SHA-256 of "hello world"
	want := "b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9"
	if got != want {
		t.Errorf("Fetch() = %q, want %q", got, want)
	}
}

func TestFetchDetectsContentChange(t *testing.T) {
	body := "version 1"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(body))
	}))
	defer srv.Close()

	client := NewClient()
	first, err := client.Fetch(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}

	body = "version 2"
	second, err := client.Fetch(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}

	if first == second {
		t.Errorf("Fetch() returned the same hash %q for different content", first)
	}
}

func TestFetchSendsUserAgent(t *testing.T) {
	var gotUA string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
	}))
	defer srv.Close()

	client := NewClient()
	if _, err := client.Fetch(context.Background(), srv.URL); err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if gotUA != userAgent {
		t.Errorf("User-Agent = %q, want %q", gotUA, userAgent)
	}
}

func TestFetchUnexpectedStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	client := NewClient()
	if _, err := client.Fetch(context.Background(), srv.URL); err == nil {
		t.Fatal("Fetch() error = nil, want error for 500 status")
	}
}

func TestFetchInvalidURL(t *testing.T) {
	client := NewClient()
	if _, err := client.Fetch(context.Background(), "://not-a-url"); err == nil {
		t.Fatal("Fetch() error = nil, want error for an invalid URL")
	}
}
