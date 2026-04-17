package app

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/forechoandlook/gtask/internal/config"
	"github.com/forechoandlook/gtask/internal/daemon"
	"github.com/forechoandlook/gtask/internal/model"
	"github.com/forechoandlook/gtask/internal/service"
	"github.com/forechoandlook/gtask/internal/store"
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

	// Handle global --json flag
	jsonMode := false
	var finalArgs []string
	for _, arg := range args {
		if arg == "--json" {
			jsonMode = true
		} else {
			finalArgs = append(finalArgs, arg)
		}
	}
	args = finalArgs

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
	rpcSvc, rpcErr := daemon.NewRPCClient("tcp", net.JoinHostPort(host, port))
	if rpcErr == nil {
		svc = rpcSvc
	}

	var cmdErr error
	switch args[0] {
	case "add":
		cmdErr = runAdd(ctx, svc, stdout, args[1:], jsonMode)
	case "todo", "list":
		cmdErr = runTodo(ctx, svc, stdout, args[1:], jsonMode)
	case "done":
		cmdErr = runDone(ctx, svc, stdout, args[1:], jsonMode)
	case "filter":
		cmdErr = runFilter(ctx, svc, stdout, args[1:], jsonMode)
	case "show":
		cmdErr = runShow(ctx, svc, stdout, args[1:], jsonMode)
	case "update":
		cmdErr = runUpdate(ctx, svc, stdout, args[1:], jsonMode)
	case "delete":
		cmdErr = runDelete(ctx, svc, stdout, args[1:], jsonMode)
	case "upgrade":
		cmdErr = runSelfUpgrade(ctx, stdout)
	case "sync":
		msg, syncErr := svc.Sync(ctx)
		if syncErr != nil {
			return syncErr
		}
		if jsonMode {
			json.NewEncoder(stdout).Encode(map[string]string{"message": msg})
		} else {
			fmt.Fprintln(stdout, msg)
		}
		return nil
	case "help", "--help", "-h":
		printUsage(stdout)
		return nil
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
	if cmdErr != nil {
		if cmdErr == flag.ErrHelp {
			return nil
		}
		return cmdErr
	}
	return nil
}

