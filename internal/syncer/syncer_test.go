package syncer

import (
	"strings"
	"testing"
	"time"

	"github.com/forechoandlook/gtask/internal/model"
)

func TestBuildPayload(t *testing.T) {
	now := time.Date(2026, 4, 15, 10, 0, 0, 0, time.UTC)
	task := model.Task{
		ID:        7,
		Title:     "demo",
		Priority:  2,
		Source:    "cli",
		TargetAt:  &now,
		UpdatedAt: now,
		MetaJSON:  `{"kind":"demo"}`,
		NotesJSON: `[{"at":"2026-04-15T10:00:00Z","text":"n1"}]`,
	}
	payload := buildPayload(task)
	if payload["title"] != "demo" {
		t.Fatalf("unexpected title: %v", payload["title"])
	}
	notes, ok := payload["notes"].(string)
	if !ok || !strings.Contains(notes, `"local_id": 7`) {
		t.Fatalf("unexpected notes payload: %v", payload["notes"])
	}
}
