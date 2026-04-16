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
	"github.com/forechoandlook/gtask/internal/daemon"
	"github.com/forechoandlook/gtask/internal/service"
	"github.com/forechoandlook/gtask/internal/store"
	"net"
)

func getHostPort(args []string) (string, string, []string) {
	host := os.Getenv("GTASK_HOST")
	port := os.Getenv("GTASK_PORT")
	if host == "" {
		host = "127.0.0.1"
	}
	if port == "" {
		port = "8765"
	}
	// simply extract --host and --port
	var newArgs []string
	for i := 0; i < len(args); i++ {
		if args[i] == "--host" && i+1 < len(args) {
			host = args[i+1]
			i++
		} else if args[i] == "--port" && i+1 < len(args) {
			port = args[i+1]
			i++
		} else {
			newArgs = append(newArgs, args[i])
		}
	}
	return host, port, newArgs
}

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

	host, port, cleanArgs := getHostPort(args)
	args = cleanArgs

	if len(args) == 0 {
		printUsage(stdout)
		return nil
	}

	localSvc := &service.LocalService{Store: st, Cfg: cfg}

	if args[0] == "daemon" {
		d := daemon.NewDaemon(localSvc, host, port)
		return d.Start()
	}

	var svc service.Service = localSvc
	rpcSvc, err := daemon.NewRPCClient("tcp", net.JoinHostPort(host, port))
	if err == nil {
		svc = rpcSvc
	}

	switch args[0] {
	case "add":
		return runAdd(ctx, svc, stdout, args[1:])
	case "list":
		return runList(ctx, svc, stdout, args[1:])
	case "filter":
		return runFilter(ctx, svc, stdout, args[1:])
	case "show":
		return runShow(ctx, svc, stdout, args[1:])
	case "update":
		return runUpdate(ctx, svc, stdout, args[1:])
	case "delete":
		return runDelete(ctx, svc, stdout, args[1:])
	case "sync":
		msg, err := svc.Sync(ctx)
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

func runAdd(ctx context.Context, svc service.Service, stdout io.Writer, args []string) error {
	fs := newFlagSet("add")
	title := fs.String("title", "", "task title")
	priority := fs.Int("priority", 0, "priority")
	source := fs.String("source", "", "task source")
	kind := fs.String("kind", "", "task kind stored in meta, for example text or command")
	parent := fs.Int64("parent", 0, "parent task id stored in meta.parent_id")
	startAt := fs.String("start", "", "start time, supports RFC3339 or 'YYYY-MM-DD HH'")
	targetAt := fs.String("target", "", "target time, supports RFC3339 or 'YYYY-MM-DD HH'")
	startDays := fs.Int("start-days", 0, "set start time to N days from now")
	days := fs.Int("days", 0, "set target time to N days from now")
	meta := fs.String("meta", "{}", "json metadata")
	note := fs.String("note", "", "initial note")
	if err := fs.Parse(args); err != nil {
		return err
	}

	finalTitle := *title
	if strings.TrimSpace(finalTitle) == "" && fs.NArg() > 0 {
		finalTitle = fs.Arg(0)
	}
	finalNote := *note
	if finalNote == "" && fs.NArg() > 1 {
		finalNote = fs.Arg(1)
	}

	if strings.TrimSpace(finalTitle) == "" {
		return fmt.Errorf("title is required")
	}
	if !json.Valid([]byte(*meta)) {
		return fmt.Errorf("meta must be valid json")
	}
	metaJSON, err := mergeMeta(*meta, strings.TrimSpace(*kind), hasFlag(args, "parent"), *parent)
	if err != nil {
		return err
	}
	task, err := svc.AddTask(ctx, store.AddInput{
		Title:    finalTitle,
		Priority: *priority,
		Source:   *source,
		StartAt:  resolveTimeArg(*startAt, *startDays),
		TargetAt: resolveTimeArg(*targetAt, *days),
		MetaJSON: metaJSON,
		Note:     finalNote,
	})
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "added task %d: %s\n", task.ID, task.Title)
	return nil
}

func runList(ctx context.Context, svc service.Service, stdout io.Writer, args []string) error {
	fs := newFlagSet("list")
	all := fs.Bool("all", false, "include completed tasks")
	if err := fs.Parse(args); err != nil {
		return err
	}
	tasks, err := svc.ListTasks(ctx, *all)
	if err != nil {
		return err
	}
	for _, task := range tasks {
		metaSummary := summarizeMeta(task.MetaJSON)
		fmt.Fprintf(stdout, "%d\t[%s]\tp%d\t%s\tsource=%s\tkind=%s\tparent=%s\ttarget=%s\tsynced=%s\n",
			task.ID,
			status(task.Completed),
			task.Priority,
			task.Title,
			emptyDash(task.Source),
			emptyDash(metaSummary.Kind),
			formatParent(metaSummary.ParentID),
			formatMaybe(task.TargetAt),
			formatMaybe(task.LastSyncedAt),
		)
	}
	return nil
}