func runAdd(ctx context.Context, svc service.Service, stdout io.Writer, args []string, jsonMode bool) error {
	fs := newFlagSet("add")
	title := fs.String("title", "", "task title")
	priority := fs.Int("priority", 0, "priority")
	source := fs.String("source", "", "task source")
	kind := fs.String("kind", "", "task kind")
	parent := fs.Int64("parent", 0, "parent task id")
	startAt := fs.String("start", "", "start time")
	targetAt := fs.String("target", "", "target time")
	startDays := fs.Int("start-days", 0, "start days from now")
	days := fs.Int("days", 0, "target days from now")
	meta := fs.String("meta", "{}", "json metadata")
	note := fs.String("note", "", "initial note")
	monitorCmd := fs.String("monitor-cmd", "", "command to run periodically")
	monitorInterval := fs.String("monitor-interval", "10m", "how often to run monitor command")
	recurrence := fs.String("recurrence", "", "recurrence interval (e.g. 24h)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	finalTitle := *title
	if strings.TrimSpace(finalTitle) == "" && fs.NArg() > 0 {
		arg0 := fs.Arg(0)
		if !strings.HasPrefix(arg0, "-") {
			finalTitle = arg0
		}
	}
	finalNote := *note
	if finalNote == "" && fs.NArg() > 1 {
		arg1 := fs.Arg(1)
		if !strings.HasPrefix(arg1, "-") {
			finalNote = arg1
		}
	}

	if strings.TrimSpace(finalTitle) == "" {
		return fmt.Errorf("title is required")
	}
	if !json.Valid([]byte(*meta)) {
		return fmt.Errorf("meta must be valid json")
	}
	metaJSON, err := mergeMeta(*meta, metaUpdates{
		kind:            strings.TrimSpace(*kind),
		parentSet:       hasFlag(args, "parent"),
		parent:          *parent,
		monitorCmd:      *monitorCmd,
		monitorInterval: *monitorInterval,
		recurrence:      *recurrence,
	})
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
	if jsonMode {
		return json.NewEncoder(stdout).Encode(task)
	}
	fmt.Fprintf(stdout, "added task %d: %s\n", task.ID, task.Title)
	return nil
}

func runTodo(ctx context.Context, svc service.Service, stdout io.Writer, args []string, jsonMode bool) error {
	fs := newFlagSet("todo")
	if err := fs.Parse(args); err != nil {
		return err
	}
	tasks, err := svc.ListTasks(ctx, false)
	if err != nil {
		return err
	}
	if jsonMode {
		return json.NewEncoder(stdout).Encode(tasks)
	}

	printTasks(stdout, tasks, "")
	return nil
}

func runDone(ctx context.Context, svc service.Service, stdout io.Writer, args []string, jsonMode bool) error {
	fs := newFlagSet("done")
	if err := fs.Parse(args); err != nil {
		return err
	}
	completed := true
	tasks, err := svc.ListTasksFiltered(ctx, store.ListFilter{Completed: &completed})
	if err != nil {
		return err
	}
	if jsonMode {
		return json.NewEncoder(stdout).Encode(tasks)
	}

	printTasks(stdout, tasks, "")
	return nil
}

func printTasks(w io.Writer, tasks []model.Task, label string) {
	if len(tasks) > 0 {
		fmt.Fprintln(w, "id,title,priority,target_time,kind,src,parent,audit,note")
	}
	for _, t := range tasks {
		meta := summarizeMeta(t.MetaJSON)
		nc := countNotes(t.NotesJSON)
		
		priorityStr := fmt.Sprintf("p%d", t.Priority)
		
		targetStr := "-"
		if t.TargetAt != nil {
			targetStr = t.TargetAt.Local().Format("2006-01-02 15:04")
		}
		
		kind := meta.Kind
		if kind == "" {
			kind = "-"
		}

		source := t.Source
		if source == "" {
			source = "-"
		}

		parentStr := "-"
		if meta.ParentID != nil {
			parentStr = fmt.Sprintf("%d", *meta.ParentID)
		}
		
		auditStr := "-"
		var missing []string
		if t.Source == "" { missing = append(missing, "S") }
		if meta.Kind == "" { missing = append(missing, "K") }
		if nc == 0 { missing = append(missing, "N") }
		if len(missing) > 0 && !t.Completed {
			auditStr = "MISSING_" + strings.Join(missing, "")
		}

		noteStr := "-"
		if nc > 0 {
			noteStr = getLatestNote(t.NotesJSON)
		}

		// CSV Line: id,title,priority,target_time,kind,src,parent,audit,note
		fmt.Fprintf(w, "%d,%q,%s,%s,%s,%s,%s,%s,%q\n",
			t.ID, t.Title, priorityStr, targetStr, kind, source, parentStr, auditStr, noteStr)
	}
}

func getLatestNote(raw string) string {
	var notes []model.Note
	if err := json.Unmarshal([]byte(raw), &notes); err != nil {
		return ""
	}
	if len(notes) == 0 {
		return ""
	}
	// Assuming notes are appended to the end
	return notes[len(notes)-1].Text
}

func truncate(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > n {
		return s[:n-3] + "..."
	}
	return s
}

func runFilter(ctx context.Context, svc service.Service, stdout io.Writer, args []string, jsonMode bool) error {
	fs := newFlagSet("filter")
	all := fs.Bool("all", false, "include completed tasks")
	source := fs.String("source", "", "filter by source")
	query := fs.String("query", "", "keyword search")
	completed := fs.String("completed", "", "true or false")
	kind := fs.String("kind", "", "filter by kind")
	parent := fs.Int64("parent", 0, "filter by parent_id")
	pmin := fs.Int("priority-min", 0, "min priority")
	pmax := fs.Int("priority-max", 0, "max priority")
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
			return err
		}
		filter.Completed = &v
	}
	tasks, err := svc.ListTasksFiltered(ctx, filter)
	if err != nil {
		return err
	}

	var filtered []model.Task
	for _, task := range tasks {
		metaSummary := summarizeMeta(task.MetaJSON)
		if strings.TrimSpace(*kind) != "" && metaSummary.Kind != strings.TrimSpace(*kind) {
			continue
		}
		if hasFlag(args, "parent") && !matchesParent(metaSummary.ParentID, *parent) {
			continue
		}
		filtered = append(filtered, task)
	}

	if jsonMode {
		return json.NewEncoder(stdout).Encode(filtered)
	}

	var pending, completedItems []model.Task
	for _, t := range filtered {
		if t.Completed {
			completedItems = append(completedItems, t)
		} else {
			pending = append(pending, t)
		}
	}

	printTasks(stdout, pending, "Pending Tasks")
	if len(completedItems) > 0 {
		fmt.Fprintln(stdout, "\n--- Completed ---")
		printTasks(stdout, completedItems, "Completed Tasks")
	}
	return nil
}

