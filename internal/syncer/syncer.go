package syncer

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/forechoandlook/gtask/internal/config"
	"github.com/forechoandlook/gtask/internal/gws"
	"github.com/forechoandlook/gtask/internal/model"
	"github.com/forechoandlook/gtask/internal/store"
)

type Syncer struct {
	cfg   config.Config
	store *store.Store
	gws   gws.Client
}

func New(cfg config.Config, st *store.Store) *Syncer {
	return &Syncer{cfg: cfg, store: st}
}

func (s *Syncer) Sync(ctx context.Context) (string, error) {
	listID, err := s.resolveTaskListID(ctx)
	if err != nil {
		return "", err
	}
	tasks, err := s.store.ListTasks(ctx, true)
	if err != nil {
		return "", err
	}
	created := 0
	updated := 0
	for _, task := range tasks {
		payload := buildPayload(task)
		if strings.TrimSpace(task.GoogleTaskID) == "" {
			remote, err := s.gws.InsertTask(ctx, listID, payload)
			if err != nil {
				return "", fmt.Errorf("sync local task %d: %w", task.ID, err)
			}
			if err := s.store.UpsertSyncState(ctx, task.ID, listID, remote.ID, time.Now().UTC()); err != nil {
				return "", err
			}
			created++
			continue
		}
		if _, err := s.gws.UpdateTask(ctx, listID, task.GoogleTaskID, payload); err != nil {
			return "", fmt.Errorf("update remote task %d: %w", task.ID, err)
		}
		if err := s.store.UpsertSyncState(ctx, task.ID, listID, task.GoogleTaskID, time.Now().UTC()); err != nil {
			return "", err
		}
		updated++
	}
	return fmt.Sprintf("synced %d tasks to %s (%d created, %d updated)", len(tasks), listID, created, updated), nil
}

func (s *Syncer) resolveTaskListID(ctx context.Context) (string, error) {
	if strings.TrimSpace(s.cfg.DefaultGoogleListID) != "" {
		return s.cfg.DefaultGoogleListID, nil
	}
	lists, err := s.gws.ListTaskLists(ctx)
	if err != nil {
		return "", err
	}
	for _, item := range lists {
		if item.Title == s.cfg.DefaultGoogleList {
			s.cfg.DefaultGoogleListID = item.ID
			if err := config.Save(s.cfg); err != nil {
				return "", err
			}
			return item.ID, nil
		}
	}
	return "", fmt.Errorf("google task list %q not found", s.cfg.DefaultGoogleList)
}

func buildPayload(task model.Task) map[string]any {
	payload := map[string]any{
		"title":  task.Title,
		"status": "needsAction",
		"notes":  buildNotes(task),
	}
	if strings.TrimSpace(task.GoogleTaskID) != "" {
		payload["id"] = task.GoogleTaskID
	}
	if task.Completed {
		payload["status"] = "completed"
		payload["completed"] = time.Now().UTC().Format(time.RFC3339)
	}
	if task.TargetAt != nil {
		payload["due"] = task.TargetAt.UTC().Format(time.RFC3339)
	}
	return payload
}

func buildNotes(task model.Task) string {
	body := map[string]any{
		"local_id":   task.ID,
		"priority":   task.Priority,
		"source":     task.Source,
		"start_at":   formatTime(task.StartAt),
		"target_at":  formatTime(task.TargetAt),
		"updated_at": task.UpdatedAt.UTC().Format(time.RFC3339),
		"meta":       json.RawMessage(task.MetaJSON),
		"notes":      json.RawMessage(task.NotesJSON),
	}
	raw, _ := json.MarshalIndent(body, "", "  ")
	return string(raw)
}

func formatTime(v *time.Time) any {
	if v == nil {
		return nil
	}
	return v.UTC().Format(time.RFC3339)
}
