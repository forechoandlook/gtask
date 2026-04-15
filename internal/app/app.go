package app

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/forechoandlook/gtask/internal/config"
	"github.com/forechoandlook/gtask/internal/store"
	"github.com/forechoandlook/gtask/internal/syncer"
)

func Run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		printUsage(stdout)
		return nil
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	st, err := store.Open(cfg.DBPath)
	if err != nil {
		return err
	}
	defer st.Close()

	switch args[0] {
	case "add":
		return runAdd(ctx, st, stdout, args[1:])
	case "list":
		return runList(ctx, st, stdout, args[1:])
	case "filter":
		return runFilter(ctx, st, stdout, args[1:])
	case "show":
		return runShow(ctx, st, stdout, args[1:])
	case "update":
		return runUpdate(ctx, st, stdout, args[1:])
	case "delete":
		return runDelete(ctx, st, stdout, args[1:])
	case "sync":
		msg, err := syncer.New(cfg, st).Sync(ctx)
		if err != nil {
			return err
		}
		fmt.Fprintln(stdout, msg)
		return nil
	case "help", "--help", "-h":
		printUsage(stdout)
		return nil
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func runAdd(ctx context.Context, st *store.Store, stdout io.Writer, args []string) error {
	fs := newFlagSet("add")
	title := fs.String("title", "", "task title")
	priority := fs.Int("priority", 0, "priority")
	source := fs.String("source", "", "task source")
	startAt := fs.String("start", "", "start time, supports RFC3339 or 'YYYY-MM-DD HH'")
	targetAt := fs.String("target", "", "target time, supports RFC3339 or 'YYYY-MM-DD HH'")
	startDays := fs.Int("start-days", 0, "set start time to N days from now")
	days := fs.Int("days", 0, "set target time to N days from now")
	meta := fs.String("meta", "{}", "json metadata")
	note := fs.String("note", "", "initial note")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*title) == "" {
		return fmt.Errorf("title is required")
	}
	if !json.Valid([]byte(*meta)) {
		return fmt.Errorf("meta must be valid json")
	}
	task, err := st.AddTask(ctx, store.AddInput{
		Title:    *title,
		Priority: *priority,
		Source:   *source,
		StartAt:  resolveTimeArg(*startAt, *startDays),
		TargetAt: resolveTimeArg(*targetAt, *days),
		MetaJSON: *meta,
		Note:     *note,
	})
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "added task %d: %s\n", task.ID, task.Title)
	return nil
}

func runList(ctx context.Context, st *store.Store, stdout io.Writer, args []string) error {
	fs := newFlagSet("list")
	all := fs.Bool("all", false, "include completed tasks")
	if err := fs.Parse(args); err != nil {
		return err
	}
	tasks, err := st.ListTasks(ctx, *all)
	if err != nil {
		return err
	}
	for _, task := range tasks {
		fmt.Fprintf(stdout, "%d\t[%s]\tp%d\t%s\tsource=%s\ttarget=%s\tsynced=%s\n",
			task.ID,
			status(task.Completed),
			task.Priority,
			task.Title,
			emptyDash(task.Source),
			formatMaybe(task.TargetAt),
			formatMaybe(task.LastSyncedAt),
		)
	}
	return nil
}

func runFilter(ctx context.Context, st *store.Store, stdout io.Writer, args []string) error {
	fs := newFlagSet("filter")
	all := fs.Bool("all", false, "include completed tasks unless --completed is set")
	source := fs.String("source", "", "filter by exact source")
	query := fs.String("query", "", "substring match against title/meta/notes")
	completed := fs.String("completed", "", "true or false")
	pmin := fs.Int("priority-min", 0, "minimum priority")
	pmax := fs.Int("priority-max", 0, "maximum priority")
	if err := fs.Parse(args); err != nil {
		return err
	}
	filter := store.ListFilter{
		IncludeCompleted: *all,
		Source:           *source,
		Query:            *query,
	}
	if hasFlag(args, "priority-min") {
		filter.PriorityMin = pmin
	}
	if hasFlag(args, "priority-max") {
		filter.PriorityMax = pmax
	}
	if strings.TrimSpace(*completed) != "" {
		v, err := strconv.ParseBool(*completed)
		if err != nil {
			return fmt.Errorf("parse completed: %w", err)
		}
		filter.Completed = &v
	}
	tasks, err := st.ListTasksFiltered(ctx, filter)
	if err != nil {
		return err
	}
	for _, task := range tasks {
		fmt.Fprintf(stdout, "%d\t[%s]\tp%d\t%s\tsource=%s\ttarget=%s\tsynced=%s\n",
			task.ID,
			status(task.Completed),
			task.Priority,
			task.Title,
			emptyDash(task.Source),
			formatMaybe(task.TargetAt),
			formatMaybe(task.LastSyncedAt),
		)
	}
	return nil
}