func runFilter(ctx context.Context, svc service.Service, stdout io.Writer, args []string) error {
	fs := newFlagSet("filter")
	all := fs.Bool("all", false, "include completed tasks unless --completed is set")
	source := fs.String("source", "", "filter by exact source")
	query := fs.String("query", "", "substring match against title/meta/notes")
	completed := fs.String("completed", "", "true or false")
	kind := fs.String("kind", "", "filter by meta.kind")
	parent := fs.Int64("parent", 0, "filter by meta.parent_id")
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
	tasks, err := svc.ListTasksFiltered(ctx, filter)
	if err != nil {
		return err
	}
	for _, task := range tasks {
		metaSummary := summarizeMeta(task.MetaJSON)
		if strings.TrimSpace(*kind) != "" && metaSummary.Kind != strings.TrimSpace(*kind) {
			continue
		}
		if hasFlag(args, "parent") && !matchesParent(metaSummary.ParentID, *parent) {
			continue
		}
		fmt.Fprintf(stdout, "%d\t[%s]\tp%d\t%s\tsource=%s\tkind=%s\tparent=%s\ttarget=%s\tsynced=%s\n",
			task.ID,
			status(task.Completed),
			task.Priority,
			task.Title,
			emptyDash(task.Source),
			emptyDash(metaSummary.Kind),
			formatParent(metaSummary.ParentID),
			formatMaybe(task.TargetAt),
			formatMaybe(task.LastSyncedAt),
		)
	}
	return nil
}

func runShow(ctx context.Context, svc service.Service, stdout io.Writer, args []string) error {
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
	task, err := svc.GetTask(ctx, id)
	if err != nil {
		return err
	}
	metaSummary := summarizeMeta(task.MetaJSON)
	fmt.Fprintf(stdout, "id: %d\n", task.ID)
	fmt.Fprintf(stdout, "title: %s\n", task.Title)
	fmt.Fprintf(stdout, "status: %s\n", status(task.Completed))
	fmt.Fprintf(stdout, "priority: %d\n", task.Priority)
	fmt.Fprintf(stdout, "source: %s\n", emptyDash(task.Source))
	fmt.Fprintf(stdout, "kind: %s\n", emptyDash(metaSummary.Kind))
	fmt.Fprintf(stdout, "parent_id: %s\n", formatParent(metaSummary.ParentID))
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

func runUpdate(ctx context.Context, svc service.Service, stdout io.Writer, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: gtask update <id> [flags]")
	}
	id, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		return fmt.Errorf("parse id: %w", err)
	}
	fs := newFlagSet("update")
	title := fs.String("title", "", "new title")
	priority := fs.Int("priority", 0, "new priority")
	source := fs.String("source", "", "new source")
	kind := fs.String("kind", "", "set meta.kind")
	parent := fs.String("parent", "", "set meta.parent_id, or null to clear")
	startAt := fs.String("start", "", "set start time, supports RFC3339, 'YYYY-MM-DD HH', or null")
	targetAt := fs.String("target", "", "set target time, supports RFC3339, 'YYYY-MM-DD HH', or null")
	startDays := fs.Int("start-days", 0, "set start time to N days from now")
	days := fs.Int("days", 0, "set target time to N days from now")
	meta := fs.String("meta", "", "replace metadata json")
	completed := fs.String("completed", "", "true or false")
	note := fs.String("note", "", "append note")
	if err := fs.Parse(args[1:]); err != nil {
		return err
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
	if strings.TrimSpace(*meta) != "" || hasFlag(args, "kind") || hasFlag(args, "parent") {
		baseMeta := "{}"
		current, err := svc.GetTask(ctx, id)
		if err != nil {
			return err
		}
		baseMeta = current.MetaJSON
		if strings.TrimSpace(*meta) != "" {
			baseMeta = *meta
		}
		parentSet, parentValue, err := parseParentUpdateArg(*parent, hasFlag(args, "parent"))
		if err != nil {
			return err
		}
		metaJSON, err := mergeMeta(baseMeta, strings.TrimSpace(*kind), parentSet, parentValue)
		if err != nil {
			return err
		}
		in.MetaJSON = &metaJSON
	}
	if strings.TrimSpace(*meta) != "" {
		if !json.Valid([]byte(*meta)) {
			return fmt.Errorf("meta must be valid json")
		}
	}
	if strings.TrimSpace(*completed) != "" {
		v, err := strconv.ParseBool(*completed)
		if err != nil {
			return fmt.Errorf("parse completed: %w", err)
		}
		in.Completed = &v
	}
	in.AppendNote = *note
	task, err := svc.UpdateTask(ctx, in)
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "updated task %d: %s\n", task.ID, task.Title)
	return nil
}

