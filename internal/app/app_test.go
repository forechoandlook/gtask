package app

import (
	"encoding/json"
	"strings"
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

func TestExtractPositionalArgs(t *testing.T) {
	cases := []struct {
		args []string
		want []string
	}{
		{[]string{"1", "2", "3"}, []string{"1", "2", "3"}},
		{[]string{"1", "2", "--note", "foo"}, []string{"1", "2"}},
		{[]string{"--note", "foo", "1", "2"}, []string{"1", "2"}},
		{[]string{"1", "--note", "foo", "2", "--target", "2026-04-20"}, []string{"1", "2"}},
		{[]string{"1", "--note=foo", "2"}, []string{"1", "2"}},
		{[]string{"--note", "foo"}, nil},
		{[]string{}, nil},
	}
	for _, tc := range cases {
		got := extractPositionalArgs(tc.args)
		if len(got) != len(tc.want) {
			t.Errorf("extractPositionalArgs(%v) = %v, want %v", tc.args, got, tc.want)
			continue
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("extractPositionalArgs(%v)[%d] = %q, want %q", tc.args, i, got[i], tc.want[i])
			}
		}
	}
}

func TestParseIDs(t *testing.T) {
	cases := []struct {
		args    []string
		want    []int64
		wantErr bool
	}{
		{[]string{"1"}, []int64{1}, false},
		{[]string{"1", "2", "3"}, []int64{1, 2, 3}, false},
		{[]string{"1,2,3"}, []int64{1, 2, 3}, false},
		{[]string{"1", "2,3", "4"}, []int64{1, 2, 3, 4}, false},
		{[]string{"1,,2"}, []int64{1, 2}, false},
		{[]string{"abc"}, nil, true},
		{[]string{"1,abc,3"}, nil, true},
	}
	for _, tc := range cases {
		got, err := parseIDs(tc.args)
		if tc.wantErr {
			if err == nil {
				t.Errorf("parseIDs(%v): expected error", tc.args)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseIDs(%v): unexpected error: %v", tc.args, err)
			continue
		}
		if len(got) != len(tc.want) {
			t.Errorf("parseIDs(%v) = %v, want %v", tc.args, got, tc.want)
			continue
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("parseIDs(%v)[%d] = %d, want %d", tc.args, i, got[i], tc.want[i])
			}
		}
	}
}

func TestGetLatestNote(t *testing.T) {
	cases := []struct {
		name     string
		raw      string
		contains []string
		empty    bool
	}{
		{
			name:     "single note",
			raw:      `[{"at":"2026-04-18T15:30:00Z","text":"Test note"}]`,
			contains: []string{"2026-04-18", "Test note"},
		},
		{
			name:     "returns latest of multiple",
			raw:      `[{"at":"2026-04-18T10:00:00Z","text":"First"},{"at":"2026-04-18T15:30:00Z","text":"Latest"}]`,
			contains: []string{"Latest"},
		},
		{
			name:     "newlines replaced with spaces",
			raw:      `[{"at":"2026-04-18T15:30:00Z","text":"Line 1\nLine 2"}]`,
			contains: []string{"Line 1 Line 2"},
		},
		{name: "empty array", raw: `[]`, empty: true},
		{name: "invalid json", raw: `{bad}`, empty: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := getLatestNote(tc.raw)
			if tc.empty {
				if got != "" {
					t.Errorf("expected empty, got %q", got)
				}
				return
			}
			for _, sub := range tc.contains {
				if !strings.Contains(got, sub) {
					t.Errorf("getLatestNote: %q does not contain %q", got, sub)
				}
			}
		})
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
