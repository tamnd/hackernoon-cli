package hackernoon_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/tamnd/hackernoon-cli/hackernoon"
)

func rssFixture(items []string) string {
	var body string
	for _, item := range items {
		body += item
	}
	return `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0"><channel><title>HackerNoon</title><link>https://hackernoon.com</link>` + body + `</channel></rss>`
}

const item1 = `<item>
  <title>How to Build a REST API</title>
  <link>https://hackernoon.com/how-to-build-a-rest-api</link>
  <pubDate>Mon, 15 Jan 2024 10:00:00 +0000</pubDate>
  <description>A guide to building REST APIs with Go.</description>
  <dc:creator xmlns:dc="http://purl.org/dc/elements/1.1/">Alice</dc:creator>
  <category>go</category>
  <category>api</category>
</item>`

const item2 = `<item>
  <title>Understanding Kubernetes</title>
  <link>https://hackernoon.com/understanding-kubernetes</link>
  <pubDate>Tue, 16 Jan 2024 10:00:00 +0000</pubDate>
  <description>Deep dive into Kubernetes architecture.</description>
  <dc:creator xmlns:dc="http://purl.org/dc/elements/1.1/">Bob</dc:creator>
  <category>kubernetes</category>
</item>`

func newTestClient(t *testing.T, body string) (*hackernoon.Client, func()) {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") == "" {
			t.Error("request carried no User-Agent")
		}
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = fmt.Fprint(w, body)
	}))
	cfg := hackernoon.DefaultConfig()
	cfg.BaseURL = ts.URL
	cfg.Rate = 0
	return hackernoon.NewClient(cfg), ts.Close
}

func TestLatest(t *testing.T) {
	feed := rssFixture([]string{item1, item2})
	c, close := newTestClient(t, feed)
	defer close()

	stories, err := c.Latest(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(stories) != 2 {
		t.Fatalf("got %d stories, want 2", len(stories))
	}
	if stories[0].Title != "How to Build a REST API" {
		t.Errorf("Title = %q, want %q", stories[0].Title, "How to Build a REST API")
	}
	if stories[0].URL == "" {
		t.Error("URL is empty")
	}
}

func TestSearch(t *testing.T) {
	feed := rssFixture([]string{item1, item2})
	c, close := newTestClient(t, feed)
	defer close()

	stories, err := c.Search(context.Background(), "kubernetes", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(stories) != 1 {
		t.Fatalf("got %d stories for 'kubernetes', want 1", len(stories))
	}
}

func TestClientRetriesOn503(t *testing.T) {
	var hits int
	feed := rssFixture([]string{item1})
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if hits < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = fmt.Fprint(w, feed)
	}))
	defer ts.Close()

	cfg := hackernoon.DefaultConfig()
	cfg.BaseURL = ts.URL
	cfg.Rate = 0
	cfg.Retries = 5
	c := hackernoon.NewClient(cfg)

	start := time.Now()
	stories, err := c.Latest(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(stories) == 0 {
		t.Fatal("got 0 stories after recovery")
	}
	if hits != 3 {
		t.Errorf("server saw %d hits, want 3", hits)
	}
	if time.Since(start) < 500*time.Millisecond {
		t.Error("retries did not back off")
	}
}
