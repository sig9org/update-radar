// Package scraper retrieves rendered Cisco Software Download release pages.
package scraper

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/chromedp"
	"github.com/sig9org/update-radar/internal/debugx"
	"github.com/sig9org/update-radar/internal/model"
)

const releaseTreeSelector = ".p-tree-root-children"

// Options controls browser startup and page retrieval.
type Options struct {
	Timeout   time.Duration
	Headless  bool
	UserAgent string
	Log       io.Writer
}

// Browser renders pages in a local Chrome or Chromium process.
type Browser struct {
	ctx       context.Context
	cancel    context.CancelFunc
	allocStop context.CancelFunc
	timeout   time.Duration
	userAgent string
	fetchMu   sync.Mutex
}

// Pool owns independent Chrome processes. A browser is leased to at most one
// Fetch call at a time, so pages never share tabs, browser storage, service
// workers, or other origin-scoped state while sites are checked concurrently.
type Pool struct {
	browsers  []*Browser
	available chan *Browser
}

// Open starts a browser. Its user interface is displayed unless Headless is
// enabled.
func Open(ctx context.Context, opts Options) (*Browser, error) {
	allocOpts := append([]chromedp.ExecAllocatorOption(nil), chromedp.DefaultExecAllocatorOptions[:]...)
	if opts.Headless {
		allocOpts = append(allocOpts,
			chromedp.Flag("headless", "new"),
			chromedp.Flag("hide-scrollbars", true),
			chromedp.Flag("mute-audio", true),
		)
	} else {
		allocOpts = append(allocOpts, chromedp.Flag("headless", false))
	}
	if strings.TrimSpace(opts.UserAgent) != "" {
		allocOpts = append(allocOpts, chromedp.UserAgent(strings.TrimSpace(opts.UserAgent)))
	}
	if opts.Log != nil {
		allocOpts = append(allocOpts, chromedp.CombinedOutput(opts.Log))
	}
	allocCtx, allocStop := chromedp.NewExecAllocator(ctx, allocOpts...)
	browserCtx, cancel := chromedp.NewContext(allocCtx)
	if err := chromedp.Run(browserCtx); err != nil {
		cancel()
		allocStop()
		return nil, fmt.Errorf("launch browser: %w", err)
	}
	var detectedUserAgent string
	if err := chromedp.Run(browserCtx, chromedp.Evaluate("navigator.userAgent", &detectedUserAgent)); err != nil {
		cancel()
		allocStop()
		return nil, fmt.Errorf("read browser user agent: %w", err)
	}
	return &Browser{
		ctx: browserCtx, cancel: cancel, allocStop: allocStop,
		timeout: opts.Timeout, userAgent: browserUserAgent(opts.UserAgent, detectedUserAgent),
	}, nil
}

// OpenPool starts size independent Chrome processes. Each ExecAllocator uses
// its own temporary user-data directory, which chromedp removes when closed.
func OpenPool(ctx context.Context, opts Options, size int) (*Pool, error) {
	if size < 1 {
		return nil, fmt.Errorf("browser pool size must be greater than zero")
	}
	pool := &Pool{
		browsers:  make([]*Browser, 0, size),
		available: make(chan *Browser, size),
	}
	// Start browsers one at a time. Concurrent Chrome startup is considerably
	// less reliable on macOS and does not improve page-fetch concurrency.
	for index := 0; index < size; index++ {
		browser, err := Open(ctx, opts)
		if err != nil {
			// Chrome can occasionally fail during rapid consecutive launches on
			// macOS. Retry once before tearing down browsers already started.
			timer := time.NewTimer(250 * time.Millisecond)
			select {
			case <-timer.C:
				browser, err = Open(ctx, opts)
			case <-ctx.Done():
				timer.Stop()
				err = ctx.Err()
			}
			if err != nil {
				pool.Close()
				return nil, fmt.Errorf("launch browser worker %d: %w", index+1, err)
			}
		}
		pool.browsers = append(pool.browsers, browser)
		pool.available <- browser
	}
	return pool, nil
}

func browserUserAgent(configured, detected string) string {
	if configured = strings.TrimSpace(configured); configured != "" {
		return configured
	}
	return strings.ReplaceAll(detected, "HeadlessChrome", "Chrome")
}

// UserAgent returns the value applied to monitored pages.
func (b *Browser) UserAgent() string {
	if b == nil {
		return ""
	}
	return b.userAgent
}

// UserAgent returns the value applied by the pool's browser workers.
func (p *Pool) UserAgent() string {
	if p == nil || len(p.browsers) == 0 {
		return ""
	}
	return p.browsers[0].UserAgent()
}

