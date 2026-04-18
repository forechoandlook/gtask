package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/forechoandlook/gtask/internal/model"
	"github.com/forechoandlook/gtask/internal/service"
	"github.com/forechoandlook/gtask/internal/store"
	"github.com/forechoandlook/gtask/internal/version"
)

func runAdd(ctx context.Context, svc service.Service, stdout io.Writer, args []string, jsonMode bool) error {
	fs := newFlagSet("add")
	title := fs.String("title", "", "task title")
	priority := fs.Int("priority", 0, "task priority")
	source := fs.String("source", "", "task source (e.g. github, manual)")
	kind := fs.String("kind", "", "task kind (meta.kind)")
	parent := fs.Int64("parent", 0, "parent task id (meta.parent_id)")
	startAt := fs.String("start", "", "start time")
	targetAt := fs.String("target", "", "target time")
	startDays := fs.Int("start-days", 0, "start days from now")
	days := fs.Int("days", 0, "target days from now")
	meta := fs.String("meta", "{}", "extra metadata json")
	note := fs.String("note", "", "initial note")
	monitorCmd := fs.String("monitor-cmd", "", "command to run periodically")
	monitorInterval := fs.String("monitor-interval", "10m", "how often to run monitor")
	recurrence := fs.String("recurrence", "", "recurrence interval")

	if err := fs.Parse(args); err != nil {
		return err
	}

	arg0 := fs.Arg(0)
	arg1 := fs.Arg(1)

	finalTitle := *title
	finalNote := *note

	if finalTitle == "" && arg0 != "" {
		finalTitle = arg0
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

func runFilter(ctx context.Context, svc service.Service, stdout io.Writer, args []string, jsonMode bool) error {
	fs := newFlagSet("filter")
	all := fs.Bool("all", false, "show both todo and done tasks")
	source := fs.String("source", "", "filter by source")
	kind := fs.String("kind", "", "filter by kind")
	parent := fs.Int64("parent", -1, "filter by parent_id (0 for no parent)")
	query := fs.String("query", "", "filter by title/note content")
	completedStr := fs.String("completed", "", "filter by status (true/false)")
	priorityMin := fs.Int("priority-min", -1, "filter by minimum priority")
	priorityMax := fs.Int("priority-max", -1, "filter by maximum priority")

	if err := fs.Parse(args); err != nil {
		return err
	}

	var filter store.ListFilter
	if !*all {
		f := false
		filter.Completed = &f
	}
	if strings.TrimSpace(*completedStr) != "" {
		v, err := strconv.ParseBool(*completedStr)
		if err != nil {
			return fmt.Errorf("parse completed: %w", err)
		}
		filter.Completed = &v
	}
	if strings.TrimSpace(*source) != "" {
		filter.Source = *source
	}
	if strings.TrimSpace(*kind) != "" {
		filter.Kind = *kind
	}
	if *parent != -1 {
		filter.ParentID = parent
	}
	if strings.TrimSpace(*query) != "" {
		filter.Query = *query
	}
	if *priorityMin != -1 {
		filter.PriorityMin = priorityMin
	}
	if *priorityMax != -1 {
		filter.PriorityMax = priorityMax
	}

	tasks, err := svc.ListTasksFiltered(ctx, filter)
	if err != nil {
		return err
	}

	if jsonMode {
		return json.NewEncoder(stdout).Encode(tasks)
	}

	printTasks(stdout, tasks, "")
	return nil
}

func runShow(ctx context.Context, svc service.Service, stdout io.Writer, args []string, jsonMode bool) error {
	fs := newFlagSet("show")
	csvMode := fs.Bool("csv", false, "output in csv format")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() == 0 {
		return fmt.Errorf("usage: gtask show [--csv] <id1> [id2...]")
	}

	ids, err := parseIDs(fs.Args())
	if err != nil {
		return err
	}

	var tasks []model.Task
	for _, id := range ids {
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
		printTasks(stdout, tasks, "")
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

	ids, err := parseIDs(fs.Args())
	if err != nil {
		return err
	}
	if len(ids) == 0 {
		return fmt.Errorf("usage: gtask update [flags] <id1> [id2...]")
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

	ids, err := parseIDs(fs.Args())
	if err != nil {
		return err
	}
	if len(ids) == 0 {
		return fmt.Errorf("usage: gtask delete <id1> [id2...]")
	}

	var deletedIDs []int64
	for _, id := range ids {
		if err := svc.DeleteTask(ctx, id); err != nil {
			return err
		}
		deletedIDs = append(deletedIDs, id)
	}

	if jsonMode {
		if len(deletedIDs) == 1 {
			return json.NewEncoder(stdout).Encode(map[string]any{"id": deletedIDs[0], "deleted": true})
		}
		return json.NewEncoder(stdout).Encode(map[string]any{"ids": deletedIDs, "deleted": true})
	}
	for _, id := range deletedIDs {
		fmt.Fprintf(stdout, "deleted task %d\n", id)
	}
	return nil
}

func runVersion(stdout io.Writer) {
	fmt.Fprintf(stdout, "gtask %s (%s)\n", version.Version, version.Commit)
}
