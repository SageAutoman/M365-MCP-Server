package main

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/ledongthuc/pdf"
	"github.com/xuri/excelize/v2"
)

const maxTextChars = 100_000
const truncationMarker = "\n\n[Content truncated by server limit.]"

type fileAttachment struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	ContentType string `json:"content_type"`
	Content     []byte `json:"content"`
}

func (f fileAttachment) metadata() map[string]any {
	return map[string]any{"id": f.ID, "name": f.Name, "content_type": f.ContentType, "content_length": len(f.Content)}
}

type extractionResult struct {
	Format    string `json:"format"`
	Content   string `json:"content"`
	Truncated bool   `json:"truncated"`
}

type textSink struct {
	text      []rune
	truncated bool
}

func (s *textSink) add(text string) {
	for _, char := range text {
		if len(s.text) == maxTextChars {
			s.truncated = true
			return
		}
		s.text = append(s.text, char)
	}
}

func (s *textSink) result(format string) extractionResult {
	if s.truncated {
		marker := []rune(truncationMarker)
		s.text = append(s.text[:maxTextChars-len(marker)], marker...)
	}
	text := string(s.text)
	if format != "txt" && format != "markdown" {
		text = strings.TrimSpace(text)
	}
	return extractionResult{format, text, s.truncated}
}

