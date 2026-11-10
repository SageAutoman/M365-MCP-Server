package main

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/xuri/excelize/v2"
)

func zipFixture(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var output bytes.Buffer
	archive := zip.NewWriter(&output)
	for name, content := range files {
		writer, err := archive.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := writer.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}

func pdfFixture() []byte {
	stream := "BT /F1 12 Tf 72 720 Td (Hello PDF) Tj ET"
	objects := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 4 0 R /Resources << /Font << /F1 5 0 R >> >> >>",
		fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(stream), stream),
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
	}
	var output bytes.Buffer
	output.WriteString("%PDF-1.4\n")
	offsets := []int{0}
	for i, object := range objects {
		offsets = append(offsets, output.Len())
		fmt.Fprintf(&output, "%d 0 obj\n%s\nendobj\n", i+1, object)
	}
	xref := output.Len()
	fmt.Fprintf(&output, "xref\n0 %d\n0000000000 65535 f \n", len(objects)+1)
	for _, offset := range offsets[1:] {
		fmt.Fprintf(&output, "%010d 00000 n \n", offset)
	}
	fmt.Fprintf(&output, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objects)+1, xref)
	return output.Bytes()
}

func TestAttachmentFormats(t *testing.T) {
	book := excelize.NewFile()
	defer book.Close()
	book.SetCellValue("Sheet1", "A1", "Hello XLSX")
	book.SetCellValue("Sheet1", "C1", 42)
	xlsx, err := book.WriteToBuffer()
	if err != nil {
		t.Fatal(err)
	}
	pptx := zipFixture(t, map[string]string{
		"ppt/presentation.xml":            `<p:presentation xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><p:sldIdLst><p:sldId r:id="second" id="256"/><p:sldId id="257" r:id="first"/></p:sldIdLst></p:presentation>`,
		"ppt/_rels/presentation.xml.rels": `<Relationships><Relationship Id="first" Target="slides/slide1.xml"/><Relationship Id="second" Target="slides/slide2.xml"/></Relationships>`,
		"ppt/slides/slide1.xml":           `<sld><p><t>Last slide</t></p></sld>`,
		"ppt/slides/slide2.xml":           `<sld><p><t>Hello PPTX</t></p></sld>`,
	})
	cases := []struct {
		name    string
		content []byte
		want    string
	}{
		{"notes.TXT", []byte("\xef\xbb\xbf Hello TXT\n"), " Hello TXT\n"},
		{"notes.md", []byte("# Markdown\n"), "# Markdown\n"},
		{"notes.pdf", pdfFixture(), "Hello PDF"},
		{"notes.docx", zipFixture(t, map[string]string{"word/document.xml": `<document><body><p><t>Hello DOCX</t></p><tbl><tr><tc><p><t>Cell</t></p></tc></tr></tbl></body></document>`}), "Hello DOCX"},
		{"notes.xlsx", xlsx.Bytes(), "Hello XLSX\t\t42"},
		{"notes.pptx", pptx, "[Slide 1]\nHello PPTX"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := extractAttachment(fileAttachment{Name: tc.name, Content: tc.content})
			if err != nil || !strings.Contains(result.Content, tc.want) {
				t.Fatalf("extraction failed: %+v %v", result, err)
			}
			if result.Truncated {
				t.Fatal("unexpected truncation")
			}
		})
	}
}

func TestExtractionLimits(t *testing.T) {
	result, err := extractAttachment(fileAttachment{Name: "notes.txt", Content: []byte(strings.Repeat("字", maxTextChars+1))})
	if err != nil || !result.Truncated || utf8.RuneCountInString(result.Content) != maxTextChars || !strings.HasSuffix(result.Content, truncationMarker) {
		t.Fatalf("incorrect Unicode limit: %v", err)
	}
	for _, file := range []fileAttachment{{Name: "x.exe", Content: []byte("text")}, {Name: "x.pdf", Content: []byte("broken PDF")}, {Name: "x.docx", Content: []byte("broken zip")}, {Name: "x.txt", Content: make([]byte, maxAttachmentBytes+1)}} {
		if _, err := extractAttachment(file); err == nil {
			t.Errorf("accepted invalid attachment %s", file.Name)
		}
	}
	oversize := zipFixture(t, map[string]string{"word/document.xml": "x"})
	central := bytes.Index(oversize, []byte{'P', 'K', 1, 2})
	if central < 0 {
		t.Fatal("missing central directory")
	}
	binary.LittleEndian.PutUint32(oversize[central+24:central+28], 65<<20)
	if _, err := officeArchive(oversize); err == nil {
		t.Fatal("accepted expanded archive bomb")
	}
	if _, err := officeArchive(zipFixture(t, map[string]string{"../secret": "x"})); err == nil {
		t.Fatal("accepted traversal archive")
	}
	if format, err := attachmentFormat(fileAttachment{Name: "no-extension", ContentType: "text/plain; charset=utf-8"}); err != nil || format != "txt" {
		t.Fatal("MIME fallback failed")
	}
}
