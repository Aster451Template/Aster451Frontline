package utils

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func TestHashSourcePathNormalizesRelativePath(t *testing.T) {
	got := HashSourcePath("./public\\guide.md")
	wantBytes := sha256.Sum256([]byte("public/guide.md"))
	want := hex.EncodeToString(wantBytes[:])
	if got != want {
		t.Fatalf("HashSourcePath() = %q, want %q", got, want)
	}
	if HashSourcePath("public/guide.md") != got {
		t.Fatal("equivalent path spellings should produce the same hash")
	}
}
