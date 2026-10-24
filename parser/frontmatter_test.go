package parser

import (
	"errors"
	"strings"
	"testing"
)

func TestParseFrontMatterParsesDocumentAndBody(t *testing.T) {
	input := []byte("=====\ntitle: Guide\npath: /guide\nauthor: Test\ntags: [docs]\n=====\n\n# Body\n")
	doc, body, err := ParseFrontMatter(input)
	if err != nil {
		t.Fatalf("ParseFrontMatter() error = %v", err)
	}
	if doc.Title != "Guide" || doc.Path != "/guide" || doc.Author != "Test" {
		t.Errorf("parsed document = %#v", doc)
	}
	if len(doc.Tags) != 1 || doc.Tags[0] != "docs" {
		t.Errorf("parsed tags = %#v", doc.Tags)
	}
	if string(body) != "# Body" {
		t.Errorf("body = %q, want %q", body, "# Body")
	}
	if doc.ID == [16]byte{} || doc.CreatedAt.IsZero() || doc.UpdatedAt.IsZero() {
		t.Errorf("ID and timestamps should be populated: %#v", doc)
	}
}

func TestParseFrontMatterReportsMissingDelimiter(t *testing.T) {
	_, _, err := ParseFrontMatter([]byte("=====\ntitle: Missing close\n"))
	if !errors.Is(err, ErrMissingDelimiter) {
		t.Fatalf("ParseFrontMatter() error = %v, want ErrMissingDelimiter", err)
	}
}

func TestParseFrontMatterRequiresTitle(t *testing.T) {
	_, _, err := ParseFrontMatter([]byte("=====\npath: /guide\nauthor: Test\n=====\nbody"))
	if err == nil || !strings.Contains(err.Error(), "title is required") {
		t.Fatalf("ParseFrontMatter() error = %v, want required-title error", err)
	}
}