// MaxConcurrentFetches reports the safe concurrency for the shared Chrome
// connection. Chrome 151 on macOS has been observed to reuse target state when
// multiple rendered pages are navigated concurrently, so fetches are
// deliberately serialized to protect snapshot correctness.
func (b *Browser) MaxConcurrentFetches() int { return 1 }

// MaxConcurrentFetches reports the number of independent browser workers.
func (p *Pool) MaxConcurrentFetches() int {
	if p == nil {
		return 0
	}
	return len(p.browsers)
}

// Close stops the browser and removes its temporary profile.
func (b *Browser) Close() {
	if b == nil {
		return
	}
	if b.cancel != nil {
		b.cancel()
	}
	if b.allocStop != nil {
		b.allocStop()
	}
}

// Close stops every browser process in the pool.
func (p *Pool) Close() {
	if p == nil {
		return
	}
	for _, browser := range p.browsers {
		browser.Close()
	}
	p.browsers = nil
}

// Fetch leases one independent browser for the duration of a site check.
func (p *Pool) Fetch(ctx context.Context, site model.Site) (model.Snapshot, error) {
	if p == nil || p.available == nil {
		return model.Snapshot{}, errors.New("browser pool is not initialized")
	}
	var browser *Browser
	select {
	case browser = <-p.available:
	case <-ctx.Done():
		return model.Snapshot{}, ctx.Err()
	}
	defer func() { p.available <- browser }()
	return browser.Fetch(ctx, site)
}

// Fetch loads a site and extracts its Suggested, Latest, and Deferred releases.
func (b *Browser) Fetch(ctx context.Context, site model.Site) (model.Snapshot, error) {
	b.fetchMu.Lock()
	defer b.fetchMu.Unlock()
	timedCtx, timeoutCancel := context.WithTimeout(b.ctx, b.timeout)
	defer timeoutCancel()
	stopCancelForwarding := context.AfterFunc(ctx, timeoutCancel)
	defer stopCancelForwarding()
	var html string
	var finalURL string
	actions := []chromedp.Action{
		chromedp.Navigate(site.URL),
	}
	if b.userAgent != "" {
		actions = append([]chromedp.Action{emulation.SetUserAgentOverride(b.userAgent).WithAcceptLanguage("en-US,en;q=0.9")}, actions...)
	}
	if err := chromedp.Run(timedCtx, actions...); err != nil {
		return model.Snapshot{}, fmt.Errorf("navigate to %s: %w", site.URL, err)
	}
	if err := chromedp.Run(timedCtx, chromedp.WaitReady(releaseTreeSelector)); err != nil {
		return model.Snapshot{}, fmt.Errorf("wait for Cisco release tree: %w", err)
	}
	if err := chromedp.Run(timedCtx,
		chromedp.WaitReady(".p-tree-node-label"),
		chromedp.Sleep(500*time.Millisecond),
	); err != nil {
		return model.Snapshot{}, fmt.Errorf("wait for Cisco release entries: %w", err)
	}
	toggles, expanded, err := expandReleaseTree(timedCtx)
	if err != nil {
		return model.Snapshot{}, fmt.Errorf("expand Cisco release tree: %w", err)
	}
	debugx.Printf("Cisco release tree: found %d toggle control(s), expanded %d collapsed node(s)", toggles, expanded)
	if err := chromedp.Run(timedCtx,
		chromedp.Sleep(500*time.Millisecond),
		chromedp.Location(&finalURL),
		chromedp.OuterHTML("html", &html),
	); err != nil {
		return model.Snapshot{}, fmt.Errorf("read rendered page: %w", err)
	}
	if !sameDownloadPage(site.URL, finalURL) {
		return model.Snapshot{}, fmt.Errorf("unexpected rendered page: requested %s, got %s", site.URL, finalURL)
	}
	snapshot, err := Parse(strings.NewReader(html), time.Now().UTC())
	if err != nil {
		return model.Snapshot{}, err
	}
	if snapshot.ProductName == "Unknown Product" {
		return model.Snapshot{}, errors.New("the rendered page did not contain a product title")
	}
	if len(snapshot.Suggested) == 0 && len(snapshot.Latest) == 0 && len(snapshot.Deferred) == 0 {
		return model.Snapshot{}, errors.New("the page contained no Suggested Release, Latest Release, or Deferred Release entries")
	}
	return snapshot, nil
}

