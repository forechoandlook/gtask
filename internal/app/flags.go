package app

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

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

func parseIDs(args []string) ([]int64, error) {
	var ids []int64
	for _, arg := range args {
		parts := strings.Split(arg, ",")
		for _, p := range parts {
			if p == "" {
				continue
			}
			id, err := strconv.ParseInt(p, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("parse id %q: %w", p, err)
			}
			ids = append(ids, id)
		}
	}
	return ids, nil
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
			fmt.Fprintln(os.Stderr, "usage: gtask show [--csv] <id1> [id2...]")
		case "update":
			fmt.Fprintln(os.Stderr, "usage: gtask update [flags] <id1> [id2...]")
		case "delete":
			fmt.Fprintln(os.Stderr, "usage: gtask delete <id1> [id2...]")
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
