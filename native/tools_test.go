package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type bearerTransport struct{ token string }

func (b bearerTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	copy := request.Clone(request.Context())
	copy.Header.Set("Authorization", "Bearer "+b.token)
	return http.DefaultTransport.RoundTrip(copy)
}

func TestMCPAllMailToolsOverAuthenticatedHTTP(t *testing.T) {
	root := t.TempDir()
	t.Setenv("M365_ATTACHMENT_ROOT", root)
	if err := os.WriteFile(filepath.Join(root, "report.zip"), []byte("attachment"), 0600); err != nil {
		t.Fatal(err)
	}
	graph := testGraph(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Error("wrong Graph identity")
		}
		if strings.Contains(r.URL.Path, "failure") {
			w.WriteHeader(500)
			io.WriteString(w, `{"error":{"message":"PROVIDER_SECRET"}}`)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/attachments/file") {
			json.NewEncoder(w).Encode(map[string]any{"@odata.type": "#microsoft.graph.fileAttachment", "id": "file", "name": "notes.txt", "contentType": "text/plain", "size": 9999, "contentBytes": base64.StdEncoding.EncodeToString([]byte("hello"))})
			return
		}
		if strings.HasSuffix(r.URL.Path, "/attachments") {
			if r.Method == http.MethodPost {
				io.WriteString(w, `{"id":"attached-file"}`)
				return
			}
			io.WriteString(w, `{"value":[{"id":"file","name":"notes.txt","size":9999,"contentBytes":"MUST_NOT_LEAK"}]}`)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/move") {
			io.WriteString(w, `{"id":"moved-id"}`)
			return
		}
		if r.URL.Path == "/me/messages" {
			if r.Method == http.MethodPost {
				io.WriteString(w, `{"id":"new-draft"}`)
				return
			}
			io.WriteString(w, `{"value":[{"id":"message","subject":"hello"}]}`)
			return
		}
		io.WriteString(w, `{"id":"message","isDraft":true,"body":{"content":"hello"}}`)
	})
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	downloads := newDownloads()
	service := newMailTools(graph, downloads, logger)
	service.extract = func(_ context.Context, file fileAttachment) (extractionResult, error) { return extractAttachment(file) }
	server := newMCP(service)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	token := strings.Repeat("a", 64)
	listener, err := downloads.start(ctx, "127.0.0.1:0", authenticatedMCP(server, token))
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	endpoint := "http://" + listener.Addr().String() + "/mcp"
	unauthorized, err := http.Post(endpoint, "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	unauthorized.Body.Close()
	if unauthorized.StatusCode != 401 {
		t.Fatal("MCP was accessible without a local token")
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "native-test", Version: "1"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: endpoint, HTTPClient: &http.Client{Transport: bearerTransport{token}}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	listed, err := session.ListTools(ctx, nil)
	if err != nil || len(listed.Tools) != 12 {
		t.Fatalf("expected twelve tools: %v %v", listed, err)
	}
	cases := []struct {
		name  string
		args  map[string]any
		field string
	}{
		{"mail_search", map[string]any{"query": "hello"}, "messages"},
		{"mail_get", map[string]any{"message_id": "message"}, "message"},
		{"mail_create_draft", map[string]any{"to_recipients": []string{"user@example.com"}, "subject": "Report", "body": "Review this report."}, "draft_id"},
		{"mail_send_draft", map[string]any{"draft_id": "draft"}, "send_accepted"},
		{"mail_add_draft_attachment", map[string]any{"draft_id": "draft", "relative_path": "report.zip"}, "content_sha256"},
		{"mail_list_attachments", map[string]any{"message_id": "message"}, "attachments"},
		{"mail_read_attachment", map[string]any{"message_id": "message", "attachment_id": "file"}, "content"},
		{"mail_download_attachment", map[string]any{"message_id": "message", "attachment_id": "file"}, "download_url"},
		{"mail_mark_read", map[string]any{"message_id": "message", "is_read": false}, "is_read"},
		{"mail_archive", map[string]any{"message_id": "message"}, "archived"},
		{"mail_move", map[string]any{"message_id": "message", "destination_folder_id": "folder"}, "destination_folder_id"},
		{"mail_set_category", map[string]any{"message_id": "message", "categories": []string{}}, "categories"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: tc.name, Arguments: tc.args})
			if err != nil {
				t.Fatal(err)
			}
			if result.IsError || len(result.Content) == 0 {
				t.Fatalf("tool failed: %+v", result)
			}
			text, ok := result.Content[0].(*mcp.TextContent)
			if !ok {
				t.Fatal("missing JSON content")
			}
			var payload map[string]any
			if json.Unmarshal([]byte(text.Text), &payload) != nil || payload[tc.field] == nil {
				t.Fatalf("invalid tool result %s", text.Text)
			}
			if strings.Contains(text.Text, "MUST_NOT_LEAK") {
				t.Fatal("metadata exposed attachment bytes")
			}
			if tc.name == "mail_archive" || tc.name == "mail_move" {
				if payload["message_id"] != "moved-id" || payload["source_message_id"] != "message" {
					t.Fatal("lost moved message ID")
				}
			}
			if tc.name == "mail_read_attachment" || tc.name == "mail_download_attachment" {
				if payload["attachment"].(map[string]any)["content_length"] != float64(5) {
					t.Fatal("used provider metadata as decoded byte length")
				}
			}
			if tc.name == "mail_download_attachment" {
				address := payload["download_url"].(string)
				response, err := http.Get(address)
				if err != nil {
					t.Fatal(err)
				}
				content, _ := io.ReadAll(response.Body)
				response.Body.Close()
				if string(content) != "hello" || response.StatusCode != 200 {
					t.Fatal("download failed")
				}
				response, err = http.Get(address)
				if err != nil {
					t.Fatal(err)
				}
				response.Body.Close()
				if response.StatusCode != 404 {
					t.Fatal("download could be replayed")
				}
			}
		})
	}
	for _, name := range []string{"mail_get", "mail_mark_read"} {
		result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: map[string]any{"message_id": "failure"}})
		if err != nil || !result.IsError {
			t.Fatalf("missing sanitized failure: %v %v", result, err)
		}
		encoded, _ := json.Marshal(result)
		if strings.Contains(string(encoded), "PROVIDER_SECRET") || !strings.Contains(string(encoded), "ref ") {
			t.Fatal("unsafe error boundary")
		}
		if name == "mail_mark_read" && !strings.Contains(string(encoded), "Verify mailbox state") {
			t.Fatal("write failure omitted verification advice")
		}
	}
	if strings.Contains(logs.String(), "PROVIDER_SECRET") || strings.Contains(logs.String(), "test-token") || strings.Contains(logs.String(), "notes.txt") {
		t.Fatal("sensitive data leaked to audit log")
	}
	graph.token = func(context.Context) (string, error) { return "", signInRequiredError{} }
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "mail_get", Arguments: map[string]any{"message_id": "message"}})
	if err != nil || !result.IsError {
		t.Fatal("expired sign-in was not rejected")
	}
	encoded, _ := json.Marshal(result)
	if !strings.Contains(string(encoded), "run login") {
		t.Fatal("expired sign-in did not provide an actionable error")
	}
}
