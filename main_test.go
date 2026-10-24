package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMarkdownPathsFindsMarkdownRecursively(t *testing.T) {
	root := t.TempDir()
	files := []string{
		"index.md",
		filepath.Join("nested", "guide.MD"),
		filepath.Join("nested", "ignore.txt"),
	}
	for _, name := range files {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("content"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	got, err := loadMarkdownPaths(root)
	if err != nil {
		t.Fatalf("loadMarkdownPaths() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("loadMarkdownPaths() returned %d files, want 2: %#v", len(got), got)
	}
	for _, path := range got {
		if filepath.Ext(path) != ".md" && filepath.Ext(path) != ".MD" {
			t.Errorf("non-Markdown path returned: %s", path)
		}
	}
}