func runShow(ctx context.Context, st *store.Store, stdout io.Writer, args []string) error {
	fs := newFlagSet("show")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("usage: gtask show <id>")
	}
	id, err := strconv.ParseInt(fs.Arg(0), 10, 64)
	if err != nil {
		return fmt.Errorf("parse id: %w", err)
	}
	task, err := st.GetTask(ctx, id)
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "id: %d\n", task.ID)
	fmt.Fprintf(stdout, "title: %s\n", task.Title)
	fmt.Fprintf(stdout, "status: %s\n", status(task.Completed))
	fmt.Fprintf(stdout, "priority: %d\n", task.Priority)
	fmt.Fprintf(stdout, "source: %s\n", emptyDash(task.Source))
	fmt.Fprintf(stdout, "start_at: %s\n", formatMaybe(task.StartAt))
	fmt.Fprintf(stdout, "target_at: %s\n", formatMaybe(task.TargetAt))
	fmt.Fprintf(stdout, "updated_at: %s\n", task.UpdatedAt.UTC().Format(time.RFC3339))
	fmt.Fprintf(stdout, "google_task_list_id: %s\n", emptyDash(task.GoogleTaskListID))
	fmt.Fprintf(stdout, "google_task_id: %s\n", emptyDash(task.GoogleTaskID))
	fmt.Fprintf(stdout, "last_synced_at: %s\n", formatMaybe(task.LastSyncedAt))
	fmt.Fprintln(stdout, "meta:")
	fmt.Fprintln(stdout, indentJSON(task.MetaJSON))
	fmt.Fprintln(stdout, "notes:")
	fmt.Fprintln(stdout, indentJSON(task.NotesJSON))
	return nil
}

func runUpdate(ctx context.Context, st *store.Store, stdout io.Writer, args []string) error {
	fs := newFlagSet("update")
	title := fs.String("title", "", "new title")
	priority := fs.Int("priority", 0, "new priority")
	source := fs.String("source", "", "new source")
	startAt := fs.String("start", "", "set start time, supports RFC3339, 'YYYY-MM-DD HH', or null")
	targetAt := fs.String("target", "", "set target time, supports RFC3339, 'YYYY-MM-DD HH', or null")
	startDays := fs.Int("start-days", 0, "set start time to N days from now")
	days := fs.Int("days", 0, "set target time to N days from now")
	meta := fs.String("meta", "", "replace metadata json")
	completed := fs.String("completed", "", "true or false")
	note := fs.String("note", "", "append note")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("usage: gtask update <id> [flags]")
	}
	id, err := strconv.ParseInt(fs.Arg(0), 10, 64)
	if err != nil {
		return fmt.Errorf("parse id: %w", err)
	}
	var in store.UpdateInput
	in.ID = id
	if fs.Lookup("title").Value.String() != "" {
		in.Title = title
	}
	if changedIntFlag(args, "priority") {
		in.Priority = priority
	}
	if fs.Lookup("source").Value.String() != "" {
		in.Source = source
	}
	if hasFlag(args, "start") {
		v := parseNullableFlag(*startAt)
		in.StartAt = &v
	}
	if hasFlag(args, "start-days") {
		v := futureDays(*startDays)
		in.StartAt = &v
	}
	if hasFlag(args, "target") {
		v := parseNullableFlag(*targetAt)
		in.TargetAt = &v
	}
	if hasFlag(args, "days") {
		v := futureDays(*days)
		in.TargetAt = &v
	}
	if strings.TrimSpace(*meta) != "" {
		if !json.Valid([]byte(*meta)) {
			return fmt.Errorf("meta must be valid json")
		}
		in.MetaJSON = meta
	}
	if strings.TrimSpace(*completed) != "" {
		v, err := strconv.ParseBool(*completed)
		if err != nil {
			return fmt.Errorf("parse completed: %w", err)
		}
		in.Completed = &v
	}
	in.AppendNote = *note
	task, err := st.UpdateTask(ctx, in)
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "updated task %d: %s\n", task.ID, task.Title)
	return nil
}

