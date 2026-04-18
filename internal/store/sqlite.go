package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"github.com/forechoandlook/gtask/internal/model"
)

type Store struct {
	db *sql.DB
}

type ListFilter struct {
	IncludeCompleted bool
	Completed        *bool
	Source           string
	Kind             string
	ParentID         *int64
	Query            string
	PriorityMin      *int
	PriorityMax      *int
}

type AddInput struct {
	Title    string
	Priority int
	Source   string
	StartAt  *time.Time
	TargetAt *time.Time
	MetaJSON string
	Note     string
}

type UpdateInput struct {
	ID         int64
	Title      *string
	Priority   *int
	Source     *string
	StartAt    **time.Time
	TargetAt   **time.Time
	MetaJSON   *string
	Completed  *bool
	AppendNote string
}

func Open(dbPath string) (*Store, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	s := &Store{db: db}
	if err := s.configure(); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) configure() error {
	if _, err := s.db.Exec(`PRAGMA busy_timeout = 5000;`); err != nil {
		return fmt.Errorf("configure sqlite timeout: %w", err)
	}
	if _, err := s.db.Exec(`PRAGMA journal_mode = WAL;`); err != nil {
		return fmt.Errorf("configure sqlite wal: %w", err)
	}
	return nil
}

func (s *Store) migrate() error {
	const q = `
CREATE TABLE IF NOT EXISTS tasks (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	title TEXT NOT NULL,
	priority INTEGER NOT NULL DEFAULT 0,
	source TEXT NOT NULL DEFAULT '',
	start_at TEXT NULL,
	target_at TEXT NULL,
	updated_at TEXT NOT NULL,
	meta_json TEXT NOT NULL DEFAULT '{}',
	completed INTEGER NOT NULL DEFAULT 0,
	notes_json TEXT NOT NULL DEFAULT '[]',
	google_task_list_id TEXT NOT NULL DEFAULT '',
	google_task_id TEXT NOT NULL DEFAULT '',
	last_synced_at TEXT NULL
);
`
	if _, err := s.db.Exec(q); err != nil {
		return fmt.Errorf("migrate sqlite: %w", err)
	}
	return nil
}

