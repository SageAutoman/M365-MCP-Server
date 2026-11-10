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
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestDraftValidationAndWriteBoundaries(t *testing.T) {
	var writes int
	graph := testGraph(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			io.WriteString(w, `{"id":"received","isDraft":false}`)
			return
		}
		writes++
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["body"].(map[string]any)["contentType"] != "Text" || r.URL.Path != "/me/messages" {
			t.Error("draft creation changed content type or mailbox boundary")
		}
		io.WriteString(w, `{"id":"new-draft"}`)
	})
	service := newMailTools(graph, newDownloads(), slog.New(slog.NewJSONHandler(io.Discard, nil)))
	for _, input := range []draftInput{
		{Subject: "subject", Body: "body"},
		{ToRecipients: []string{"User <user@example.com>"}, Subject: "subject", Body: "body"},
		{ToRecipients: []string{"user@example.com"}, Subject: "injected\r\nsubject", Body: "body"},
		{ToRecipients: []string{"user@example.com"}, Subject: "subject", Body: strings.Repeat("a", 100001)},
	} {
		if _, err := service.createDraft(context.Background(), input); err == nil {
			t.Error("invalid draft was accepted")
		}
	}
	if _, err := service.sendDraft(context.Background(), draftIDInput{"received"}); err == nil {
		t.Error("non-draft message was sent")
	}
	if writes != 0 {
		t.Fatal("rejected draft arguments performed a Graph write")
	}
	result, err := service.createDraft(context.Background(), draftInput{ToRecipients: []string{"user@example.com"}, Subject: "subject", Body: "body"})
	if err != nil || result["draft_id"] != "new-draft" || writes != 1 {
		t.Fatalf("draft creation failed: %v %v", result, err)
	}
}

func TestLocalArtifactBoundsAndDirectAttachment(t *testing.T) {
	root := t.TempDir()
	t.Setenv("M365_ATTACHMENT_ROOT", root)
	if err := os.WriteFile(filepath.Join(root, "report.zip"), []byte("report bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, relative := range []string{"../secret.zip", "/secret.zip", "C:/secret.zip", "report.zip:stream", "folder//report.zip", "report.exe", "report.zip."} {
		if _, err := localArtifact(relative); err == nil {
			t.Fatalf("unsafe artifact path accepted: %s", relative)
		}
	}
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.zip"), []byte("secret"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err == nil {
		if _, err := localArtifact("escape/secret.zip"); err == nil {
			t.Fatal("symlink escaped the attachment root")
		}
	}
	var attached bool
	graph := testGraph(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			io.WriteString(w, `{"id":"draft","isDraft":true}`)
			return
		}
		if r.URL.Path != "/me/messages/draft/attachments" || r.Method != http.MethodPost {
			t.Fatal("wrong attachment target")
		}
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if body["contentBytes"] != base64.StdEncoding.EncodeToString([]byte("report bytes")) || body["contentType"] != "application/zip" {
			t.Error("attachment payload differs from the bounded local artifact")
		}
		attached = true
		io.WriteString(w, `{"id":"attachment"}`)
	})
	service := newMailTools(graph, newDownloads(), slog.New(slog.NewJSONHandler(io.Discard, nil)))
	if _, err := service.addDraftAttachment(context.Background(), draftAttachmentInput{"draft", "report.zip", "application/pdf"}); err == nil || attached {
		t.Fatal("MIME mismatch was not rejected before writing")
	}
	result, err := service.addDraftAttachment(context.Background(), draftAttachmentInput{"draft", "report.zip", ""})
	if err != nil || !attached || result["content_length"] != len("report bytes") || len(result["content_sha256"].(string)) != 64 {
		t.Fatalf("direct attachment failed: %v %v", result, err)
	}
}

func TestAttachmentUploadURLAndChunking(t *testing.T) {
	for _, address := range []string{
		"http://outlook.office.com/api/AttachmentSessions(1)?token=x",
		"https://attacker.example/api/AttachmentSessions(1)?token=x",
		"https://outlook.office.com:444/api/AttachmentSessions(1)?token=x",
		"https://outlook.office.com/api/other?token=x",
		"https://outlook.office.com/api/AttachmentSessions(1)",
		"https://user@outlook.office.com/api/AttachmentSessions(1)?token=x",
	} {
		if validUploadURL(address) {
			t.Fatal("unsafe Outlook upload URL accepted")
		}
	}
	content := bytes.Repeat([]byte("x"), 3<<20)
	var uploaded bytes.Buffer
	var calls int
	graph := newGraph(func(context.Context) (string, error) {
		t.Fatal("upload requested a Graph token")
		return "", nil
	})
	graph.client.Transport = roundTripFunc(func(request *http.Request) (*http.Response, error) {
		calls++
		if request.Method != http.MethodPut || request.Header.Get("Authorization") != "" || request.Header.Get("Content-Range") == "" {
			t.Error("upload leaked credentials or lost its range")
		}
		io.Copy(&uploaded, request.Body)
		status := http.StatusOK
		if uploaded.Len() == len(content) {
			status = http.StatusCreated
		}
		return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(`{}`)), Header: make(http.Header)}, nil
	})
	if err := graph.uploadAttachment(context.Background(), "https://outlook.office.com/api/AttachmentSessions(1)?token=x", content); err != nil {
		t.Fatal(err)
	}
	if calls != 2 || !bytes.Equal(uploaded.Bytes(), content) {
		t.Fatal("chunked upload lost or repeated bytes")
	}
	calls = 0
	graph.client.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 503, Body: io.NopCloser(strings.NewReader(`{}`)), Header: make(http.Header)}, nil
	})
	if err := graph.uploadAttachment(context.Background(), "https://outlook.office.com/api/AttachmentSessions(1)?token=x", content); err == nil || calls != 1 {
		t.Fatal("ambiguous attachment write was retried")
	}
}
