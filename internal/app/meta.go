package app

import (
	"encoding/json"
	"fmt"
	"strings"
)

type metaSummary struct {
	Kind     string
	ParentID *int64
}

type metaUpdates struct {
	kind            string
	parentSet       bool
	parent          int64
	monitorCmd      string
	monitorInterval string
	recurrence      string
}

func mergeMeta(raw string, up metaUpdates) (string, error) {
	var meta map[string]any
	if strings.TrimSpace(raw) == "" {
		raw = "{}"
	}
	if err := json.Unmarshal([]byte(raw), &meta); err != nil {
		return "", fmt.Errorf("meta must be valid json object")
	}
	if meta == nil {
		meta = map[string]any{}
	}
	if up.kind != "" {
		meta["kind"] = up.kind
	}
	if up.parentSet {
		if up.parent > 0 {
			meta["parent_id"] = up.parent
		} else {
			delete(meta, "parent_id")
		}
	}
	if up.monitorCmd != "" {
		meta["monitor_cmd"] = up.monitorCmd
	}
	if up.monitorInterval != "" {
		meta["monitor_interval"] = up.monitorInterval
	}
	if up.recurrence != "" {
		meta["recurrence"] = up.recurrence
	}
	out, err := json.Marshal(meta)
	if err != nil {
		return "", fmt.Errorf("marshal meta: %w", err)
	}
	return string(out), nil
}

func summarizeMeta(raw string) metaSummary {
	var meta map[string]any
	if err := json.Unmarshal([]byte(raw), &meta); err != nil {
		return metaSummary{}
	}
	out := metaSummary{}
	if v, ok := meta["kind"].(string); ok {
		out.Kind = strings.TrimSpace(v)
	}
	if v, ok := toInt64(meta["parent_id"]); ok {
		out.ParentID = &v
	}
	return out
}

func matchesParent(parentID *int64, want int64) bool {
	if want <= 0 {
		return parentID == nil
	}
	return parentID != nil && *parentID == want
}

func toInt64(v any) (int64, bool) {
	switch n := v.(type) {
	case float64:
		return int64(n), true
	case int64:
		return n, true
	case int:
		return int64(n), true
	case json.Number:
		i, err := n.Int64()
		return i, err == nil
	default:
		return 0, false
	}
}