// expandReleaseTree opens collapsed tree nodes so releases loaded behind a
// toggler, including Deferred Release child versions, are present in the DOM.
func expandReleaseTree(ctx context.Context) (int, int, error) {
	const expandCollapsed = `(function () {
		const root = document.querySelector("ul.p-tree-root-children");
		if (!root) return { found: 0, clicked: 0 };
		const toggles = new Set(root.querySelectorAll(
			'.p-tree-node-toggle-button, [class*="toggler"]'
		));
		for (const item of root.querySelectorAll('[role="treeitem"][aria-expanded="false"]')) {
			const toggle = item.querySelector('.p-tree-node-toggle-button, [class*="toggler"]');
			if (toggle) toggles.add(toggle);
		}
		let clicked = 0;
		for (const node of toggles) {
			if (node.dataset.updateRadarExpanded === "true") continue;
			const item = node.closest('li.p-tree-node, [role="treeitem"]');
			const markup = (typeof node.className === "string" ? node.className : "") + " " + node.innerHTML;
			let isCollapsed = node.getAttribute("aria-expanded") === "false" ||
				(item && item.getAttribute("aria-expanded") === "false") ||
				/chevron-right|angle-right|plus|collapsed|closed/.test(markup);
			if (!isCollapsed && item) {
				const children = Array.from(item.children).find((child) =>
					child.matches('.p-tree-node-children, [role="group"]')
				);
				isCollapsed = !children || children.hidden || getComputedStyle(children).display === "none";
			}
			if (isCollapsed) {
				node.dataset.updateRadarExpanded = "true";
				node.click();
				clicked++;
			}
		}
		return { found: toggles.size, clicked: clicked };
	})()`
	type expansionResult struct {
		Found   int `json:"found"`
		Clicked int `json:"clicked"`
	}
	found := 0
	expanded := 0
	for attempt := 0; attempt < 8; attempt++ {
		var result expansionResult
		if err := chromedp.Run(ctx, chromedp.Evaluate(expandCollapsed, &result)); err != nil {
			return found, expanded, err
		}
		if result.Found > found {
			found = result.Found
		}
		if result.Clicked == 0 {
			return found, expanded, nil
		}
		expanded += result.Clicked
		select {
		case <-ctx.Done():
			return found, expanded, ctx.Err()
		case <-time.After(250 * time.Millisecond):
		}
	}
	return found, expanded, nil
}

func sameDownloadPage(requested, rendered string) bool {
	want, wantErr := url.Parse(requested)
	got, gotErr := url.Parse(rendered)
	if wantErr != nil || gotErr != nil {
		return false
	}
	wantPath := strings.TrimRight(want.EscapedPath(), "/")
	gotPath := strings.TrimRight(got.EscapedPath(), "/")
	return strings.EqualFold(want.Hostname(), got.Hostname()) &&
		(gotPath == wantPath || strings.HasPrefix(gotPath, wantPath+"/"))
}

// Parse extracts release information from rendered Cisco page HTML.
func Parse(r io.Reader, fetchedAt time.Time) (model.Snapshot, error) {
	doc, err := goquery.NewDocumentFromReader(r)
	if err != nil {
		return model.Snapshot{}, fmt.Errorf("parse rendered page: %w", err)
	}
	productName := strings.TrimSpace(doc.Find("h2#release-product-title").First().Text())
	if productName == "" {
		productName = "Unknown Product"
	}
	result := model.Snapshot{ProductName: productName, FetchedAt: fetchedAt}
	root := doc.Find("ul.p-tree-root-children").First()
	suggestedFound := false
	latestFound := false
	deferredFound := false
	root.Find("li.p-tree-node").EachWithBreak(func(_ int, node *goquery.Selection) bool {
		text := node.Text()
		if !suggestedFound && strings.Contains(text, "Suggested Release") {
			result.Suggested = versions(node)
			suggestedFound = true
		}
		if !latestFound && strings.Contains(text, "Latest Release") {
			result.Latest = versions(node)
			latestFound = true
		}
		if !deferredFound && strings.Contains(text, "Deferred Release") {
			result.Deferred = versions(node)
			deferredFound = true
		}
		return !suggestedFound || !latestFound || !deferredFound
	})
	return result, nil
}

func versions(section *goquery.Selection) []string {
	seen := map[string]struct{}{}
	result := []string{}
	// Search the entire section instead of only its first child list. Cisco's
	// rendered tree can use a different wrapper for nested releases, such as
	// APIC's Deferred Release 2.3 -> 2.3(1e).
	section.Find("span.p-tree-node-label").Each(func(_ int, label *goquery.Selection) {
		value := strings.TrimSpace(label.Text())
		for _, suffix := range []string{"Suggested Release", "Latest Release", "Deferred Release", "All Release"} {
			value = strings.TrimSpace(strings.TrimSuffix(value, suffix))
		}
		if value == "" {
			return
		}
		if _, exists := seen[value]; exists {
			return
		}
		seen[value] = struct{}{}
		result = append(result, value)
	})
	return result
}
