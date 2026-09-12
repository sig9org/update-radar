package feed

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCheckConditionalGetAndItemDiff(t *testing.T) {
	modified := "Mon, 02 Jan 2006 15:04:05 GMT"
	var gotIMS string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotIMS = r.Header.Get("If-Modified-Since")
		w.Header().Set("Content-Type", "application/rss+xml")
		w.Header().Set("Last-Modified", modified)
		if gotIMS == modified {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		_, _ = w.Write([]byte(`<rss><channel><item><guid>1</guid><title>First</title><link>https://example.test/1</link><description>old</description></item></channel></rss>`))
	}))
	defer server.Close()
	c := NewClient(0, "test")
	next, ev, err := c.Check(context.Background(), Site{Name: "test", URL: server.URL}, State{})
	if err != nil || ev.ChangedAny() || len(next.Items) != 1 {
		t.Fatalf("first check: next=%#v event=%#v err=%v", next, ev, err)
	}
	next, ev, err = c.Check(context.Background(), Site{Name: "test", URL: server.URL}, next)
	if err != nil || ev.ChangedAny() || gotIMS != modified {
		t.Fatalf("304 check: next=%#v event=%#v ims=%q err=%v", next, ev, gotIMS, err)
	}
}

func TestParseAtomAndJSONFeed(t *testing.T) {
	atomItems, err := parse([]byte(`<feed><entry><id>a</id><title>Atom</title><link href="https://example.test/a"/><content>body</content></entry></feed>`), "application/atom+xml")
	if err != nil || len(atomItems) != 1 || atomItems[0].Content != "body" {
		t.Fatalf("atom: %#v %v", atomItems, err)
	}
	jsonItems, err := parse([]byte(`{"version":"https://jsonfeed.org/version/1.1","items":[{"id":"j","title":"JSON","content_text":"body"}]}`), "application/feed+json")
	if err != nil || len(jsonItems) != 1 || jsonItems[0].Title != "JSON" {
		t.Fatalf("json: %#v %v", jsonItems, err)
	}
}
