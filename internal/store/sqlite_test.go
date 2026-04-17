package store

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestAddListUpdateTask(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "gtask.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	ctx := context.Background()
	task, err := st.AddTask(ctx, AddInput{
		Title:    "first",
		Priority: 3,
		Source:   "manual",
		MetaJSON: `{"k":"v"}`,
		Note:     "seed",
	})
	if err != nil {
		t.Fatal(err)
	}
	if task.ID == 0 {
		t.Fatal("expected non-zero id")
	}

	all, err := st.ListTasks(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 {
		t.Fatalf("expected 1 task, got %d", len(all))
	}

	done := true
	updated, err := st.UpdateTask(ctx, UpdateInput{
		ID:         task.ID,
		Completed:  &done,
		AppendNote: "done",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !updated.Completed {
		t.Fatal("expected completed=true")
	}
}

func TestFilterAndDeleteTask(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "gtask.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	ctx := context.Background()
	_, err = st.AddTask(ctx, AddInput{Title: "alpha task", Priority: 1, Source: "cli", MetaJSON: `{"kind":"a"}`})
	if err != nil {
		t.Fatal(err)
	}
	beta, err := st.AddTask(ctx, AddInput{Title: "beta task", Priority: 4, Source: "idea1", MetaJSON: `{"kind":"b"}`, Note: "CDP flow"})
	if err != nil {
		t.Fatal(err)
	}

	tasks, err := st.ListTasksFiltered(ctx, ListFilter{IncludeCompleted: true, Source: "idea1", Query: "CDP"})
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 1 || tasks[0].ID != beta.ID {
		t.Fatalf("unexpected filtered tasks: %+v", tasks)
	}

	if err := st.DeleteTask(ctx, beta.ID); err != nil {
		t.Fatal(err)
	}
	_, err = st.GetTask(ctx, beta.ID)
	if err == nil {
		t.Fatal("expected task to be deleted")
	}
}

func TestUpdateRecordsCompletedAt(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "gtask.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	ctx := context.Background()
	task, _ := st.AddTask(ctx, AddInput{Title: "to complete"})

	done := true
	updated, err := st.UpdateTask(ctx, UpdateInput{
		ID:        task.ID,
		Completed: &done,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Verify meta contains completed_at
	var meta map[string]any
	if err := json.Unmarshal([]byte(updated.MetaJSON), &meta); err != nil {
		t.Fatal(err)
	}
	if _, ok := meta["completed_at"].(string); !ok {
		t.Fatalf("expected completed_at in meta, got: %s", updated.MetaJSON)
	}
}

func TestOpenConfiguresBusyTimeout(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "gtask.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	var busyTimeout int
	if err := st.db.QueryRow(`PRAGMA busy_timeout;`).Scan(&busyTimeout); err != nil {
		t.Fatal(err)
	}
	if busyTimeout != 5000 {
		t.Fatalf("expected busy_timeout=5000, got %d", busyTimeout)
	}
}
