---
name: gtask
description: "本地 + Google Tasks 任务管理。仅在用户要读/改真实待办时使用；不要把它当成 agent 自己的临时计划板。"
metadata:
  version: "0.2.0"
  authors: ["zwy"]
  updated: "2026-04-17"
---
## 作用边界
在这些场景优先使用 `gtask`:
- 用户明确要查看、整理、补充、修正自己的任务/待办
- 用户要安排今晚/今天/本周优先级
- 用户要创建周期任务、监控任务、同步 Google Tasks
- 用户已经在用 `gtask`，希望继续沿用现有任务库
不要使用 `gtask`:
- 只是让 agent 临时列一个执行计划
- 只是头脑风暴想法，还没决定是否进入真实待办
- 用户明确要用其他任务系统，如 `todo.txt`、Taskwarrior、Notion、GitHub Issues
## 开始前
默认先确认 `gtask` 是否可用，再决定后续动作。
推荐最小探测顺序：
```bash
which gtask
gtask todo
```
如果用户要审阅或重排任务，不要一开始就批量写入。先读，再判断，再修正。
## 默认工作流
### 1. 审阅任务
适用于“审阅我所有 task”“给今晚优先级”“看看有哪些问题”这类请求。
常用命令：
```bash
gtask todo
gtask done
gtask show [--csv] <id1> [id2...]
gtask filter --priority-min 0 --priority-max 3
```
执行原则：
- 先用 `gtask todo` 拿当前待办视图
- 注意 CSV 中的 `audit` 列：如果显示 `MISSING_SKN`，说明该任务缺少 Source/Kind/Note，需要补齐
- 重点检查 `priority/src/kind/parent/target_time/note`
- 识别重复任务、过期未完成任务、无来源任务、无类型任务、范围过大的任务
- 只有在已经形成明确判断后，才用 `update` 写回修正
审阅时的标准输出应尽量包含：
- 今晚或本周优先级排序
- 重复/冲突任务
- 缺失字段
- 建议拆分或降级的任务
- 已经写回的修正项和未写回的建议项
### 2. 新建任务
能补字段时，不要只写标题。
优先使用：
```bash
gtask add "标题" --priority 1 --source "xxx" --kind "feature" --days 1 --note "为什么要做"
```
如果用户只给了一个模糊标题，至少补：
- `source`
- `kind`
- 一条说明任务边界的 `note`
如果是大任务，优先建立父子关系：
```bash
gtask add "总任务" --kind epic
gtask add "今晚切片" --kind feature --parent <id>
```
### 3. 修正任务
常用命令：
```bash
gtask show [--csv] <id1> [id2...]
gtask update [flags] <id1> [id2...]
```
修正时优先做这些事情：
- 给缺失的 `source/kind` 补齐
- 支持批量更新：如果多个任务属于同一类，使用 `gtask update --kind xxx id1,id2`
- 给重复任务建立父子关系，或降级其中一个为总任务
- 把已过期但仍是 `todo` 的 `target_at` 重排
- 给范围过大的任务追加 note，收窄成可执行切片
- 清理明显错误的元数据
### 4. 周期 / 监控任务
仅在用户明确要“定时检查”“条件满足自动完成”“每日/每周重复”时使用。
```bash
gtask add "检查服务状态" --monitor-cmd "curl -sf http://example.com | grep OK" --monitor-interval 1m
gtask add "每日备份" --recurrence 24h
```
不要把普通任务误写成监控任务。`monitor_*` 字段应只出现在真正的监控场景。
### 5. 同步 Google Tasks
```bash
gtask sync
```
同步前后关注：
- 首次授权是否完成
- 默认列表是否正确
- 本地任务是否已经补好关键字段
- 失败时先看错误文本，不要盲目重试
## 已知限制和坑
这些点应当默认记住，避免重复试探：
- `gtask --help` 而不是 `gtask add --help`，后者会因为参数过多而输出混乱
- `gtask list --all` 适合总览，不适合直接做精细审阅；需要配合 `show`
- 任务库可能存在脏数据：缺失 `source/kind`、错误 `meta`、重复任务、过期未重排的 `target_at`
- 某些任务虽然有时间估计，但不一定能直接代表今晚优先级；要结合依赖、范围和可落地性重新判断
## 写入规范
写回 `gtask` 时，默认遵循这些规则：
- 新任务尽量补齐 `source` 和 `kind`
- 大任务优先拆成 `epic` + 子任务
- 模糊任务必须追加 `note` 说明边界、验收或今晚切片
- 过期的 `todo` 任务要么重排 `target_at`，要么清空时间，不要维持失真状态
- 不要给无关任务写入 `monitor_interval`、`recurrence` 等特殊字段
- 不要为了“看起来整齐”批量改动所有任务；只改与当前请求直接相关的项
## 常用命令
```bash
gtask todo
gtask done
gtask filter --source "idea1" --kind "feature"
gtask show 1 2 3
gtask add "标题" "第一条备注"
gtask update --note "追加说明" 1,2
gtask update --completed true 1
gtask delete <id1> [id2...]
gtask sync
```
## 时间格式
支持这些格式：
- `2026-04-17T23:00:00+08:00`
- `2026-04-17 23:00`
- `2026-04-17`
- `--days 3`
- `--start-days 1`
默认使用本地时区解释简短格式。
## 配置和路径
- 默认数据目录：`~/.gtask`
- 默认配置文件：`~/.gtask/config.json`
- 默认数据库：`~/.gtask/gtask.db`
- 可执行文件常见位置：`~/.local/bin/gtask`
## 一键安装 / 卸载
```bash
curl -fsSL https://raw.githubusercontent.com/forechoandlook/gtask/main/install.sh | bash
curl -fsSL https://raw.githubusercontent.com/forechoandlook/gtask/main/uninstall.sh | bash
```