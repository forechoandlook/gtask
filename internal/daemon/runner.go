package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/rpc"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/forechoandlook/gtask/internal/model"
	"github.com/forechoandlook/gtask/internal/service"
	"github.com/forechoandlook/gtask/internal/store"
	"github.com/gen2brain/beeep"
)

type Daemon struct {
	svc  service.Service
	host string
	port string
}

func NewDaemon(svc service.Service, host, port string) *Daemon {
	return &Daemon{
		svc:  svc,
		host: host,
		port: port,
	}
}

func (d *Daemon) notify(title, msg string) {
	err := beeep.Notify(title, msg, "")
	if err != nil {
		log.Printf("notify err: %v", err)
	}
}

func (d *Daemon) Start() error {
	addr := net.JoinHostPort(d.host, d.port)
	l, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	log.Printf("gtask daemon listening on %s", addr)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigChan
		log.Printf("received signal %v, shutting down...", sig)
		cancel()
		l.Close()
	}()

	return d.serveListener(ctx, l)
}

func (d *Daemon) serveListener(ctx context.Context, l net.Listener) error {
	rpcServer := rpc.NewServer()
	srv := &RPCServer{
		svc:    d.svc,
		notify: d.notify,
	}
	err := rpcServer.RegisterName("RPCServer", srv)
	if err != nil {
		return err
	}

	go d.checker(ctx)

	for {
		conn, err := l.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return nil
			default:
				log.Printf("accept err: %v", err)
				continue
			}
		}
		go rpcServer.ServeConn(conn)
	}
}

func (d *Daemon) checker(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			log.Printf("checker stopping...")
			return
		case <-ticker.C:
			tasks, err := d.svc.ListTasksFiltered(ctx, store.ListFilter{IncludeCompleted: true})
			if err != nil {
				log.Printf("checker list tasks err: %v", err)
				continue
			}

			now := time.Now()
			for _, t := range tasks {
				if !t.Completed {
					d.handleMonitor(ctx, t, now)
					if t.StartAt != nil && isClose(now, *t.StartAt) {
						d.notify("Task Starting Soon", t.Title)
					}
					if t.TargetAt != nil && isClose(now, *t.TargetAt) {
						d.notify("Task Target Approaching", t.Title)
					}
				} else {
					d.handleRecurrence(ctx, t, now)
				}
			}
		}
	}
}

func (d *Daemon) handleMonitor(ctx context.Context, t model.Task, now time.Time) {
	var meta map[string]any
	if err := json.Unmarshal([]byte(t.MetaJSON), &meta); err != nil {
		return
	}

	cmdStr, _ := meta["monitor_cmd"].(string)
	if cmdStr == "" {
		return
	}

	intervalStr, _ := meta["monitor_interval"].(string)
	if intervalStr == "" {
		intervalStr = "10m"
	}
	interval, err := time.ParseDuration(intervalStr)
	if err != nil {
		interval = 10 * time.Minute
	}

	lastStr, _ := meta["last_monitored_at"].(string)
	var last time.Time
	if lastStr != "" {
		last, _ = time.Parse(time.RFC3339, lastStr)
	}

	if now.Sub(last) < interval {
		return
	}

	log.Printf("monitoring task %d: running %s", t.ID, cmdStr)
	cmd := exec.Command("sh", "-c", cmdStr)
	out, err := cmd.CombinedOutput()
	outputStr := string(out)

	meta["last_monitored_at"] = now.Format(time.RFC3339)
	metaJSON, _ := json.Marshal(meta)
	newMeta := string(metaJSON)

	if err == nil {
		log.Printf("task %d monitor condition met!", t.ID)
		d.notify("Monitor Condition Met", fmt.Sprintf("Task %d: %s", t.ID, t.Title))
		completed := true
		_, updateErr := d.svc.UpdateTask(ctx, store.UpdateInput{
			ID:        t.ID,
			Completed: &completed,
			MetaJSON:  &newMeta,
			AppendNote: fmt.Sprintf("Monitor condition met (Exit Code 0). Output: %s", strings.TrimSpace(outputStr)),
		})
		if updateErr != nil {
			log.Printf("update task %d err: %v", t.ID, updateErr)
		}
	} else {
		_, _ = d.svc.UpdateTask(ctx, store.UpdateInput{
			ID:       t.ID,
			MetaJSON: &newMeta,
		})
	}
}

func (d *Daemon) handleRecurrence(ctx context.Context, t model.Task, now time.Time) {
	var meta map[string]any
	if err := json.Unmarshal([]byte(t.MetaJSON), &meta); err != nil {
		return
	}

	recurrenceStr, _ := meta["recurrence"].(string)
	if recurrenceStr == "" {
		return
	}

	interval, err := time.ParseDuration(recurrenceStr)
	if err != nil {
		return
	}

	lastBase := t.UpdatedAt
	if catStr, ok := meta["completed_at"].(string); ok && catStr != "" {
		if ct, err := time.Parse(time.RFC3339, catStr); err == nil {
			lastBase = ct
		}
	}

	if now.Sub(lastBase) < interval {
		return
	}

	log.Printf("respawning recurring task %d", t.ID)
	completed := false
	delete(meta, "last_monitored_at")
	delete(meta, "completed_at")
	metaJSON, _ := json.Marshal(meta)
	newMeta := string(metaJSON)

	var startAt, targetAt *time.Time
	if t.StartAt != nil {
		v := t.StartAt.Add(interval)
		startAt = &v
	}
	if t.TargetAt != nil {
		v := t.TargetAt.Add(interval)
		targetAt = &v
	}

	var startAtUpdate, targetAtUpdate **time.Time
	if startAt != nil {
		startAtUpdate = &startAt
	}
	if targetAt != nil {
		targetAtUpdate = &targetAt
	}

	_, updateErr := d.svc.UpdateTask(ctx, store.UpdateInput{
		ID:        t.ID,
		Completed: &completed,
		MetaJSON:  &newMeta,
		StartAt:   startAtUpdate,
		TargetAt:  targetAtUpdate,
		AppendNote: fmt.Sprintf("Recurring task respawned after %s", recurrenceStr),
	})
	if updateErr != nil {
		log.Printf("respawn task %d err: %v", t.ID, updateErr)
	} else {
		d.notify("Recurring Task Respawned", t.Title)
	}
}

func boolPtr(b bool) *bool {
	return &b
}

func isClose(t1, t2 time.Time) bool {
	diff := t1.Sub(t2)
	if diff < 0 {
		diff = -diff
	}
	return diff <= 1*time.Minute
}