func runDelete(ctx context.Context, svc service.Service, stdout io.Writer, args []string) error {
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
	if err := svc.DeleteTask(ctx, id); err != nil {
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
	fmt.Fprintln(w, "  gtask add [title] [note] [--title <title>] [--priority N] [--source X] [--kind K] [--parent ID] [--start TIME] [--start-days N] [--target TIME|--days N] [--meta JSON] [--note TEXT]")
	fmt.Fprintln(w, "  gtask list [--all]")
	fmt.Fprintln(w, "  gtask filter [--all] [--source X] [--kind K] [--parent ID] [--query TEXT] [--completed true|false] [--priority-min N] [--priority-max N]")
	fmt.Fprintln(w, "  gtask show <id>")
	fmt.Fprintln(w, "  gtask update <id> [--title T] [--priority N] [--source X] [--kind K] [--parent ID|null] [--start TIME|null] [--start-days N] [--target TIME|null] [--days N] [--meta JSON] [--completed true|false] [--note TEXT]")
	fmt.Fprintln(w, "  gtask delete <id>")
	fmt.Fprintln(w, "  gtask sync")
	fmt.Fprintln(w, "  gtask daemon [--host 127.0.0.1] [--port 8765]")
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
	fmt.Fprintln(w, "  --kind and --parent are stored in meta as meta.kind and meta.parent_id.")
	fmt.Fprintln(w, "  update --start null or --target null clears that field.")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Examples:")
	fmt.Fprintln(w, `  gtask add --title "write docs" --priority 2 --source aistudio --kind text --days 3 --note "first note"`)
	fmt.Fprintln(w, "  gtask add \"write docs\" \"first note\" --priority 2 --source aistudio --kind text --days 3")
	fmt.Fprintln(w, `  gtask add --title "run sync" --kind command --parent 4 --meta '{"cmd":"opencli sync","cwd":"/Users/zzwy/tmp/opencli-rs"}'`)
	fmt.Fprintln(w, `  gtask add --title "night run" --target "2026-04-20 21"`)
	fmt.Fprintln(w, `  gtask filter --source idea1 --kind command`)
	fmt.Fprintln(w, `  gtask show 4`)
	fmt.Fprintln(w, `  gtask update 1 --kind command --parent 4 --completed true --note "done locally"`)
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

type metaSummary struct {
	Kind     string
	ParentID *int64
}

func mergeMeta(raw, kind string, parentSet bool, parent int64) (string, error) {
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
	if strings.TrimSpace(kind) != "" {
		meta["kind"] = strings.TrimSpace(kind)
	}
	if parentSet {
		if parent > 0 {
			meta["parent_id"] = parent
		} else {
			delete(meta, "parent_id")
		}
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

func parseParentUpdateArg(raw string, parentFlagSet bool) (bool, int64, error) {
	if !parentFlagSet {
		return false, 0, nil
	}
	v := strings.TrimSpace(raw)
	if v == "" || strings.EqualFold(v, "null") {
		return true, 0, nil
	}
	id, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return false, 0, fmt.Errorf("parse parent: %w", err)
	}
	if id <= 0 {
		return false, 0, fmt.Errorf("parent must be positive")
	}
	return true, id, nil
}

func matchesParent(parentID *int64, want int64) bool {
	if want <= 0 {
		return parentID == nil
	}
	return parentID != nil && *parentID == want
}

func formatParent(v *int64) string {
	if v == nil {
		return "-"
	}
	return strconv.FormatInt(*v, 10)
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
			fmt.Fprintln(os.Stderr, "usage: gtask add [title] [note] [--title <title>] [--priority N] [--source X] [--kind K] [--parent ID] [--start TIME] [--start-days N] [--target TIME] [--days N] [--meta JSON] [--note TEXT]")
		case "list":
			fmt.Fprintln(os.Stderr, "usage: gtask list [--all]")
		case "filter":
			fmt.Fprintln(os.Stderr, "usage: gtask filter [--all] [--source X] [--kind K] [--parent ID] [--query TEXT] [--completed true|false] [--priority-min N] [--priority-max N]")
		case "show":
			fmt.Fprintln(os.Stderr, "usage: gtask show <id>")
		case "update":
			fmt.Fprintln(os.Stderr, "usage: gtask update <id> [--title T] [--priority N] [--source X] [--kind K] [--parent ID|null] [--start TIME|null] [--start-days N] [--target TIME|null] [--days N] [--meta JSON] [--completed true|false] [--note TEXT]")
		case "delete":
			fmt.Fprintln(os.Stderr, "usage: gtask delete <id>")
		case "daemon":
			fmt.Fprintln(os.Stderr, "usage: gtask daemon [--host 127.0.0.1] [--port 8765]")
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
