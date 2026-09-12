// Package feed checks RSS, Atom, and JSON Feed documents.
package feed

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Site struct {
	Name string `yaml:"name"`
	URL  string `yaml:"url"`
}
type Item struct {
	ID, Title, URL, Summary, Content, Published, Updated string `yaml:",omitempty"`
}
type State struct {
	LastModified string `yaml:"last_modified,omitempty"`
	ETag         string `yaml:"etag,omitempty"`
	BodyHash     string `yaml:"body_hash,omitempty"`
	Items        []Item `yaml:"items,omitempty"`
}
type Event struct {
	Site                Site
	Added, Changed      []Item
	LastModifiedChanged bool
	BodyChanged         bool
}

func (e Event) ChangedAny() bool {
	return len(e.Added) > 0 || len(e.Changed) > 0 || e.LastModifiedChanged || e.BodyChanged
}

type Client struct {
	HTTPClient *http.Client
	UserAgent  string
}

func NewClient(timeout time.Duration, userAgent string) *Client {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &Client{HTTPClient: &http.Client{Timeout: timeout}, UserAgent: userAgent}
}

func (c *Client) Check(ctx context.Context, site Site, previous State) (State, Event, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, site.URL, nil)
	if err != nil {
		return previous, Event{}, err
	}
	ua := c.UserAgent
	if strings.TrimSpace(ua) == "" {
		ua = "update-radar/1.0 (+https://github.com/sig9org/update-radar)"
	}
	req.Header.Set("User-Agent", ua)
	if previous.LastModified != "" {
		req.Header.Set("If-Modified-Since", previous.LastModified)
	}
	if previous.ETag != "" {
		req.Header.Set("If-None-Match", previous.ETag)
	}
	client := c.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return previous, Event{}, fmt.Errorf("feed: request %s: %w", site.URL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotModified {
		return previous, Event{}, nil
	}
	if resp.StatusCode != http.StatusOK {
		return previous, Event{}, fmt.Errorf("feed: get %s: unexpected status %d", site.URL, resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return previous, Event{}, fmt.Errorf("feed: read %s: %w", site.URL, err)
	}
	items, err := parse(body, resp.Header.Get("Content-Type"))
	if err != nil {
		return previous, Event{}, fmt.Errorf("feed: parse %s: %w", site.URL, err)
	}
	next := State{LastModified: resp.Header.Get("Last-Modified"), ETag: resp.Header.Get("ETag"), BodyHash: hash(body), Items: items}
	if next.LastModified == "" {
		next.LastModified = previous.LastModified
	}
	if len(previous.Items) == 0 {
		return next, Event{}, nil
	}
	old := make(map[string]Item, len(previous.Items))
	for _, x := range previous.Items {
		old[key(x)] = x
	}
	var added, changed []Item
	for _, x := range items {
		k := key(x)
		before, ok := old[k]
		if !ok {
			added = append(added, x)
		} else if fingerprint(before) != fingerprint(x) {
			changed = append(changed, x)
		}
	}
	return next, Event{Site: site, Added: added, Changed: changed,
		LastModifiedChanged: previous.LastModified != "" && next.LastModified != previous.LastModified,
		BodyChanged:         previous.BodyHash != "" && previous.BodyHash != next.BodyHash}, nil
}

func key(x Item) string {
	if x.ID != "" {
		return x.ID
	}
	if x.URL != "" {
		return x.URL
	}
	return x.Title
}
func fingerprint(x Item) string { b, _ := json.Marshal(x); return hash(b) }
func hash(b []byte) string      { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }

type rss struct {
	Items []struct {
		GUID        string `xml:"guid"`
		Title       string `xml:"title"`
		Link        string `xml:"link"`
		Description string `xml:"description"`
		PubDate     string `xml:"pubDate"`
		Content     string `xml:"encoded"`
	} `xml:"channel>item"`
}
type atom struct {
	Entries []struct {
		ID    string `xml:"id"`
		Title string `xml:"title"`
		Links []struct {
			Href string `xml:"href,attr"`
		} `xml:"link"`
		Summary   string `xml:"summary"`
		Content   string `xml:"content"`
		Published string `xml:"published"`
		Updated   string `xml:"updated"`
	} `xml:"entry"`
}
type jsonFeed struct {
	Items []struct {
		ID            string `json:"id"`
		URL           string `json:"url"`
		ExternalURL   string `json:"external_url"`
		Title         string `json:"title"`
		ContentHTML   string `json:"content_html"`
		ContentText   string `json:"content_text"`
		Summary       string `json:"summary"`
		DatePublished string `json:"date_published"`
		DateModified  string `json:"date_modified"`
	} `json:"items"`
}

func parse(body []byte, contentType string) ([]Item, error) {
	trim := strings.TrimSpace(string(body))
	if strings.HasPrefix(trim, "{") {
		var f jsonFeed
		if err := json.Unmarshal(body, &f); err != nil {
			return nil, err
		}
		out := make([]Item, 0, len(f.Items))
		for _, x := range f.Items {
			u := x.URL
			if u == "" {
				u = x.ExternalURL
			}
			content := x.ContentHTML
			if content == "" {
				content = x.ContentText
			}
			out = append(out, Item{x.ID, x.Title, u, x.Summary, content, x.DatePublished, x.DateModified})
		}
		return out, nil
	}
	if strings.Contains(strings.ToLower(contentType), "atom") || strings.Contains(trim, "<feed") {
		var f atom
		if err := xml.Unmarshal(body, &f); err != nil {
			return nil, err
		}
		out := make([]Item, 0, len(f.Entries))
		for _, x := range f.Entries {
			u := ""
			if len(x.Links) > 0 {
				u = x.Links[0].Href
			}
			out = append(out, Item{x.ID, x.Title, u, x.Summary, x.Content, x.Published, x.Updated})
		}
		return out, nil
	}
	var f rss
	if err := xml.Unmarshal(body, &f); err != nil {
		return nil, err
	}
	out := make([]Item, 0, len(f.Items))
	for _, x := range f.Items {
		id := x.GUID
		if id == "" {
			id = x.Link
		}
		content := x.Content
		if content == "" {
			content = x.Description
		}
		out = append(out, Item{id, x.Title, x.Link, x.Description, content, x.PubDate, ""})
	}
	return out, nil
}