func formatTaskLine(t model.Task) string {
	meta := summarizeMeta(t.MetaJSON)
	nc := countNotes(t.NotesJSON)
	
	statusIcon := "[ ]"
	if t.Completed {
		statusIcon = "[x]"
	}
	
	// ID [ ] Title
	line := fmt.Sprintf("%-3d %s %s", t.ID, statusIcon, t.Title)

	// Indicators
	var indicators []string
	if meta.ParentID != nil {
		indicators = append(indicators, fmt.Sprintf("↑%d", *meta.ParentID))
	}
	if t.TargetAt != nil && !t.Completed && t.TargetAt.Before(time.Now()) {
		indicators = append(indicators, "⚠ OVERDUE")
	}
	
	// Missing fields check
	var missing []string
	if t.Source == "" {
		missing = append(missing, "src")
	}
	if meta.Kind == "" {
		missing = append(missing, "kind")
	}
	if nc == 0 {
		missing = append(missing, "note")
	}
	if len(missing) > 0 && !t.Completed {
		indicators = append(indicators, fmt.Sprintf("!miss(%s)", strings.Join(missing, ",")))
	}

	// Task size check (arbitrary: title > 80 or notes > 3)
	if len(t.Title) > 80 || nc > 3 {
		indicators = append(indicators, "LARGE")
	}

	if len(indicators) > 0 {
		line += " | " + strings.Join(indicators, " ")
	}

	// Minimal Tags: p0 source kind
	var tags []string
	if t.Priority != 0 {
		tags = append(tags, fmt.Sprintf("p%d", t.Priority))
	}
	if t.Source != "" {
		tags = append(tags, t.Source)
	}
	if meta.Kind != "" {
		tags = append(tags, meta.Kind)
	}
	if len(tags) > 0 {
		line += " #" + strings.Join(tags, " #")
	}

	// Minimal Time: 2h ago / in 3d / no time
	if t.TargetAt != nil {
		rel := humanize.RelTime(*t.TargetAt, time.Now(), "", "")
		if strings.HasSuffix(rel, " from now") {
			rel = "in " + strings.TrimSuffix(rel, " from now")
		}
		line += " " + rel
	}

	// Simple indicators
	if strings.Contains(t.MetaJSON, "recurrence") {
		line += " ↺"
	}
	if strings.Contains(t.MetaJSON, "monitor_cmd") {
		line += " 👁"
	}

	return line
}

func countNotes(raw string) int {
	if strings.TrimSpace(raw) == "" || raw == "[]" || raw == "null" {
		return 0
	}
	var notes []any
	if err := json.Unmarshal([]byte(raw), &notes); err != nil {
		return 0
	}
	return len(notes)
}