func attachmentFormat(f fileAttachment) (string, error) {
	name := strings.ReplaceAll(f.Name, `\`, "/")
	ext := strings.ToLower(path.Ext(name))
	formats := map[string]string{".txt": "txt", ".md": "markdown", ".markdown": "markdown", ".pdf": "pdf", ".docx": "docx", ".xlsx": "xlsx", ".pptx": "pptx"}
	if format := formats[ext]; format != "" {
		return format, nil
	}
	media := strings.ToLower(strings.TrimSpace(strings.SplitN(f.ContentType, ";", 2)[0]))
	mediaTypes := map[string]string{
		"text/plain": "txt", "text/markdown": "markdown", "text/x-markdown": "markdown", "application/pdf": "pdf",
		"application/vnd.openxmlformats-officedocument.wordprocessingml.document":   "docx",
		"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":         "xlsx",
		"application/vnd.openxmlformats-officedocument.presentationml.presentation": "pptx",
	}
	if format := mediaTypes[media]; format != "" {
		return format, nil
	}
	return "", inputError{"attachment_id", "unsupported_format", "supported formats are PDF, DOCX, XLSX, PPTX, TXT, and Markdown"}
}

func officeArchive(content []byte) (map[string]*zip.File, error) {
	archive, err := zip.NewReader(bytes.NewReader(content), int64(len(content)))
	if err != nil {
		return nil, errors.New("invalid Office archive")
	}
	if len(archive.File) > 2048 {
		return nil, errors.New("Office archive contains too many entries")
	}
	files := make(map[string]*zip.File, len(archive.File))
	var total uint64
	for _, file := range archive.File {
		if file.UncompressedSize64 > 64<<20 || total > (64<<20)-file.UncompressedSize64 {
			return nil, errors.New("Office archive exceeds the expanded byte limit")
		}
		total += file.UncompressedSize64
		if file.Flags&1 != 0 || strings.Contains(file.Name, `\`) || strings.HasPrefix(file.Name, "/") || path.Clean(file.Name) == ".." || strings.HasPrefix(path.Clean(file.Name), "../") {
			return nil, errors.New("encrypted or unsafe Office archives are not supported")
		}
		if _, exists := files[file.Name]; exists {
			return nil, errors.New("duplicate Office archive entry")
		}
		files[file.Name] = file
	}
	return files, nil
}

func officePart(files map[string]*zip.File, name string) ([]byte, error) {
	file := files[name]
	if file == nil {
		return nil, errors.New("Office document is missing a required part")
	}
	r, err := file.Open()
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return readBounded(r, 64<<20)
}

func xmlText(content []byte, sink *textSink) error {
	decoder := xml.NewDecoder(bytes.NewReader(content))
	for !sink.truncated {
		token, err := decoder.Token()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return errors.New("invalid Office XML")
		}
		switch element := token.(type) {
		case xml.StartElement:
			switch element.Name.Local {
			case "t":
				var text string
				if err := decoder.DecodeElement(&text, &element); err != nil {
					return err
				}
				sink.add(text)
			case "tab":
				sink.add("\t")
			case "br":
				sink.add("\n")
			}
		case xml.EndElement:
			switch element.Name.Local {
			case "p", "tr":
				sink.add("\n")
			case "tc":
				sink.add("\t")
			}
		}
	}
	return nil
}

func extractAttachment(file fileAttachment) (result extractionResult, err error) {
	defer func() {
		if recover() != nil {
			result, err = extractionResult{}, errors.New("attachment parser rejected the document")
		}
	}()
	if len(file.Content) > maxAttachmentBytes {
		return result, errors.New("attachment exceeds the byte limit")
	}
	format, err := attachmentFormat(file)
	if err != nil {
		return result, err
	}
	sink := &textSink{}
	switch format {
	case "txt", "markdown":
		text := bytes.TrimPrefix(file.Content, []byte{0xef, 0xbb, 0xbf})
		sink.add(strings.ToValidUTF8(string(text), "\ufffd"))
	case "pdf":
		reader, err := pdf.NewReader(bytes.NewReader(file.Content), int64(len(file.Content)))
		if err != nil {
			return result, err
		}
		pages := reader.NumPage()
		if pages < 0 || pages > 100_000 {
			return result, errors.New("PDF page count exceeds the limit")
		}
		for n := 1; n <= pages && !sink.truncated; n++ {
			text, err := reader.Page(n).GetPlainText(nil)
			if err != nil {
				return result, err
			}
			if strings.TrimSpace(text) != "" {
				sink.add(fmt.Sprintf("[Page %d]\n%s\n\n", n, strings.TrimSpace(text)))
			}
		}
	case "docx", "xlsx", "pptx":
		files, err := officeArchive(file.Content)
		if err != nil {
			return result, err
		}
		switch format {
		case "docx":
			content, err := officePart(files, "word/document.xml")
			if err != nil {
				return result, err
			}
			if err = xmlText(content, sink); err != nil {
				return result, err
			}
		case "xlsx":
			book, err := excelize.OpenReader(bytes.NewReader(file.Content), excelize.Options{UnzipSizeLimit: 64 << 20, UnzipXMLSizeLimit: 64 << 20})
			if err != nil {
				return result, err
			}
			defer book.Close()
			for _, name := range book.GetSheetList() {
				sink.add("[Sheet " + name + "]\n")
				rows, err := book.Rows(name)
				if err != nil {
					return result, err
				}
				for rows.Next() && !sink.truncated {
					cells, err := rows.Columns()
					if err != nil {
						rows.Close()
						return result, err
					}
					line := strings.TrimRight(strings.Join(cells, "\t"), "\t")
					if strings.TrimSpace(line) != "" {
						sink.add(line + "\n")
					}
				}
				rows.Close()
				if rows.Error() != nil {
					return result, rows.Error()
				}
				if sink.truncated {
					break
				}
			}
		case "pptx":
			if err := extractPresentation(files, sink); err != nil {
				return result, err
			}
		}
	}
	return sink.result(format), nil
}

func extractPresentation(files map[string]*zip.File, sink *textSink) error {
	content, err := officePart(files, "ppt/presentation.xml")
	if err != nil {
		return err
	}
	var presentation struct {
		Slides []struct {
			ID string `xml:"http://schemas.openxmlformats.org/officeDocument/2006/relationships id,attr"`
		} `xml:"sldIdLst>sldId"`
	}
	if err := xml.Unmarshal(content, &presentation); err != nil {
		return err
	}
	content, err = officePart(files, "ppt/_rels/presentation.xml.rels")
	if err != nil {
		return err
	}
	var relations struct {
		Items []struct {
			ID     string `xml:"Id,attr"`
			Target string `xml:"Target,attr"`
			Mode   string `xml:"TargetMode,attr"`
		} `xml:"Relationship"`
	}
	if err := xml.Unmarshal(content, &relations); err != nil {
		return err
	}
	targets := map[string]string{}
	for _, item := range relations.Items {
		if item.Mode != "External" {
			target := path.Clean(path.Join("ppt", item.Target))
			if strings.HasPrefix(target, "ppt/slides/") {
				targets[item.ID] = target
			}
		}
	}
	for index, slide := range presentation.Slides {
		content, err := officePart(files, targets[slide.ID])
		if err != nil {
			return err
		}
		sink.add(fmt.Sprintf("[Slide %d]\n", index+1))
		if err := xmlText(content, sink); err != nil {
			return err
		}
		sink.add("\n")
		if sink.truncated {
			break
		}
	}
	return nil
}

type boundedBuffer struct{ bytes.Buffer }

func (b *boundedBuffer) Write(data []byte) (int, error) {
	if b.Len()+len(data) > 1<<20 {
		return 0, errors.New("worker output exceeds the byte limit")
	}
	return b.Buffer.Write(data)
}

func isolatedExtraction(ctx context.Context, file fileAttachment) (extractionResult, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	executable, err := os.Executable()
	if err != nil {
		return extractionResult{}, err
	}
	cmd := exec.CommandContext(ctx, executable, "extract-worker")
	// The worker receives no credentials and waits for input until its Job Object is attached.
	cmd.Env = []string{"GOMEMLIMIT=384MiB", "GOMAXPROCS=1", "SYSTEMROOT=" + os.Getenv("SYSTEMROOT")}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return extractionResult{}, err
	}
	var output boundedBuffer
	cmd.Stdout = &output
	if err := cmd.Start(); err != nil {
		stdin.Close()
		return extractionResult{}, errors.New("could not start the attachment worker")
	}
	release, err := attachWorkerLimits(cmd.Process.Pid)
	if err != nil {
		stdin.Close()
		cmd.Process.Kill()
		cmd.Wait()
		return extractionResult{}, errors.New("could not apply Windows attachment resource limits")
	}
	defer release()
	writeErr := json.NewEncoder(stdin).Encode(file)
	stdin.Close()
	waitErr := cmd.Wait()
	if writeErr != nil || waitErr != nil || ctx.Err() != nil {
		return extractionResult{}, errors.New("attachment extraction failed or exceeded resource limits")
	}
	var result extractionResult
	if json.Unmarshal(output.Bytes(), &result) != nil || !utf8.ValidString(result.Content) || utf8.RuneCountInString(result.Content) > maxTextChars {
		return extractionResult{}, errors.New("attachment worker returned an invalid result")
	}
	return result, nil
}

func runExtractionWorker() error {
	data, err := readBounded(os.Stdin, (maxAttachmentBytes*4/3)+(64<<10))
	if err != nil {
		return err
	}
	var file fileAttachment
	if err := json.Unmarshal(data, &file); err != nil || len(file.ID) > 4096 || len(file.Name) > 4096 || len(file.ContentType) > 1024 {
		return errors.New("invalid attachment worker input")
	}
	result, err := extractAttachment(file)
	if err != nil {
		return errors.New("attachment could not be parsed")
	}
	return json.NewEncoder(os.Stdout).Encode(result)
}
