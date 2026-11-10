package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const maxAttachmentBytes = 10 << 20
const maxGraphBytes = 16 << 20

type tokenSource func(context.Context) (string, error)

type graphClient struct {
	client *http.Client
	token  tokenSource
	base   string
	wait   func(context.Context, time.Duration) error
}

func newGraph(source tokenSource) *graphClient {
	return &graphClient{
		client: &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		}},
		token: source, base: "https://graph.microsoft.com/v1.0", wait: waitContext,
	}
}

func readBounded(r io.Reader, maxBytes int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxBytes {
		return nil, errors.New("response exceeds the byte limit")
	}
	return data, nil
}

func waitContext(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

type graphError struct{ status int }

func (e graphError) Error() string { return fmt.Sprintf("Microsoft Graph returned HTTP %d", e.status) }

func (g *graphClient) request(ctx context.Context, method, path string, params url.Values, payload any) (map[string]any, error) {
	parsed, err := url.Parse(path)
	if err != nil || parsed.IsAbs() || parsed.Host != "" || parsed.RawQuery != "" || parsed.Fragment != "" ||
		(parsed.Path != "/me" && !strings.HasPrefix(parsed.Path, "/me/")) {
		return nil, errors.New("Graph operations must use a relative /me path")
	}
	for _, segment := range strings.Split(parsed.Path, "/") {
		if segment == "." || segment == ".." {
			return nil, errors.New("Graph path traversal is not allowed")
		}
	}
	token, err := g.token(ctx)
	if err != nil {
		return nil, err
	}
	var body []byte
	if payload != nil {
		body, err = json.Marshal(payload)
		if err != nil {
			return nil, err
		}
	}
	address := g.base + path
	if len(params) > 0 {
		address += "?" + params.Encode()
	}
	for attempt := 0; attempt <= 3; attempt++ {
		req, err := http.NewRequestWithContext(ctx, method, address, bytes.NewReader(body))
		if err != nil {
			return nil, errors.New("cannot construct Graph request")
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Accept", "application/json")
		if payload != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		response, err := g.client.Do(req)
		if err != nil {
			if method != http.MethodGet || attempt == 3 {
				return nil, errors.New("Microsoft Graph connection failed")
			}
			if err := g.wait(ctx, time.Second<<attempt); err != nil {
				return nil, err
			}
			continue
		}
		data, readErr := readBounded(response.Body, maxGraphBytes)
		response.Body.Close()
		if readErr != nil {
			return nil, errors.New("Microsoft Graph response could not be read within the byte limit")
		}
		status := response.StatusCode
		retry := status == 429 || (method == http.MethodGet && (status == 408 || status == 500 || status == 502 || status == 503 || status == 504))
		if retry && attempt < 3 {
			delay := retryDelay(response.Header.Get("Retry-After"), attempt, time.Now())
			if delay > 30*time.Second {
				return nil, graphError{status}
			}
			if err := g.wait(ctx, delay); err != nil {
				return nil, err
			}
			continue
		}
		if status < 200 || status >= 300 {
			return nil, graphError{status}
		}
		result := map[string]any{}
		if len(data) != 0 && json.Unmarshal(data, &result) != nil {
			return nil, errors.New("Microsoft Graph returned an invalid response")
		}
		return result, nil
	}
	return nil, errors.New("Microsoft Graph retry limit reached")
}

func retryDelay(value string, attempt int, now time.Time) time.Duration {
	if seconds, err := strconv.ParseUint(strings.TrimSpace(value), 10, 64); err == nil {
		if seconds > 30 {
			return 31 * time.Second // The caller rejects waits above its deadline without retrying.
		}
		return time.Duration(seconds) * time.Second
	} else if errors.Is(err, strconv.ErrRange) {
		return 31 * time.Second
	}
	if date, err := http.ParseTime(value); err == nil {
		return max(0, date.Sub(now))
	}
	return time.Second << attempt
}

func resourceSegment(value string) (string, error) {
	if value == "" || value == "." || value == ".." || len(value) > 4096 {
		return "", errors.New("resource ID must contain between 1 and 4096 characters and must not be a traversal segment")
	}
	return url.PathEscape(value), nil
}
