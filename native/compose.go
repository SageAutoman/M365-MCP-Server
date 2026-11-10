package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/mail"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"
)

const maxSendAttachmentBytes = 20 << 20

type draftInput struct {
	ToRecipients  []string `json:"to_recipients"`
	Subject       string   `json:"subject"`
	Body          string   `json:"body"`
	CCRecipients  []string `json:"cc_recipients,omitempty"`
	BCCRecipients []string `json:"bcc_recipients,omitempty"`
}

type draftIDInput struct {
	DraftID string `json:"draft_id"`
}

type draftAttachmentInput struct {
	DraftID      string `json:"draft_id"`
	RelativePath string `json:"relative_path" jsonschema:"Path under the operator-configured M365_ATTACHMENT_ROOT"`
	ContentType  string `json:"content_type,omitempty" jsonschema:"Must match the file extension when supplied"`
}

func recipients(values []string, param string, required bool) ([]map[string]any, error) {
	if (required && len(values) == 0) || len(values) > 50 {
		return nil, inputError{param, "out_of_range", "provide at most 50 addresses per field and at least one To recipient"}
	}
	result := make([]map[string]any, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		address, err := mail.ParseAddress(value)
		if err != nil || len(value) > 320 || address.Address != value || !strings.Contains(value, "@") || strings.ContainsAny(value, "\r\n") {
			return nil, inputError{param, "invalid_address", "each recipient must be a plain email address"}
		}
		result = append(result, map[string]any{"emailAddress": map[string]string{"address": value}})
	}
	return result, nil
}

func (s *mailTools) createDraft(ctx context.Context, args draftInput) (map[string]any, error) {
	to, err := recipients(args.ToRecipients, "to_recipients", true)
	if err != nil {
		return nil, err
	}
	cc, err := recipients(args.CCRecipients, "cc_recipients", false)
	if err != nil {
		return nil, err
	}
	bcc, err := recipients(args.BCCRecipients, "bcc_recipients", false)
	if err != nil {
		return nil, err
	}
	if len(to)+len(cc)+len(bcc) > 100 {
		return nil, inputError{"to_recipients", "too_many_recipients", "at most 100 recipients in total"}
	}
	if strings.TrimSpace(args.Subject) == "" || utf8.RuneCountInString(args.Subject) > 255 || strings.ContainsAny(args.Subject, "\r\n\x00") {
		return nil, inputError{"subject", "invalid_text", "provide a single-line subject of 1 to 255 characters"}
	}
	if strings.TrimSpace(args.Body) == "" || utf8.RuneCountInString(args.Body) > 100000 || strings.ContainsRune(args.Body, 0) {
		return nil, inputError{"body", "invalid_text", "provide a plain-text body of 1 to 100000 characters"}
	}
	result, err := s.graph.request(ctx, http.MethodPost, "/me/messages", nil, map[string]any{
		"subject": args.Subject, "body": map[string]string{"contentType": "Text", "content": args.Body},
		"toRecipients": to, "ccRecipients": cc, "bccRecipients": bcc,
	})
	if err != nil {
		return nil, err
	}
	id, _ := result["id"].(string)
	if id == "" {
		return nil, errors.New("Microsoft Graph draft response has no ID")
	}
	return map[string]any{"draft_id": id, "created": true}, nil
}

func draftPath(id string) (string, error) {
	segment, err := resourceSegment(id)
	if err != nil {
		return "", inputError{"draft_id", "invalid_id", err.Error()}
	}
	return "/me/messages/" + segment, nil
}

func (s *mailTools) requireDraft(ctx context.Context, path string) error {
	result, err := s.graph.request(ctx, http.MethodGet, path, url.Values{"$select": {"id,isDraft"}}, nil)
	if err != nil {
		return err
	}
	if result["isDraft"] != true {
		return inputError{"draft_id", "not_a_draft", "the message must be an existing draft"}
	}
	return nil
}

func (s *mailTools) sendDraft(ctx context.Context, args draftIDInput) (map[string]any, error) {
	path, err := draftPath(args.DraftID)
	if err != nil {
		return nil, err
	}
	if err := s.requireDraft(ctx, path); err != nil {
		return nil, err
	}
	if _, err := s.graph.request(ctx, http.MethodPost, path+"/send", nil, map[string]any{}); err != nil {
		return nil, err
	}
	return map[string]any{"draft_id": args.DraftID, "send_accepted": true, "delivery_confirmed": false}, nil
}

