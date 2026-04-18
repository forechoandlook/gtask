package app

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"

	"github.com/forechoandlook/gtask/internal/config"
	"github.com/forechoandlook/gtask/internal/daemon"
	"github.com/forechoandlook/gtask/internal/service"
	"github.com/forechoandlook/gtask/internal/store"
)

func getHostPort(args []string) (string, string, []string) {
	host := os.Getenv("GTASK_HOST")
	if host == "" {
		host = "127.0.0.1"
	}
	port := os.Getenv("GTASK_PORT")
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
	host, port, args := getHostPort(args)

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

	if len(args) < 1 {
		printUsage(stdout)
		return nil
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	cmd := args[0]
	if cmd == "daemon" {
		st, err := store.Open(cfg.DBPath)
		if err != nil {
			return err
		}
		svc := service.New(cfg, st)
		return daemon.NewDaemon(svc, host, port).Start()
	}

	client, err := daemon.NewRPCClient("tcp", net.JoinHostPort(host, port))
	var svc service.Service
	if err != nil {
		// fallback to local
		st, err := store.Open(cfg.DBPath)
		if err != nil {
			return err
		}
		svc = service.New(cfg, st)
	} else {
		svc = client
	}

	var cmdErr error
	switch cmd {
	case "add":
		cmdErr = runAdd(ctx, svc, stdout, args, jsonMode)
	case "todo":
		cmdErr = runTodo(ctx, svc, stdout, args, jsonMode)
	case "done":
		cmdErr = runDone(ctx, svc, stdout, args, jsonMode)
	case "filter":
		cmdErr = runFilter(ctx, svc, stdout, args, jsonMode)
	case "show":
		cmdErr = runShow(ctx, svc, stdout, args, jsonMode)
	case "update":
		cmdErr = runUpdate(ctx, svc, stdout, args, jsonMode)
	case "delete":
		cmdErr = runDelete(ctx, svc, stdout, args, jsonMode)
	case "sync":
		msg, err := svc.Sync(ctx)
		if err == nil {
			if jsonMode {
				fmt.Fprintf(stdout, `{"status":"success","message":%q}`, msg)
				fmt.Fprintln(stdout)
			} else {
				fmt.Fprintln(stdout, msg)
			}
		}
		cmdErr = err
	case "upgrade":
		cmdErr = runSelfUpgrade(ctx, stdout)
	case "version":
		runVersion(stdout)
	case "help", "-h", "--help":
		printUsage(stdout)
	default:
		printUsage(stderr)
		cmdErr = fmt.Errorf("unknown command: %s", cmd)
	}

	return cmdErr
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
  delete <id1> [id2...]  Remove tasks by IDs
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

  show [--csv] <id1> [id2...]
     --csv             Output in CSV format

  update [flags] <id1> [id2...]
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
