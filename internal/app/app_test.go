package app

import (
	"testing"
	"time"
)

func TestParseTimeFormats(t *testing.T) {
	cases := []string{
		"2026-04-15T23:00:00+08:00",
		"2026-04-15 23",
		"2026-04-15 23:30",
		"2026-04-15",
	}
	for _, input := range cases {
		if got := parseTime(input); got == nil {
			t.Fatalf("expected parse success for %q", input)
		}
	}
}

func TestResolveTimeArgDays(t *testing.T) {
	got := resolveTimeArg("", 3)
	if got == nil {
		t.Fatal("expected non-nil time")
	}
	if got.Before(time.Now().Add(71 * time.Hour)) {
		t.Fatal("expected about 3 days in the future")
	}
}
