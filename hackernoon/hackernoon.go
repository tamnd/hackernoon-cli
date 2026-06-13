// Package hackernoon is the library behind the hn2 command line:
// the HTTP client, request shaping, and the typed data models for Hackernoon.
//
// Data comes from the Hackernoon RSS feed (https://hackernoon.com/feed) and
// per-tag feeds (https://hackernoon.com/tagged/<tag>/feed). Both are public
// and require no API key.
package hackernoon

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// DefaultUserAgent identifies the client to Hackernoon.
const DefaultUserAgent = "hn2/dev (+https://github.com/tamnd/hackernoon-cli)"

// ErrNotFound is returned when a resource cannot be located.
var ErrNotFound = errors.New("not found")

// Config holds constructor parameters.
type Config struct {
	BaseURL   string
	UserAgent string
	Rate      time.Duration
	Retries   int
	Timeout   time.Duration
}

// DefaultConfig returns sensible defaults.
func DefaultConfig() Config {
	return Config{
		BaseURL:   "https://hackernoon.com",
		UserAgent: DefaultUserAgent,
		Rate:      200 * time.Millisecond,
		Retries:   5,
		Timeout:   30 * time.Second,
	}
}

// Client talks to Hackernoon over HTTP.
type Client struct {
	http      *http.Client
	baseURL   string
	userAgent string
	rate      time.Duration
	retries   int

	mu   sync.Mutex
	last time.Time
}

// NewClient returns a Client with the given config.
func NewClient(cfg Config) *Client {
	if cfg.BaseURL == "" {
		cfg.BaseURL = DefaultConfig().BaseURL
	}
	if cfg.UserAgent == "" {
		cfg.UserAgent = DefaultConfig().UserAgent
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = DefaultConfig().Timeout
	}
	return &Client{
		http:      &http.Client{Timeout: cfg.Timeout},
		baseURL:   strings.TrimRight(cfg.BaseURL, "/"),
		userAgent: cfg.UserAgent,
		rate:      cfg.Rate,
		retries:   cfg.Retries,
	}
}

// get fetches a URL with pacing and retries.
func (c *Client) get(ctx context.Context, rawURL string) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt <= c.retries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff(attempt)):
			}
		}
		body, retry, err := c.do(ctx, rawURL)
		if err == nil {
			return body, nil
		}
		lastErr = err
		if !retry {
			return nil, err
		}
	}
	return nil, fmt.Errorf("get %s: %w", rawURL, lastErr)
}

