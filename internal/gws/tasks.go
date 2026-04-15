package gws

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

type Client struct{}

type TaskList struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

type TaskListsResponse struct {
	Items []TaskList `json:"items"`
}

type RemoteTask struct {
	ID string `json:"id"`
}

func (Client) ListTaskLists(ctx context.Context) ([]TaskList, error) {
	var resp TaskListsResponse
	if err := runJSON(ctx, &resp, "tasks", "tasklists", "list"); err != nil {
		return nil, err
	}
	return resp.Items, nil
}

func (Client) InsertTask(ctx context.Context, taskListID string, payload map[string]any) (RemoteTask, error) {
	var out RemoteTask
	if err := runJSON(ctx, &out, "tasks", "tasks", "insert", "--params", fmt.Sprintf(`{"tasklist":%q}`, taskListID), "--json", mustJSON(payload)); err != nil {
		return RemoteTask{}, err
	}
	return out, nil
}

func (Client) UpdateTask(ctx context.Context, taskListID, taskID string, payload map[string]any) (RemoteTask, error) {
	var out RemoteTask
	if err := runJSON(ctx, &out, "tasks", "tasks", "update", "--params", fmt.Sprintf(`{"tasklist":%q,"task":%q}`, taskListID, taskID), "--json", mustJSON(payload)); err != nil {
		return RemoteTask{}, err
	}
	return out, nil
}

func runJSON(ctx context.Context, out any, args ...string) error {
	cmd := exec.CommandContext(ctx, "gws", args...)
	raw, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("run gws %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(raw)))
	}
	trimmed := extractJSON(raw)
	if err := json.Unmarshal(trimmed, out); err != nil {
		return fmt.Errorf("decode gws output: %w", err)
	}
	return nil
}

func mustJSON(v any) string {
	raw, _ := json.Marshal(v)
	return string(raw)
}

func extractJSON(raw []byte) []byte {
	text := strings.TrimSpace(string(raw))
	idxObj := strings.IndexByte(text, '{')
	idxArr := strings.IndexByte(text, '[')
	switch {
	case idxObj >= 0 && idxArr >= 0 && idxArr < idxObj:
		return []byte(text[idxArr:])
	case idxObj >= 0:
		return []byte(text[idxObj:])
	case idxArr >= 0:
		return []byte(text[idxArr:])
	default:
		return []byte(text)
	}
}