func (s *Store) AddTask(ctx context.Context, in AddInput) (model.Task, error) {
	now := time.Now().UTC()
	notesJSON := "[]"
	if strings.TrimSpace(in.Note) != "" {
		raw, err := json.Marshal([]model.Note{{At: now, Text: strings.TrimSpace(in.Note)}})
		if err != nil {
			return model.Task{}, fmt.Errorf("marshal notes: %w", err)
		}
		notesJSON = string(raw)
	}
	if strings.TrimSpace(in.MetaJSON) == "" {
		in.MetaJSON = "{}"
	}
	res, err := s.db.ExecContext(ctx, `
INSERT INTO tasks(title, priority, source, start_at, target_at, updated_at, meta_json, completed, notes_json)
VALUES(?, ?, ?, ?, ?, ?, ?, 0, ?)`,
		strings.TrimSpace(in.Title),
		in.Priority,
		strings.TrimSpace(in.Source),
		timePtrString(in.StartAt),
		timePtrString(in.TargetAt),
		now.Format(time.RFC3339),
		in.MetaJSON,
		notesJSON,
	)
	if err != nil {
		return model.Task{}, fmt.Errorf("insert task: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return model.Task{}, fmt.Errorf("last insert id: %w", err)
	}
	return s.GetTask(ctx, id)
}

func (s *Store) GetTask(ctx context.Context, id int64) (model.Task, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT id, title, priority, source, start_at, target_at, updated_at, meta_json, completed, notes_json, google_task_list_id, google_task_id, last_synced_at
FROM tasks WHERE id = ?`, id)
	return scanTask(row)
}

func (s *Store) ListTasks(ctx context.Context, includeCompleted bool) ([]model.Task, error) {
	return s.ListTasksFiltered(ctx, ListFilter{IncludeCompleted: includeCompleted})
}

func (s *Store) ListTasksFiltered(ctx context.Context, filter ListFilter) ([]model.Task, error) {
	q := `
SELECT id, title, priority, source, start_at, target_at, updated_at, meta_json, completed, notes_json, google_task_list_id, google_task_id, last_synced_at
FROM tasks`
	var clauses []string
	args := []any{}
	if !filter.IncludeCompleted && filter.Completed == nil {
		clauses = append(clauses, `completed = 0`)
	}
	if filter.Completed != nil {
		clauses = append(clauses, `completed = ?`)
		args = append(args, boolToInt(*filter.Completed))
	}
	if strings.TrimSpace(filter.Source) != "" {
		clauses = append(clauses, `source = ?`)
		args = append(args, strings.TrimSpace(filter.Source))
	}
	if strings.TrimSpace(filter.Kind) != "" {
		clauses = append(clauses, `json_extract(meta_json, '$.kind') = ?`)
		args = append(args, strings.TrimSpace(filter.Kind))
	}
	if filter.ParentID != nil {
		if *filter.ParentID == 0 {
			clauses = append(clauses, `json_extract(meta_json, '$.parent_id') IS NULL`)
		} else {
			clauses = append(clauses, `json_extract(meta_json, '$.parent_id') = ?`)
			args = append(args, *filter.ParentID)
		}
	}
	if strings.TrimSpace(filter.Query) != "" {
		clauses = append(clauses, `(title LIKE ? OR meta_json LIKE ? OR notes_json LIKE ?)`)
		like := "%" + strings.TrimSpace(filter.Query) + "%"
		args = append(args, like, like, like)
	}
	if filter.PriorityMin != nil {
		clauses = append(clauses, `priority >= ?`)
		args = append(args, *filter.PriorityMin)
	}
	if filter.PriorityMax != nil {
		clauses = append(clauses, `priority <= ?`)
		args = append(args, *filter.PriorityMax)
	}
	if len(clauses) > 0 {
		q += ` WHERE ` + strings.Join(clauses, ` AND `)
	}
	q += ` ORDER BY completed ASC, priority DESC, COALESCE(target_at, '9999-12-31T00:00:00Z') ASC, updated_at DESC, id DESC`
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("list tasks: %w", err)
	}
	defer rows.Close()

	var tasks []model.Task
	for rows.Next() {
		task, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}
	return tasks, rows.Err()
}

func (s *Store) DeleteTask(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM tasks WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete task: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete task affected rows: %w", err)
	}
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) UpdateTask(ctx context.Context, in UpdateInput) (model.Task, error) {
	task, err := s.GetTask(ctx, in.ID)
	if err != nil {
		return model.Task{}, err
	}
	if in.Title != nil {
		task.Title = strings.TrimSpace(*in.Title)
	}
	if in.Priority != nil {
		task.Priority = *in.Priority
	}
	if in.Source != nil {
		task.Source = strings.TrimSpace(*in.Source)
	}
	if in.StartAt != nil {
		task.StartAt = *in.StartAt
	}
	if in.TargetAt != nil {
		task.TargetAt = *in.TargetAt
	}
	if in.MetaJSON != nil && strings.TrimSpace(*in.MetaJSON) != "" {
		task.MetaJSON = *in.MetaJSON
	}
	if in.Completed != nil {
		oldCompleted := task.Completed
		task.Completed = *in.Completed
		if task.Completed && !oldCompleted {
			// Marked as completed, record timestamp in meta
			var meta map[string]any
			if err := json.Unmarshal([]byte(task.MetaJSON), &meta); err == nil {
				if meta == nil {
					meta = make(map[string]any)
				}
				meta["completed_at"] = time.Now().UTC().Format(time.RFC3339)
				newMeta, _ := json.Marshal(meta)
				task.MetaJSON = string(newMeta)
			}
		}
	}
	if strings.TrimSpace(in.AppendNote) != "" {
		var notes []model.Note
		if err := json.Unmarshal([]byte(task.NotesJSON), &notes); err != nil {
			return model.Task{}, fmt.Errorf("parse notes: %w", err)
		}
		notes = append(notes, model.Note{At: time.Now().UTC(), Text: strings.TrimSpace(in.AppendNote)})
		raw, err := json.Marshal(notes)
		if err != nil {
			return model.Task{}, fmt.Errorf("marshal notes: %w", err)
		}
		task.NotesJSON = string(raw)
	}
	task.UpdatedAt = time.Now().UTC()

	_, err = s.db.ExecContext(ctx, `
UPDATE tasks
SET title = ?, priority = ?, source = ?, start_at = ?, target_at = ?, updated_at = ?, meta_json = ?, completed = ?, notes_json = ?
WHERE id = ?`,
		task.Title,
		task.Priority,
		task.Source,
		timePtrString(task.StartAt),
		timePtrString(task.TargetAt),
		task.UpdatedAt.Format(time.RFC3339),
		task.MetaJSON,
		boolToInt(task.Completed),
		task.NotesJSON,
		task.ID,
	)
	if err != nil {
		return model.Task{}, fmt.Errorf("update task: %w", err)
	}
	return s.GetTask(ctx, task.ID)
}

func (s *Store) UpsertSyncState(ctx context.Context, id int64, listID, taskID string, syncedAt time.Time) error {
	_, err := s.db.ExecContext(ctx, `
UPDATE tasks SET google_task_list_id = ?, google_task_id = ?, last_synced_at = ?, updated_at = updated_at
WHERE id = ?`, listID, taskID, syncedAt.UTC().Format(time.RFC3339), id)
	if err != nil {
		return fmt.Errorf("update sync state: %w", err)
	}
	return nil
}

type scanner interface {
	Scan(dest ...any) error
}

func scanTask(s scanner) (model.Task, error) {
	var task model.Task
	var startAt, targetAt, updatedAt, lastSyncedAt sql.NullString
	var completed int
	if err := s.Scan(
		&task.ID,
		&task.Title,
		&task.Priority,
		&task.Source,
		&startAt,
		&targetAt,
		&updatedAt,
		&task.MetaJSON,
		&completed,
		&task.NotesJSON,
		&task.GoogleTaskListID,
		&task.GoogleTaskID,
		&lastSyncedAt,
	); err != nil {
		return model.Task{}, fmt.Errorf("scan task: %w", err)
	}
	task.Completed = completed == 1
	task.StartAt = parseNullableTime(startAt)
	task.TargetAt = parseNullableTime(targetAt)
	parsedUpdated := parseNullableTime(updatedAt)
	if parsedUpdated != nil {
		task.UpdatedAt = *parsedUpdated
	}
	task.LastSyncedAt = parseNullableTime(lastSyncedAt)
	return task, nil
}

func parseNullableTime(v sql.NullString) *time.Time {
	if !v.Valid || strings.TrimSpace(v.String) == "" {
		return nil
	}
	t, err := time.Parse(time.RFC3339, v.String)
	if err != nil {
		return nil
	}
	return &t
}

func timePtrString(t *time.Time) any {
	if t == nil {
		return nil
	}
	return t.UTC().Format(time.RFC3339)
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
