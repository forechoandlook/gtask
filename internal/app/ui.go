package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/forechoandlook/gtask/internal/model"
)

func printTasks(w io.Writer, tasks []model.Task, label string) {
	if len(tasks) > 0 {
		fmt.Fprintln(w, "id,title,priority,target_time,kind,src,parent,audit,note")
	}
	for _, t := range tasks {
		fmt.Fprintln(w, formatTaskLine(t))
	}
}

func formatTaskLine(t model.Task) string {
	meta := summarizeMeta(t.MetaJSON)

	priorityStr := fmt.Sprintf("p%d", t.Priority)
	targetStr := "-"
	if t.TargetAt != nil {
		targetStr = t.TargetAt.Local().Format("2006-01-02 15:04")
	}

	return fmt.Sprintf("%d,%q,%s,%s,%s,%s,%s,%s,%q",
		t.ID,
		t.Title,
		priorityStr,
		targetStr,
		emptyDash(meta.Kind),
		emptyDash(t.Source),
		formatParent(meta.ParentID),
		auditTask(t),
		truncate(getLatestNote(t.NotesJSON), 50),
	)
}

func auditTask(t model.Task) string {
	meta := summarizeMeta(t.MetaJSON)
	nc := countNotes(t.NotesJSON)
	var missing []string
	if t.Source == "" {
		missing = append(missing, "S")
	}
	if meta.Kind == "" {
		missing = append(missing, "K")
	}
	if nc == 0 {
		missing = append(missing, "N")
	}
	if len(missing) > 0 {
		return "MISSING_" + strings.Join(missing, "")
	}
	return "-"
}

func getLatestNote(raw string) string {
	var notes []struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal([]byte(raw), &notes); err != nil {
		return ""
	}
	if len(notes) == 0 {
		return ""
	}
	return notes[len(notes)-1].Text
}

func truncate(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func countNotes(raw string) int {
	var notes []any
	if err := json.Unmarshal([]byte(raw), &notes); err != nil {
		return 0
	}
	return len(notes)
}

func status(v bool) string {
	if v {
		return "done"
	}
	return "todo"
}

func formatParent(v *int64) string {
	if v == nil {
		return "-"
	}
	return fmt.Sprintf("%d", *v)
}

func formatMaybe(v *time.Time) string {
	if v == nil {
		return "-"
	}
	return v.UTC().Format(time.RFC3339)
}

func emptyDash(v string) string {
	if strings.TrimSpace(v) == "" {
		return "-"
	}
	return v
}

func indentJSON(raw string) string {
	var out bytes.Buffer
	if err := json.Indent(&out, []byte(raw), "  ", "  "); err != nil {
		return raw
	}
	return "  " + out.String()
}