func runShow(ctx context.Context, svc service.Service, stdout io.Writer, args []string, jsonMode bool) error {
	fs := newFlagSet("show")
	csvMode := fs.Bool("csv", false, "output in csv format")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() == 0 {
		return fmt.Errorf("usage: gtask show <id1> [id2...]")
	}

	var tasks []model.Task
	for _, arg := range fs.Args() {
		id, err := strconv.ParseInt(arg, 10, 64)
		if err != nil {
			return fmt.Errorf("parse id %q: %w", arg, err)
		}
		task, err := svc.GetTask(ctx, id)
		if err != nil {
			return err
		}
		tasks = append(tasks, task)
	}

	if jsonMode {
		if len(tasks) == 1 {
			return json.NewEncoder(stdout).Encode(tasks[0])
		}
		return json.NewEncoder(stdout).Encode(tasks)
	}

	if *csvMode {
		fmt.Fprintln(stdout, "id,title,status,priority,source,kind,parent_id,target_at,updated_at")
		for _, task := range tasks {
			metaSummary := summarizeMeta(task.MetaJSON)
			fmt.Fprintf(stdout, "%d,%q,%s,%d,%q,%q,%s,%s,%s\n",
				task.ID,
				task.Title,
				status(task.Completed),
				task.Priority,
				task.Source,
				metaSummary.Kind,
				formatParent(metaSummary.ParentID),
				formatMaybe(task.TargetAt),
				task.UpdatedAt.UTC().Format(time.RFC3339),
			)
		}
		return nil
	}

	for i, task := range tasks {
		if i > 0 {
			fmt.Fprintln(stdout, "---")
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
	}
	return nil
}

func runUpdate(ctx context.Context, svc service.Service, stdout io.Writer, args []string, jsonMode bool) error {
	fs := newFlagSet("update")
	title := fs.String("title", "", "new title")
	priority := fs.Int("priority", 0, "new priority")
	source := fs.String("source", "", "new source")
	kind := fs.String("kind", "", "set meta.kind")
	parent := fs.String("parent", "", "set meta.parent_id, or null to clear")
	startAt := fs.String("start", "", "set start time")
	targetAt := fs.String("target", "", "set target time")
	startDays := fs.Int("start-days", 0, "set start days")
	days := fs.Int("days", 0, "set target days")
	meta := fs.String("meta", "", "replace metadata json")
	completed := fs.String("completed", "", "true or false")
	note := fs.String("note", "", "append note")
	monitorCmd := fs.String("monitor-cmd", "", "command to run periodically")
	monitorInterval := fs.String("monitor-interval", "", "how often to run monitor command")
	recurrence := fs.String("recurrence", "", "recurrence interval")

	if err := fs.Parse(args); err != nil {
		return err
	}

	idArgs := fs.Args()
	if len(idArgs) == 0 {
		return fmt.Errorf("usage: gtask update <id1,id2,...> [flags] or gtask update [flags] <id1> <id2>")
	}

	var ids []int64
	for _, idArg := range idArgs {
		parts := strings.Split(idArg, ",")
		for _, p := range parts {
			if p == "" {
				continue
			}
			id, err := strconv.ParseInt(p, 10, 64)
			if err != nil {
				return fmt.Errorf("parse id %q: %w", p, err)
			}
			ids = append(ids, id)
		}
	}

	var updatedTasks []model.Task
	for _, id := range ids {
		var in store.UpdateInput
		in.ID = id
		if hasFlag(args, "title") {
			in.Title = title
		}
		if hasFlag(args, "priority") {
			in.Priority = priority
		}
		if hasFlag(args, "source") {
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
		if strings.TrimSpace(*meta) != "" || hasFlag(args, "kind") || hasFlag(args, "parent") ||
			hasFlag(args, "monitor-cmd") || hasFlag(args, "monitor-interval") || hasFlag(args, "recurrence") {
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
			metaJSON, err := mergeMeta(baseMeta, metaUpdates{
				kind:            strings.TrimSpace(*kind),
				parentSet:       parentSet,
				parent:          parentValue,
				monitorCmd:      *monitorCmd,
				monitorInterval: *monitorInterval,
				recurrence:      *recurrence,
			})
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
		updatedTasks = append(updatedTasks, task)
	}

	if jsonMode {
		if len(updatedTasks) == 1 {
			return json.NewEncoder(stdout).Encode(updatedTasks[0])
		}
		return json.NewEncoder(stdout).Encode(updatedTasks)
	}
	for _, task := range updatedTasks {
		fmt.Fprintf(stdout, "updated task %d: %s\n", task.ID, task.Title)
	}
	return nil
}

func runDelete(ctx context.Context, svc service.Service, stdout io.Writer, args []string, jsonMode bool) error {
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
	if jsonMode {
		return json.NewEncoder(stdout).Encode(map[string]any{"id": id, "deleted": true})
	}
	fmt.Fprintf(stdout, "deleted task %d\n", id)
	return nil
}

func printUsage(w io.Writer) {
	const help = `gtask: A local-first task CLI with Google Tasks sync

Usage:
  gtask [command] [flags]

Global Flags:
  --json    Output in JSON format

Core Commands:
  add       Create a new task
  todo      List pending tasks (CSV format)
  done      List completed tasks (CSV format)
  filter    Search and filter tasks with advanced criteria
  show      Show full details of tasks by IDs
  update    Modify tasks by IDs
  delete    Remove a task by ID
  sync      Synchronize local tasks with Google Tasks
  daemon    Start background RPC server for faster access and notifications
  upgrade   Upgrade gtask binary to the latest version
  version   Print version information

Command Details:
  add [title] [note]   Quick add or use flags for full control.
     --title TEXT      Task title (required if first positional arg is empty)
     --priority N      Task priority (default 0)
     --source TEXT     Task source (e.g. github, manual)
     --kind TEXT       Task kind (stored in meta.kind)
     --parent ID       Parent task ID (stored in meta.parent_id)
     --target TIME     Target date/time (e.g. '2026-04-15 23')
     --days N          Target time set to N days from now
     --note TEXT       Initial note for the task
     --meta JSON       Direct JSON metadata
     --monitor-cmd STR Run command periodically; completes task if exit code is 0
     --monitor-interval DUR How often to run monitor (default 10m, e.g. 1m, 1h)
     --recurrence DUR  Repeat task after completion (e.g. 24h, 1h)

  show <id1> [id2...]
     --csv             Output in CSV format

  update <id1,id2,...> or update [flags] <id1> <id2>
     --completed B     Mark as done (true) or todo (false)
     --note TEXT       Append a new note to the notes history
     --target null     Use 'null' to clear target/start time fields
     --recurrence DUR  Update recurrence interval

Time Formats:
  RFC3339:         2026-04-15T23:00:00+08:00
  Short form:      2026-04-15 23:30  (local time)
  Date only:       2026-04-15        (00:00 local time)
  Relative days:   --days 3          (Current time + 3 days)

Duration Formats:
  10m, 1h, 24h, 168h (7 days)

Environment Variables:
  GTASK_HOST       Daemon host (default 127.0.0.1)
  GTASK_PORT       Daemon port (default 8765)

Examples:
  gtask add "Buy milk" --days 1
  gtask upgrade
  gtask sync
`
	fmt.Fprint(w, help)
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
	fs.SetOutput(os.Stderr)
	fs.Usage = func() {
		switch name {
		case "add":
			fmt.Fprintln(os.Stderr, "usage: gtask add [title] [note] [--title <title>] [--priority N] [--source X] [--kind K] [--parent ID] [--start TIME] [--start-days N] [--target TIME] [--days N] [--meta JSON] [--note TEXT] [--monitor-cmd STR] [--monitor-interval DUR] [--recurrence DUR]")
		case "todo":
			fmt.Fprintln(os.Stderr, "usage: gtask todo")
		case "done":
			fmt.Fprintln(os.Stderr, "usage: gtask done")
		case "filter":
			fmt.Fprintln(os.Stderr, "usage: gtask filter [--all] [--source X] [--kind K] [--parent ID] [--query TEXT] [--completed true|false] [--priority-min N] [--priority-max N]")
		case "show":
			fmt.Fprintln(os.Stderr, "usage: gtask show <id1> [id2...] [--csv]")
		case "update":
			fmt.Fprintln(os.Stderr, "usage: gtask update <id1,id2,...> [flags] or gtask update [flags] <id1> <id2>")
		case "delete":
			fmt.Fprintln(os.Stderr, "usage: gtask delete <id>")
		case "daemon":
			fmt.Fprintln(os.Stderr, "usage: gtask daemon [--host 127.0.0.1] [--port 8765]")
		case "upgrade":
			fmt.Fprintln(os.Stderr, "usage: gtask upgrade")
		default:
			fmt.Fprintf(os.Stderr, "usage: gtask %s\n", name)
		}
		fmt.Fprintln(os.Stderr, "\nflags:")
		fs.PrintDefaults()
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
