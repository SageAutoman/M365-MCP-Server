package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func testGraph(t *testing.T, handler http.HandlerFunc) *graphClient {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	graph := newGraph(func(context.Context) (string, error) { return "test-token", nil })
	graph.base = server.URL
	graph.wait = func(context.Context, time.Duration) error { return nil }
	return graph
}

func TestGraphBoundaries(t *testing.T) {
	graph := testGraph(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Error("delegated token was not attached")
		}
		if r.URL.EscapedPath() != "/me/messages/a%2Fb%3F%23" {
			t.Errorf("resource ID was not escaped: %s", r.URL.EscapedPath())
		}
		io.WriteString(w, `{"id":"ok"}`)
	})
	for _, path := range []string{"https://evil.example/me", "//evil.example/me", "/users/alice/messages", "/me/../users", "/me/%2e%2e/users", "/me?x=1", "/me#fragment"} {
		if _, err := graph.request(context.Background(), "GET", path, nil, nil); err == nil {
			t.Errorf("accepted unsafe path %q", path)
		}
	}
	id, err := resourceSegment("a/b?#")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := graph.request(context.Background(), "GET", "/me/messages/"+id, nil, nil); err != nil {
		t.Fatal(err)
	}
}

func TestGraphDoesNotReplayAmbiguousWrites(t *testing.T) {
	calls := 0
	graph := testGraph(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(503)
		io.WriteString(w, "PROVIDER_SECRET")
	})
	_, err := graph.request(context.Background(), "POST", "/me/messages/id/move", nil, map[string]string{"destinationId": "archive"})
	if calls != 1 || err == nil || strings.Contains(err.Error(), "PROVIDER_SECRET") {
		t.Fatalf("unsafe write retry or error disclosure: calls=%d err=%v", calls, err)
	}
}

func TestGraphRetryAfterAndRedirects(t *testing.T) {
	if retryDelay("999999999999999999999999999", 0, time.Now()) <= 30*time.Second {
		t.Fatal("an overflowing Retry-After was shortened")
	}
	calls := 0
	graph := testGraph(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Retry-After", "120")
		w.WriteHeader(429)
	})
	_, err := graph.request(context.Background(), "GET", "/me/messages", nil, nil)
	if err == nil || calls != 1 {
		t.Fatal("long Retry-After must stop the request rather than shorten the delay")
	}
	waited := time.Duration(0)
	graph = testGraph(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.Header().Set("Retry-After", "2")
			w.WriteHeader(429)
			return
		}
		io.WriteString(w, `{}`)
	})
	calls = 0
	graph.wait = func(_ context.Context, delay time.Duration) error { waited = delay; return nil }
	if _, err := graph.request(context.Background(), "GET", "/me/messages", nil, nil); err != nil || calls != 2 || waited != 2*time.Second {
		t.Fatalf("retry failed: calls=%d delay=%v err=%v", calls, waited, err)
	}
	destination := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Error("followed Graph redirect") }))
	defer destination.Close()
	graph = testGraph(t, func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, destination.URL, 302) })
	if _, err := graph.request(context.Background(), "GET", "/me", nil, nil); err == nil {
		t.Fatal("accepted Graph redirect")
	}
}

func TestSearchValidationAndQuerySeparation(t *testing.T) {
	params, err := searchParams(searchInput{Query: `"budget"`, DateFrom: "2026-11-01T08:00:00+08:00"})
	if err != nil || params.Get("$filter") != "" || params.Get("$orderby") != "" || !strings.Contains(params.Get("$search"), "received>=2026-11-01T00:00:00Z") {
		t.Fatalf("invalid keyword query: %v %v", params, err)
	}
	params, err = searchParams(searchInput{DateTo: "2026-11-10T00:00:00Z"})
	if err != nil || params.Get("$search") != "" || params.Get("$filter") == "" || params.Get("$orderby") == "" {
		t.Fatalf("invalid date-only query: %v %v", params, err)
	}
	zero := 0
	for _, args := range []searchInput{{Limit: &zero}, {DateFrom: "2026-11-10"}, {DateFrom: "2026-11-10T00:00:00Z", DateTo: "2026-11-01T00:00:00Z"}, {Query: strings.Repeat("字", 2049)}} {
		if _, err := searchParams(args); err == nil {
			t.Errorf("accepted invalid input: %+v", args)
		}
	}
	if _, err := readBounded(strings.NewReader("12345"), 4); err == nil {
		t.Fatal("unbounded response")
	}
	if escaped, _ := resourceSegment("abc+def/="); url.PathEscape("abc+def/=") != escaped {
		t.Fatal("ID was not escaped consistently")
	}
}