func runDelete(ctx context.Context, st *store.Store, stdout io.Writer, args []string) error {
	fs := newFlagSet("delete")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("usage: gtask delete <id>")
	}
	id, err := strconv.ParseInt(fs.Arg(0), 10, 64)
	if err != nil {
		return fmt.Errorf("parse id: %w", err)
	}
	if err := st.DeleteTask(ctx, id); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "deleted task %d\n", id)
	return nil
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, "gtask: local SQLite-backed task CLI with Google Tasks sync")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Data files:")
	fmt.Fprintln(w, "  ~/.gtask/gtask.db")
	fmt.Fprintln(w, "  ~/.gtask/config.json")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Commands:")
	fmt.Fprintln(w, "  gtask --version")
	fmt.Fprintln(w, "  gtask add --title <title> [--priority N] [--source X] [--start TIME] [--start-days N] [--target TIME|--days N] [--meta JSON] [--note TEXT]")
	fmt.Fprintln(w, "  gtask list [--all]")
	fmt.Fprintln(w, "  gtask filter [--all] [--source X] [--query TEXT] [--completed true|false] [--priority-min N] [--priority-max N]")
	fmt.Fprintln(w, "  gtask show <id>")
	fmt.Fprintln(w, "  gtask update <id> [--title T] [--priority N] [--source X] [--start TIME|null] [--start-days N] [--target TIME|null] [--days N] [--meta JSON] [--completed true|false] [--note TEXT]")
	fmt.Fprintln(w, "  gtask delete <id>")
	fmt.Fprintln(w, "  gtask sync")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Time formats:")
	fmt.Fprintln(w, "  RFC3339 example: 2026-04-15T23:00:00+08:00")
	fmt.Fprintln(w, "  Short form:      2026-04-15 23")
	fmt.Fprintln(w, "  Relative days:   --days 3")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Notes:")
	fmt.Fprintln(w, "  RFC3339 is an internet timestamp format like 2026-04-15T23:00:00+08:00.")
	fmt.Fprintln(w, "  --days N means target time = now + N days.")
	fmt.Fprintln(w, "  --start-days N means start time = now + N days.")
	fmt.Fprintln(w, "  update --start null or --target null clears that field.")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Examples:")
	fmt.Fprintln(w, `  gtask add --title "write docs" --priority 2 --source aistudio --days 3 --meta '{"kind":"draft"}' --note "first note"`)
	fmt.Fprintln(w, `  gtask add --title "night run" --target "2026-04-20 21"`)
	fmt.Fprintln(w, `  gtask filter --source idea1 --query CDP`)
	fmt.Fprintln(w, `  gtask show 4`)
	fmt.Fprintln(w, `  gtask update 1 --completed true --note "done locally"`)
	fmt.Fprintln(w, `  gtask delete 3`)
	fmt.Fprintln(w, `  gtask sync`)
}

func parseTime(v string) *time.Time {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	for _, layout := range []string{
		time.RFC3339,
		"2006-01-02 15",
		"2006-01-02 15:04",
		"2006-01-02",
	} {
		t, err := time.ParseInLocation(layout, strings.TrimSpace(v), time.Local)
		if err == nil {
			return &t
		}
	}
	return nil
}

func parseNullableFlag(v string) *time.Time {
	if strings.TrimSpace(v) == "" || strings.EqualFold(strings.TrimSpace(v), "null") {
		return nil
	}
	return parseTime(v)
}

func resolveTimeArg(raw string, days int) *time.Time {
	if strings.TrimSpace(raw) != "" {
		return parseTime(raw)
	}
	if days != 0 {
		return futureDays(days)
	}
	return nil
}

func futureDays(days int) *time.Time {
	t := time.Now().Add(time.Duration(days) * 24 * time.Hour).UTC()
	return &t
}

func status(v bool) string {
	if v {
		return "done"
	}
	return "todo"
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

func hasFlag(args []string, name string) bool {
	for _, arg := range args {
		if arg == "--"+name || strings.HasPrefix(arg, "--"+name+"=") {
			return true
		}
	}
	return false
}

func changedIntFlag(args []string, name string) bool {
	return hasFlag(args, name)
}

func newFlagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.Usage = func() {
		switch name {
		case "add":
			fmt.Fprintln(os.Stderr, "usage: gtask add --title <title> [--priority N] [--source X] [--start TIME] [--start-days N] [--target TIME] [--days N] [--meta JSON] [--note TEXT]")
		case "list":
			fmt.Fprintln(os.Stderr, "usage: gtask list [--all]")
		case "filter":
			fmt.Fprintln(os.Stderr, "usage: gtask filter [--all] [--source X] [--query TEXT] [--completed true|false] [--priority-min N] [--priority-max N]")
		case "show":
			fmt.Fprintln(os.Stderr, "usage: gtask show <id>")
		case "update":
			fmt.Fprintln(os.Stderr, "usage: gtask update <id> [--title T] [--priority N] [--source X] [--start TIME|null] [--start-days N] [--target TIME|null] [--days N] [--meta JSON] [--completed true|false] [--note TEXT]")
		case "delete":
			fmt.Fprintln(os.Stderr, "usage: gtask delete <id>")
		default:
			fmt.Fprintf(os.Stderr, "usage: gtask %s\n", name)
		}
	}
	return fs
}

func indentJSON(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return "{}"
	}
	var v any
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return raw
	}
	out, err := json.MarshalIndent(v, "  ", "  ")
	if err != nil {
		return raw
	}
	return string(out)
}
