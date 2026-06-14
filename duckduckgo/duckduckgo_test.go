package duckduckgo

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestGet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") == "" {
			t.Error("request carried no User-Agent")
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	c := NewClient()
	c.Rate = 0

	body, err := c.Get(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "ok" {
		t.Errorf("body = %q, want %q", body, "ok")
	}
}

func TestGetRetriesOn503(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if hits < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte("recovered"))
	}))
	defer srv.Close()

	c := NewClient()
	c.Rate = 0
	c.Retries = 5

	start := time.Now()
	body, err := c.Get(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "recovered" {
		t.Errorf("body = %q after retries", body)
	}
	if hits != 3 {
		t.Errorf("server saw %d hits, want 3", hits)
	}
	if time.Since(start) < 500*time.Millisecond {
		t.Error("retries did not back off")
	}
}

func TestSearchSuccess(t *testing.T) {
	wire := wireResult{
		Heading:        "Python (programming language)",
		AbstractText:   "Python is a high-level, general-purpose programming language.",
		AbstractSource: "Wikipedia",
		AbstractURL:    "https://en.wikipedia.org/wiki/Python_(programming_language)",
		Type:           "A",
		Entity:         "",
		RelatedTopics: []struct {
			Text     string `json:"Text"`
			FirstURL string `json:"FirstURL"`
		}{
			{Text: "CPython", FirstURL: "https://duckduckgo.com/CPython"},
			{Text: "PyPy", FirstURL: "https://duckduckgo.com/PyPy"},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// verify required query params
		q := r.URL.Query()
		if q.Get("format") != "json" {
			t.Errorf("format param = %q, want json", q.Get("format"))
		}
		if q.Get("no_redirect") != "1" {
			t.Errorf("no_redirect param = %q, want 1", q.Get("no_redirect"))
		}
		if q.Get("no_html") != "1" {
			t.Errorf("no_html param = %q, want 1", q.Get("no_html"))
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(wire)
	}))
	defer srv.Close()

	c := NewClient()
	c.Rate = 0
	c.HTTP.Transport = rebaseTransport(srv.URL)

	ans, err := c.Search(context.Background(), "Python programming", true)
	if err != nil {
		t.Fatal(err)
	}
	if ans.Heading != "Python (programming language)" {
		t.Errorf("Heading = %q", ans.Heading)
	}
	if ans.Abstract == "" {
		t.Error("Abstract is empty")
	}
	if ans.Source != "Wikipedia" {
		t.Errorf("Source = %q, want Wikipedia", ans.Source)
	}
	if ans.Topics != 2 {
		t.Errorf("Topics = %d, want 2", ans.Topics)
	}
	if ans.Type != "A" {
		t.Errorf("Type = %q, want A", ans.Type)
	}
}

func TestSearchNoResult(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Empty response — no Heading, no AbstractText
		_, _ = w.Write([]byte(`{"Heading":"","AbstractText":"","Type":""}`))
	}))
	defer srv.Close()

	c := NewClient()
	c.Rate = 0
	c.HTTP.Transport = rebaseTransport(srv.URL)

	_, err := c.Search(context.Background(), "xyzzy1234567890", true)
	if err == nil {
		t.Fatal("expected error for empty result, got nil")
	}
}

func TestSearchSkipDisambig(t *testing.T) {
	var gotSkip string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotSkip = r.URL.Query().Get("skip_disambig")
		wire := wireResult{
			Heading:      "New York City",
			AbstractText: "New York is a city.",
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(wire)
	}))
	defer srv.Close()

	c := NewClient()
	c.Rate = 0
	c.HTTP.Transport = rebaseTransport(srv.URL)

	_, err := c.Search(context.Background(), "New York", true)
	if err != nil {
		t.Fatal(err)
	}
	if gotSkip != "1" {
		t.Errorf("skip_disambig = %q, want 1", gotSkip)
	}
}

func TestSearchNoSkipDisambig(t *testing.T) {
	var gotSkip string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotSkip = r.URL.Query().Get("skip_disambig")
		wire := wireResult{
			Heading:      "Mercury",
			AbstractText: "Mercury may refer to many things.",
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(wire)
	}))
	defer srv.Close()

	c := NewClient()
	c.Rate = 0
	c.HTTP.Transport = rebaseTransport(srv.URL)

	_, err := c.Search(context.Background(), "Mercury", false)
	if err != nil {
		t.Fatal(err)
	}
	if gotSkip != "" {
		t.Errorf("skip_disambig = %q, want empty (not sent)", gotSkip)
	}
}

// rebaseTransport returns an http.RoundTripper that rewrites every request to
// hit the given test server URL instead of the real host. This lets us use the
// real Client.Search without changing BaseURL.
type rebaseRoundTripper struct {
	base string
	orig http.RoundTripper
}

func (rt rebaseRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	req2 := req.Clone(req.Context())
	req2.URL.Scheme = "http"
	req2.URL.Host = rt.base[len("http://"):]
	return rt.orig.RoundTrip(req2)
}

func rebaseTransport(base string) http.RoundTripper {
	return rebaseRoundTripper{base: base, orig: http.DefaultTransport}
}
