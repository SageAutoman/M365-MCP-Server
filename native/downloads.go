package main

import (
	"context"
	"errors"
	"fmt"
	"mime"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

type downloadItem struct {
	file   fileAttachment
	expiry time.Time
}

type downloadStore struct {
	mu     sync.Mutex
	items  map[string]downloadItem
	bytes  int
	now    func() time.Time
	prefix string
}

func newDownloads() *downloadStore {
	return &downloadStore{items: map[string]downloadItem{}, now: time.Now}
}

func (d *downloadStore) prune() {
	for key, item := range d.items {
		if !d.now().Before(item.expiry) {
			delete(d.items, key)
			d.bytes -= len(item.file.Content)
		}
	}
}

func (d *downloadStore) issue(file fileAttachment) (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.prune()
	if d.prefix == "" || len(file.Content) > maxAttachmentBytes || len(d.items) >= 16 || d.bytes+len(file.Content) > 64<<20 {
		return "", errors.New("local download capacity is unavailable")
	}
	token := randomToken()
	d.items[token] = downloadItem{file, d.now().Add(5 * time.Minute)}
	d.bytes += len(file.Content)
	return d.prefix + "/downloads/" + token, nil
}

func (d *downloadStore) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	securityHeaders(w)
	if r.Method != http.MethodGet || r.URL.RawQuery != "" {
		http.NotFound(w, r)
		return
	}
	token := strings.TrimPrefix(r.URL.Path, "/downloads/")
	d.mu.Lock()
	d.prune()
	item, found := d.items[token]
	if found {
		delete(d.items, token)
		d.bytes -= len(item.file.Content)
	}
	d.mu.Unlock()
	if !found {
		http.NotFound(w, r)
		return
	}
	name := strings.ReplaceAll(item.file.Name, `\`, "/")
	parts := strings.Split(name, "/")
	name = parts[len(parts)-1]
	name = strings.Map(func(r rune) rune {
		if r < 32 || r == 127 {
			return -1
		}
		return r
	}, name)
	if name == "" {
		name = "attachment"
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": name}))
	w.Header().Set("Content-Length", fmt.Sprint(len(item.file.Content)))
	w.Write(item.file.Content)
}

func (d *downloadStore) start(ctx context.Context, address string, mcpHandler http.Handler) (net.Listener, error) {
	listener, err := net.Listen("tcp4", address)
	if err != nil {
		return nil, err
	}
	d.prefix = "http://" + listener.Addr().String()
	mux := http.NewServeMux()
	mux.Handle("/downloads/", d)
	if mcpHandler != nil {
		mux.Handle("/mcp", mcpHandler)
		mux.Handle("/mcp/", mcpHandler)
	}
	server := &http.Server{Handler: localBoundary(mux, listener.Addr().String()), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 60 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 16 << 10}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		server.Shutdown(shutdown)
	}()
	go func() { server.Serve(listener) }()
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				d.mu.Lock()
				d.prune()
				d.mu.Unlock()
			}
		}
	}()
	return listener, nil
}

func securityHeaders(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
}

func localBoundary(next http.Handler, address string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		securityHeaders(w)
		isDownload := strings.HasPrefix(r.URL.Path, "/downloads/")
		if r.Host != address || !isDownload && (r.Header.Get("Origin") != "" && r.Header.Get("Origin") != "http://"+address || r.Header.Get("Sec-Fetch-Site") == "cross-site") {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}
