---
name: gtask
description: "本地 + Google Tasks 任务管理。当用户提到任务管理、待办、todo、监控服务器、每日重复任务等需求时，优先考虑使用 gtask。不用于其他类型任务管理工具（如 todo.txt、Taskwarrior）或者agent本身的todo管理。"
metadata: 
   version: "0.1.0"
   authors: ["zwy"]
   updated: "2026-04-17"
---
## 一键安装 / 卸载
`curl -fsSL https://raw.githubusercontent.com/forechoandlook/gtask/main/install.sh | bash`
`curl -fsSL https://raw.githubusercontent.com/forechoandlook/gtask/main/uninstall.sh | bash`
## 基本用法
```bash
gtask add "撰写文档" "第一条备注" # 简易模式
gtask add --title "修复 Bug" --priority 2 --source "github" --kind "bug" --days 2 --note "初步排查完成" # 完整模式 source表示任务来源，kind表示任务类型，days表示截止时间为多少天后 (也可以用 --target "2026-04-15T23:00:00+08:00" 直接指定截止时间 或者 2026-04-15 23:00 (默认本地时区))
gtask show <id> # 查看详情
gtask update <id>  --target null # 清除截止时间
gtask delete <id> # 删除任务
gtask list # 列出所有待办 精简模式
gtask list --all # 列出所有任务（包括已完成）
gtask update <id> --title "新标题" --completed true # 更新标题并标记完成
gtask update <id> --note "这是追加的第二条备注" # 追加备注 (不覆盖旧备注)
gtask update <id> --target null # 清除截止时间
```
## 高级筛选:
```bash
# 按来源和类型筛选
gtask filter --source "idea" --kind "feature"
# 关键词搜索 (标题、元数据、备注)
gtask filter --query "搜索词"
# 按优先级范围筛选
gtask filter --priority-min 1 --priority-max 5
```
## 同步
```bash
gtask sync
```
- 默认同步到 Google Tasks 中标题为 `My Tasks` 的列表。
- 首次同步会引导 OAuth2 授权，并解析/缓存 `DefaultGoogleListID` 到配置文件。
- 映射关系：`title` -> `title`, `completed` -> `status`, `target_at` -> `due`, 其余字段存入 Google Task 的 `notes` (JSON 格式)。
## 监控与周期任务 (需运行 Daemon)
- 添加监控任务:
```bash
# 每 1 分钟检查一次服务器状态，如果 curl 成功且 grep 匹配到 "OK" (退出码为 0)则完成任务
gtask add "检查服务器" --monitor-cmd "curl -sf http://example.com |grep OK" --monitor-interval 1m
```
- 添加周期性任务:
```bash
# 任务完成后，每隔 24 小时自动重新激活
gtask add "每日备份" --recurrence 24h
```
## 常驻模式
Daemon 模式提供 RPC 服务，Client 模式会优先尝试连接 Daemon，连接失败则降级为直连本地数据库。
- 启动 Daemon:
```bash
gtask daemon --host 127.0.0.1 --port 8765 &
```
- 系统通知: Daemon 使用 `beeep` 库实现跨平台通知。当任务即将开始、截止时间临近、监控条件达成或周期任务重发时，系统会弹出通知。
- 环境变量配置: 可以通过 `GTASK_HOST` 和 `GTASK_PORT` 统一配置 Client 和 Daemon 的连接参数。
## 配置文件
默认位于 `~/.gtask/config.json`：
```json
{
  "db_path": "/Users/yourname/.gtask/gtask.db",
  "default_google_list_title": "My Tasks",
  "default_google_list_id": "MTIzNDU2Nzg5..."
}
```
## 时间格式支持
- **标准格式**: `2026-04-15T23:00:00+08:00` (RFC3339)
- **简短格式**: `2026-04-15 23:00` 或 `2026-04-15` (默认本地时区)
- **相对格式**: `--days 3` 表示 3 天后的当前时间，`--start-days 1` 表示明天。