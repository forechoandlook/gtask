package app

import (
	"encoding/json"
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

func TestMergeMeta(t *testing.T) {
	raw, err := mergeMeta(`{"cmd":"opencli sync"}`, metaUpdates{
		kind:      "command",
		parentSet: true,
		parent:    7,
	})
	if err != nil {
		t.Fatal(err)
	}
	var v map[string]any
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		t.Fatal(err)
	}
	if v["kind"] != "command" {
		t.Fatalf("unexpected kind: %#v", v["kind"])
	}
	if parent, ok := toInt64(v["parent_id"]); !ok || parent != 7 {
		t.Fatalf("unexpected parent_id: %#v", v["parent_id"])
	}
}

func TestParseParentUpdateArg(t *testing.T) {
	set, parent, err := parseParentUpdateArg("null", true)
	if err != nil {
		t.Fatal(err)
	}
	if !set || parent != 0 {
		t.Fatalf("unexpected null parse result: set=%v parent=%d", set, parent)
	}
	set, parent, err = parseParentUpdateArg("12", true)
	if err != nil {
		t.Fatal(err)
	}
	if !set || parent != 12 {
		t.Fatalf("unexpected parse result: set=%v parent=%d", set, parent)
	}
}

func TestGetHostPort(t *testing.T) {
	// mock environment variables
	t.Setenv("GTASK_HOST", "myhost")
	t.Setenv("GTASK_PORT", "1234")

	host, port, args := getHostPort([]string{"add", "--title", "test"})
	if host != "myhost" {
		t.Errorf("expected host 'myhost', got %v", host)
	}
	if port != "1234" {
		t.Errorf("expected port '1234', got %v", port)
	}
	if len(args) != 3 {
		t.Errorf("expected 3 args, got %v", args)
	}

	host, port, args = getHostPort([]string{"add", "--host", "override", "--port", "5678", "--title", "test"})
	if host != "override" {
		t.Errorf("expected host 'override', got %v", host)
	}
	if port != "5678" {
		t.Errorf("expected port '5678', got %v", port)
	}
	if len(args) != 3 {
		t.Errorf("expected 3 args, got %v", args)
	}
}
