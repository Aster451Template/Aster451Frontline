package models

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestDocumentJSONRoundTrip(t *testing.T) {
	want := Document{
		ID:        uuid.MustParse("00000000-0000-0000-0000-000000000001"),
		Title:     "Guide",
		Tags:      []string{"docs"},
		CreatedAt: time.Date(2026, 10, 24, 0, 0, 0, 0, time.UTC),
	}
	data, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	var got Document
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if got.ID != want.ID || got.Title != want.Title || len(got.Tags) != 1 || !got.CreatedAt.Equal(want.CreatedAt) {
		t.Fatalf("round-trip document = %#v, want %#v", got, want)
	}
}