func (c *Client) do(ctx context.Context, rawURL string) ([]byte, bool, error) {
	c.pace()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Accept", "application/rss+xml, application/xml, text/xml, */*")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, true, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		return nil, true, fmt.Errorf("http %d", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("http %d", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, true, err
	}
	return b, false, nil
}

func (c *Client) pace() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.rate <= 0 {
		return
	}
	if wait := c.rate - time.Since(c.last); wait > 0 {
		time.Sleep(wait)
	}
	c.last = time.Now()
}

func backoff(attempt int) time.Duration {
	d := time.Duration(attempt) * 500 * time.Millisecond
	if d > 5*time.Second {
		d = 5 * time.Second
	}
	return d
}

// ─── RSS wire types ───────────────────────────────────────────────────────────

type rssFeed struct {
	XMLName xml.Name   `xml:"rss"`
	Channel rssChannel `xml:"channel"`
}

type rssChannel struct {
	Items []rssItem `xml:"item"`
}

type rssItem struct {
	Title       string   `xml:"title"`
	Link        string   `xml:"link"`
	PubDate     string   `xml:"pubDate"`
	Description string   `xml:"description"`
	Creator     string   `xml:"creator"`
	Categories  []string `xml:"category"`
	Content     string   `xml:"encoded"`
}

func (c *Client) fetchFeed(ctx context.Context, feedURL string, limit int) ([]Story, error) {
	body, err := c.get(ctx, feedURL)
	if err != nil {
		return nil, err
	}
	var feed rssFeed
	if err := xml.Unmarshal(body, &feed); err != nil {
		return nil, fmt.Errorf("parse feed %s: %w", feedURL, err)
	}
	items := feed.Channel.Items
	if limit > 0 && limit < len(items) {
		items = items[:limit]
	}
	out := make([]Story, 0, len(items))
	for i, it := range items {
		out = append(out, rssItemToStory(&it, i+1))
	}
	return out, nil
}

func rssItemToStory(it *rssItem, rank int) Story {
	summary := stripTags(it.Description)
	if len([]rune(summary)) > 200 {
		rs := []rune(summary)
		summary = string(rs[:199]) + "…"
	}
	tags := make([]string, 0, len(it.Categories))
	seen := map[string]bool{}
	for _, cat := range it.Categories {
		cat = strings.TrimSpace(cat)
		if cat != "" && !seen[cat] {
			seen[cat] = true
			tags = append(tags, cat)
		}
	}

	link := strings.TrimSuffix(it.Link, "?source=rss")

	return Story{
		Rank:    rank,
		Title:   stripCDATA(it.Title),
		Author:  stripCDATA(it.Creator),
		Date:    parseRSSDate(it.PubDate),
		Summary: summary,
		Tags:    strings.Join(tags, ","),
		URL:     link,
	}
}

// ─── Latest ──────────────────────────────────────────────────────────────────

// Latest returns the most recent stories from the main feed.
func (c *Client) Latest(ctx context.Context, limit int) ([]Story, error) {
	return c.fetchFeed(ctx, c.baseURL+"/feed", limit)
}

// ─── Search ──────────────────────────────────────────────────────────────────

// Search filters the main feed for stories matching the query string (title,
// tags, author, or summary). If no matches are found it falls back to the
// per-tag feed for the first query word.
func (c *Client) Search(ctx context.Context, query string, limit int) ([]Story, error) {
	// Try main feed first
	stories, err := c.Latest(ctx, 0)
	if err != nil {
		return nil, err
	}

	q := strings.ToLower(strings.TrimSpace(query))
	var matched []Story
	for _, s := range stories {
		if storyMatchesQuery(s, q) {
			matched = append(matched, s)
		}
	}

	// If no in-feed hits, try the per-tag feed for the first query word
	if len(matched) == 0 {
		word := strings.Fields(q)[0]
		tagFeed := c.baseURL + "/tagged/" + url.PathEscape(word) + "/feed"
		tagStories, tagErr := c.fetchFeed(ctx, tagFeed, 0)
		if tagErr == nil {
			for _, s := range tagStories {
				if storyMatchesQuery(s, q) {
					matched = append(matched, s)
				}
			}
			// if still nothing, return everything from the tag feed
			if len(matched) == 0 {
				matched = tagStories
			}
		}
	}

	if limit > 0 && limit < len(matched) {
		matched = matched[:limit]
	}
	// re-rank
	for i := range matched {
		matched[i].Rank = i + 1
	}
	return matched, nil
}

func storyMatchesQuery(s Story, q string) bool {
	haystack := strings.ToLower(s.Title + " " + s.Tags + " " + s.Author + " " + s.Summary)
	for _, word := range strings.Fields(q) {
		if !strings.Contains(haystack, word) {
			return false
		}
	}
	return true
}

// ─── Trending ─────────────────────────────────────────────────────────────────

// Trending returns stories from the feed with the most tags (as a proxy for
// popular/trending content). Falls back to latest when the feed is short.
func (c *Client) Trending(ctx context.Context, limit int) ([]Story, error) {
	stories, err := c.Latest(ctx, 0)
	if err != nil {
		return nil, err
	}

	// Sort by tag count descending (more tags = more cross-topic = more popular)
	sorted := make([]Story, len(stories))
	copy(sorted, stories)
	sortByTagCount(sorted)

	if limit > 0 && limit < len(sorted) {
		sorted = sorted[:limit]
	}
	for i := range sorted {
		sorted[i].Rank = i + 1
	}
	return sorted, nil
}

// sortByTagCount is a simple insertion sort (feed is small, <=20 items).
func sortByTagCount(ss []Story) {
	for i := 1; i < len(ss); i++ {
		key := ss[i]
		keyCount := strings.Count(key.Tags, ",")
		j := i - 1
		for j >= 0 && strings.Count(ss[j].Tags, ",") < keyCount {
			ss[j+1] = ss[j]
			j--
		}
		ss[j+1] = key
	}
}

// ─── helpers ─────────────────────────────────────────────────────────────────

func parseRSSDate(s string) string {
	formats := []string{
		time.RFC1123Z,
		time.RFC1123,
		"Mon, 02 Jan 2006 15:04:05 -0700",
		"Mon, 2 Jan 2006 15:04:05 -0700",
		"Mon, 02 Jan 2006 15:04:05 MST",
	}
	for _, f := range formats {
		if t, err := time.Parse(f, s); err == nil {
			return t.UTC().Format(time.RFC3339)
		}
	}
	return s
}

func stripCDATA(s string) string {
	s = strings.TrimPrefix(s, "<![CDATA[")
	s = strings.TrimSuffix(s, "]]>")
	return strings.TrimSpace(s)
}

func stripTags(s string) string {
	var b strings.Builder
	inTag := false
	for _, r := range s {
		switch {
		case r == '<':
			inTag = true
		case r == '>':
			inTag = false
		case !inTag:
			b.WriteRune(r)
		}
	}
	out := b.String()
	out = strings.ReplaceAll(out, "&amp;", "&")
	out = strings.ReplaceAll(out, "&lt;", "<")
	out = strings.ReplaceAll(out, "&gt;", ">")
	out = strings.ReplaceAll(out, "&quot;", `"`)
	out = strings.ReplaceAll(out, "&#39;", "'")
	out = strings.ReplaceAll(out, "&apos;", "'")
	out = strings.TrimSpace(out)
	return out
}
