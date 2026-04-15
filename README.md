# gtask

一个很小的 Go CLI，把本地任务先存进 `~/.gtask/gtask.db`，再同步到 Google Tasks。

当前版本：`0.1.1`

安装：

```bash
curl -fsSL https://raw.githubusercontent.com/forechoandlook/gtask/main/install.sh | bash
```

卸载：

```bash
curl -fsSL https://raw.githubusercontent.com/forechoandlook/gtask/main/uninstall.sh | bash
```

查看版本：

```bash
gtask --version
```

当前功能：

- `add`：新增任务
- `list`：列出任务
- `filter`：按 source、kind、parent、关键词、完成状态、优先级范围筛选
- `show`：查看单个任务的完整详情
- `update`：更新任务，支持追加备注
- `delete`：删除任务
- `sync`：把本地任务映射并同步到 Google Tasks

时间输入支持：

- RFC3339，例如 `2026-04-15T23:00:00+08:00`
- 简写格式 `YYYY-MM-DD HH`，例如 `2026-04-15 23`
- 简写格式 `YYYY-MM-DD HH:MM`
- 仅日期 `YYYY-MM-DD`
- 相对天数，例如 `--days 3`

本地字段：

- `title`
- `priority`
- `source`
- `start_at`
- `target_at`
- `updated_at`
- `meta` JSON
- `completed`
- `notes` JSON list

轻量结构化字段：

- `kind` 和 `parent_id` 不单独建表字段，统一存进 `meta`
- CLI 仍提供一等参数：`--kind`、`--parent`
- 这样可以先保持 schema 简单，同时让常见用法不需要手写完整 JSON

Google Tasks 映射：

- `title` -> Google Task `title`
- `completed` -> Google Task `status`
- `target_at` -> Google Task `due`
- 其他字段写入 Google Task `notes`，用 JSON 保存

默认数据目录：

- `~/.gtask/config.json`
- `~/.gtask/gtask.db`

默认同步到标题为 `My Tasks` 的任务列表。首次 `sync` 会自动解析并缓存该列表的 Google Task List ID。

并发友好性：

- SQLite 连接默认启用 `busy_timeout=5000ms`
- 多个进程同时写库时会先等锁释放，而不是立刻因瞬时锁冲突失败

示例：

```bash
go run ./cmd/gtask add --title "write docs" --priority 2 --source aistudio --kind text --days 3 --note "first note"
go run ./cmd/gtask add --title "run sync" --kind command --parent 4 --meta '{"cmd":"opencli sync","cwd":"/Users/zzwy/tmp/opencli-rs"}'
go run ./cmd/gtask add --title "night run" --target "2026-04-20 21"
go run ./cmd/gtask list
go run ./cmd/gtask filter --source idea1 --kind command
go run ./cmd/gtask show 4
go run ./cmd/gtask update 1 --kind command --parent 4 --completed true --note "done locally"
go run ./cmd/gtask delete 3
go run ./cmd/gtask sync
```

安装后可直接使用：

```bash
gtask --version
gtask list
```
