# gtask

`gtask` 是一个轻量级、高性能的 Go 语言编写的命令行任务管理工具。它采用本地优先的设计理念，使用 SQLite 存储任务，并支持原生同步到 Google Tasks。

## 🌟 核心特性

- **本地优先**：所有任务首先存储在本地 SQLite 数据库 (`~/.gtask/gtask.db`)，响应极快，支持离线操作。
- **Google Tasks 原生同步**：内置 OAuth2 授权，一键双向映射并同步任务到 Google Tasks，不再依赖外部工具。
- **常驻后台 (Daemon 模式)**：支持后台运行模式，提供跨平台系统通知（支持 macOS, Windows, Linux），处理周期性任务和监控任务。
- **监控与事件回调**：可以设置定期执行命令，当命令退出状态码为 0 时自动完成任务并通知。
- **周期性任务**：支持设置重复间隔，任务完成后会自动在指定时间后再次激活。
- **结构化元数据**：除了标准字段，还支持 `meta` (JSON) 和 `kind`/`parent` 等一等参数，满足复杂的任务管理需求。
- **灵活的时间输入**：支持 RFC3339、简写日期时间、仅日期以及相对天数（如 `--days 3`）。
- **操作日志**：`notes` 字段以 JSON 列表形式存储，自动记录任务的追加备注。

## 🛠️ 前置条件

无需额外工具。首次运行 `gtask sync` 时，程序会引导你通过浏览器完成 Google 账号授权（Token 将安全存储在 `~/.gtask/token.json`）。

## 🚀 安装

### 脚本安装 (推荐)

```bash
curl -fsSL https://raw.githubusercontent.com/forechoandlook/gtask/main/install.sh | bash
```

### 卸载

```bash
curl -fsSL https://raw.githubusercontent.com/forechoandlook/gtask/main/uninstall.sh | bash
```

## 📖 核心概念

### 数据模型

| 字段 | 说明 |
| :--- | :--- |
| `title` | 任务标题 |
| `priority` | 优先级 (整数，默认 0) |
| `source` | 来源标识 (如 `github`, `aistudio`) |
| `start_at` | 开始时间 |
| `target_at` | 截止/目标时间 (对应 Google Tasks 的 `due`) |
| `completed` | 完成状态 (true/false) |
| `meta` | 扩展元数据 (JSON)，包含 `kind` 和 `parent_id` |
| `notes` | 备注列表 (JSON Array)，每次 `update --note` 会追加一条记录 |

### 轻量级结构化支持

为了保持 Schema 简洁，`kind` 和 `parent_id` 统一存储在 `meta` 字段中。但在 CLI 中，我们提供了 `--kind` 和 `--parent` 作为一级参数，方便过滤和管理：
- `kind`: 任务类型，如 `text`, `command`, `bug` 等。
- `parent`: 父任务 ID，用于构建简单的任务层级。

## 💻 命令行用法

### 1. 任务管理

- **新增任务**:
  ```bash
  # 简易模式
  gtask add "撰写文档" "第一条备注"
  # 完整模式
  gtask add --title "修复 Bug" --priority 2 --source "github" --kind "bug" --days 2 --note "初步排查完成"
  ```
- **查看详情**:
  ```bash
  gtask show <id>
  ```
- **更新任务**:
  ```bash
  # 更新标题并标记完成
  gtask update 1 --title "新标题" --completed true
  # 追加备注 (不覆盖旧备注)
  gtask update 1 --note "这是追加的第二条备注"
  # 清除截止时间
  gtask update 1 --target null
  ```
- **删除任务**:
  ```bash
  gtask delete <id>
  ```

### 2. 列表与筛选

- **列出所有待办**:
  ```bash
  gtask list
  ```
- **包含已完成任务**:
  ```bash
  gtask list --all
  ```
- **高级筛选 (filter)**:
  ```bash
  # 按来源和类型筛选
  gtask filter --source "idea" --kind "feature"
  # 关键词搜索 (标题、元数据、备注)
  gtask filter --query "搜索词"
  # 按优先级范围筛选
  gtask filter --priority-min 1 --priority-max 5
  ```

### 3. 同步 (Sync)

```bash
gtask sync
```
- 默认同步到 Google Tasks 中标题为 `My Tasks` 的列表。
- 首次同步会引导 OAuth2 授权，并解析/缓存 `DefaultGoogleListID` 到配置文件。
- 映射关系：`title` -> `title`, `completed` -> `status`, `target_at` -> `due`, 其余字段存入 Google Task 的 `notes` (JSON 格式)。

### 4. 监控与周期任务 (需运行 Daemon)

- **添加监控任务**:
  ```bash
  # 每 1 分钟检查一次服务器状态，如果 curl 成功且 grep 匹配到 "OK" (退出码为 0) 则完成任务
  gtask add "检查服务器" --monitor-cmd "curl -sf http://example.com | grep OK" --monitor-interval 1m
  ```
- **添加周期性任务**:
  ```bash
  # 任务完成后，每隔 24 小时自动重新激活
  gtask add "每日备份" --recurrence 24h
  ```

### 5. 常驻模式 (Daemon)

Daemon 模式提供 RPC 服务，Client 模式会优先尝试连接 Daemon，连接失败则降级为直连本地数据库。

- **启动 Daemon**:
  ```bash
  gtask daemon --host 127.0.0.1 --port 8765 &
  ```
- **系统通知**: Daemon 使用 `beeep` 库实现跨平台通知。当任务即将开始、截止时间临近、监控条件达成或周期任务重发时，系统会弹出通知。
- **环境变量配置**:
  可以通过 `GTASK_HOST` 和 `GTASK_PORT` 统一配置 Client 和 Daemon 的连接参数。

## ⚙️ 配置文件

默认位于 `~/.gtask/config.json`：

```json
{
  "db_path": "/Users/yourname/.gtask/gtask.db",
  "default_google_list_title": "My Tasks",
  "default_google_list_id": "MTIzNDU2Nzg5..."
}
```

## 📅 时间格式支持

- **标准格式**: `2026-04-15T23:00:00+08:00` (RFC3339)
- **简短格式**: `2026-04-15 23:00` 或 `2026-04-15` (默认本地时区)
- **相对格式**: `--days 3` 表示 3 天后的当前时间，`--start-days 1` 表示明天。

## 🛠️ 并发与可靠性

- **SQLite WAL 模式**: 支持高并发读取。
- **Busy Timeout**: 默认 5 秒繁忙超时，多进程同时写入时会自动排队，不会轻易报错。

## 📄 License

MIT