func localArtifact(relative string) (fileAttachment, error) {
	rootPath := os.Getenv("M365_ATTACHMENT_ROOT")
	if !filepath.IsAbs(rootPath) {
		return fileAttachment{}, inputError{"relative_path", "root_not_configured", "set M365_ATTACHMENT_ROOT to an absolute operator-controlled directory"}
	}
	if len(relative) == 0 || len(relative) > 1024 || strings.ContainsAny(relative, ":\\") || strings.ContainsFunc(relative, unicode.IsControl) {
		return fileAttachment{}, inputError{"relative_path", "invalid_path", "use a relative path with forward slashes under M365_ATTACHMENT_ROOT"}
	}
	for _, segment := range strings.Split(relative, "/") {
		if segment == "" || segment == "." || segment == ".." || strings.TrimRight(segment, " .") != segment {
			return fileAttachment{}, inputError{"relative_path", "invalid_path", "empty, dot, and parent path segments are not allowed"}
		}
	}
	name := filepath.Base(relative)
	if utf8.RuneCountInString(name) > 255 {
		return fileAttachment{}, inputError{"relative_path", "invalid_name", "filename must not exceed 255 characters"}
	}
	types := map[string]string{
		".docx": "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		".xlsx": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
		".pptx": "application/vnd.openxmlformats-officedocument.presentationml.presentation",
		".zip":  "application/zip",
		".pdf":  "application/pdf",
	}
	contentType, ok := types[strings.ToLower(filepath.Ext(name))]
	if !ok {
		return fileAttachment{}, inputError{"relative_path", "unsupported_type", "allowed file types are PDF, DOCX, XLSX, PPTX, and ZIP"}
	}
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		return fileAttachment{}, errors.New("could not open the configured attachment root")
	}
	defer root.Close()
	file, err := root.Open(filepath.FromSlash(relative))
	if err != nil {
		return fileAttachment{}, inputError{"relative_path", "unavailable", "file is unavailable or escapes the attachment root"}
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() < 1 || info.Size() > maxSendAttachmentBytes {
		return fileAttachment{}, inputError{"relative_path", "invalid_size", "file must be regular and contain 1 byte to 20 MiB"}
	}
	content, err := readBounded(file, maxSendAttachmentBytes)
	if err != nil || int64(len(content)) != info.Size() {
		return fileAttachment{}, errors.New("attachment changed while reading or exceeded the size limit")
	}
	return fileAttachment{Name: name, ContentType: contentType, Content: content}, nil
}

func (s *mailTools) addDraftAttachment(ctx context.Context, args draftAttachmentInput) (map[string]any, error) {
	path, err := draftPath(args.DraftID)
	if err != nil {
		return nil, err
	}
	file, err := localArtifact(args.RelativePath)
	if err != nil {
		return nil, err
	}
	if args.ContentType != "" && args.ContentType != file.ContentType {
		return nil, inputError{"content_type", "type_mismatch", "must match the artifact file type"}
	}
	if err := s.requireDraft(ctx, path); err != nil {
		return nil, err
	}
	var attachmentID any
	if len(file.Content) < 3000000 {
		result, err := s.graph.request(ctx, http.MethodPost, path+"/attachments", nil, map[string]any{
			"@odata.type": "#microsoft.graph.fileAttachment", "name": file.Name, "contentType": file.ContentType,
			"contentBytes": base64.StdEncoding.EncodeToString(file.Content),
		})
		if err != nil {
			return nil, err
		}
		attachmentID, _ = result["id"].(string)
	} else {
		result, err := s.graph.request(ctx, http.MethodPost, path+"/attachments/createUploadSession", nil, map[string]any{
			"AttachmentItem": map[string]any{"attachmentType": "file", "name": file.Name, "contentType": file.ContentType, "size": len(file.Content)},
		})
		if err != nil {
			return nil, err
		}
		address, _ := result["uploadUrl"].(string)
		if err := s.graph.uploadAttachment(ctx, address, file.Content); err != nil {
			return nil, err
		}
	}
	digest := sha256.Sum256(file.Content)
	return map[string]any{
		"draft_id": args.DraftID, "attachment_id": attachmentID, "attached": true,
		"name": file.Name, "content_type": file.ContentType, "content_length": len(file.Content), "content_sha256": hex.EncodeToString(digest[:]),
	}, nil
}

func validUploadURL(address string) bool {
	u, err := url.Parse(address)
	return err == nil && u.Scheme == "https" && strings.EqualFold(u.Hostname(), "outlook.office.com") &&
		(u.Port() == "" || u.Port() == "443") && u.User == nil && u.Fragment == "" && u.RawQuery != "" &&
		strings.HasPrefix(strings.ToLower(u.Path), "/api/") && strings.Contains(strings.ToLower(u.Path), "/attachmentsessions(")
}

func (g *graphClient) uploadAttachment(ctx context.Context, address string, content []byte) error {
	if !validUploadURL(address) {
		return errors.New("Microsoft Graph attachment upload URL was rejected")
	}
	const chunkBytes = 2 << 20
	for start := 0; start < len(content); start += chunkBytes {
		end := min(start+chunkBytes, len(content))
		request, err := http.NewRequestWithContext(ctx, http.MethodPut, address, bytes.NewReader(content[start:end]))
		if err != nil {
			return errors.New("cannot construct attachment upload request")
		}
		request.Header.Set("Content-Type", "application/octet-stream")
		request.Header.Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end-1, len(content)))
		// The upload URL authorizes this request; never forward the Graph bearer token.
		response, err := g.client.Do(request)
		if err != nil {
			return errors.New("attachment upload failed; check the draft before retrying")
		}
		_, readErr := readBounded(response.Body, maxGraphBytes)
		response.Body.Close()
		expected := http.StatusOK
		if end == len(content) {
			expected = http.StatusCreated
		}
		if readErr != nil || response.StatusCode != expected {
			return errors.New("attachment upload did not complete; check the draft before retrying")
		}
	}
	return nil
}
