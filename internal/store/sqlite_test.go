package store

import (
	"context"
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
