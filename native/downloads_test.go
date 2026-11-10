package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestDownloadsSingleUseExpiryAndCapacity(t *testing.T) {
	store := newDownloads()
	store.prefix = "http://127.0.0.1:8000"
	now := time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return now }
	address, err := store.issue(fileAttachment{Name: "../unsafe\r\nname.txt", Content: []byte("hello")})
	if err != nil {
		t.Fatal(err)
	}
	var success atomic.Int32
	var group sync.WaitGroup
	for i := 0; i < 8; i++ {
		group.Go(func() {
			response := httptest.NewRecorder()
			store.ServeHTTP(response, httptest.NewRequest("GET", address, nil))
			if response.Code == http.StatusOK {
				success.Add(1)
				if response.Body.String() != "hello" || response.Header().Get("Cache-Control") != "no-store" || strings.ContainsAny(response.Header().Get("Content-Disposition"), "\r\n") {
					t.Error("unsafe download response")
				}
			}
		})
	}
	group.Wait()
	if success.Load() != 1 || store.bytes != 0 {
		t.Fatal("download was not atomically consumed")
	}
	address, _ = store.issue(fileAttachment{Name: "expired.txt", Content: []byte("x")})
	now = now.Add(5 * time.Minute)
	response := httptest.NewRecorder()
	store.ServeHTTP(response, httptest.NewRequest("GET", address, nil))
	if response.Code != 404 || store.bytes != 0 {
		t.Fatal("expired download retained")
	}
	for i := 0; i < 16; i++ {
		if _, err := store.issue(fileAttachment{Name: "x.txt", Content: []byte("x")}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.issue(fileAttachment{Name: "x.txt"}); err == nil {
		t.Fatal("unbounded download item count")
	}
}

func TestLocalHostAndOriginBoundary(t *testing.T) {
	handler := localBoundary(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) }), "127.0.0.1:8000")
	for _, tc := range []struct {
		host, origin string
		want         int
	}{{"127.0.0.1:8000", "", 204}, {"127.0.0.1:8000", "http://127.0.0.1:8000", 204}, {"evil.example:8000", "", 403}, {"127.0.0.1:8000", "https://evil.example", 403}, {"127.0.0.1:8000", "null", 403}} {
		request := httptest.NewRequest("POST", "http://"+tc.host+"/mcp", nil)
		request.Header.Set("Origin", tc.origin)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != tc.want {
			t.Errorf("host=%q origin=%q status=%d", tc.host, tc.origin, response.Code)
		}
	}
	request := httptest.NewRequest("GET", "http://127.0.0.1:8000/downloads/secret-ticket", nil)
	request.Header.Set("Sec-Fetch-Site", "cross-site")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != 204 {
		t.Fatal("a capability download could not be opened from a client UI")
	}
}
