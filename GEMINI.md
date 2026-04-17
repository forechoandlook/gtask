# gtask: Engineering Context & Guidelines

`gtask` is a high-performance, local-first CLI task manager written in Go. It synchronizes local SQLite-stored tasks with Google Tasks using a native REST implementation.

## Project Overview

- **Core Philosophy**: Local-first performance, minimalist output (agent-friendly), and zero-dependency distribution.
- **Key Technologies**: 
  - **Go**: 1.25+
  - **SQLite**: `modernc.org/sqlite` (Pure Go implementation, cross-platform without CGO).
  - **Synchronization**: Native REST API implementation for Google Tasks (no heavy SDK).
  - **Communication**: `net/rpc` for Client-Daemon interaction.
  - **Notifications**: `github.com/gen2brain/beeep` for cross-platform system alerts.
- **Architecture**:
  - `cmd/gtask`: CLI entry point.
  - `internal/app`: Command routing, flag parsing, and UI formatting.
  - `internal/daemon`: Background runner for notifications, task monitoring, and recurring task respawning.
  - `internal/store`: SQLite persistence layer with **WAL mode** enabled for concurrent access.
  - `internal/gws`: Custom OAuth2 and Google Tasks REST client.
  - `internal/syncer`: Bi-directional sync logic.

## Building and Running

- **Build**: `go build -ldflags="-s -w" ./cmd/gtask`
- **Build with Version**: `go build -ldflags="-X github.com/forechoandlook/gtask/internal/version.Version=0.2.3" ./cmd/gtask`
- **Run**: `./gtask [command]`
- **Sync**: `./gtask sync` (Triggers browser OAuth flow if credentials/token are missing).
- **Daemon**: `./gtask daemon` (Required for monitoring and recurrence features).
- **Test**: `go test ./...` (Includes SQLite logic and sync payload tests).
- **Upgrade**: `gtask upgrade` (Atomically replaces the binary via GitHub Releases).

## Development Conventions

### Versioning & Release
- **Tag-driven**: Versions are managed via Git tags (`v*`).
- **CI Injection**: Version strings (`Version`, `Commit`) and official Google OAuth secrets (`BuiltinClientID`, `BuiltinClientSecret`) are injected at build time using `-ldflags`.
- **Release Artifacts**: GitHub Actions builds binaries for `darwin/linux` (`amd64/arm64`) and generates a `VERSION` file.

### Data Model & Persistence
- **Metadata**: Use the `meta_json` field for non-standard task attributes (`kind`, `parent_id`, `recurrence`, `monitor_cmd`).
- **Timestamps**:
  - `updated_at`: Internal SQLite update tracker.
  - `completed_at` (in `meta`): The stable base for recurrence calculations. Always update this when a task is marked `done`.
- **Concurrency**: Always keep **WAL Mode** and `busy_timeout` (5000ms) active to support Daemon and CLI simultaneous access.

### Monitoring & Recurrence
- **Monitor Logic**: Task completion is determined by the **Exit Code (0)** of the `monitor_cmd`. Support pipes and shell redirections via `sh -c`.
- **Recurrence Logic**: Respawn tasks based on the `completed_at` timestamp + `recurrence` duration.

### Output Formatting
- **Minimalist**: Default output must be clean, devoid of ANSI colors, and use simple delimiters (spaces) to ensure readability for both humans and AI agents.
- **JSON Support**: All read/write commands must respect the global `--json` flag for structured data interchange.

## Security
- **Credential Protection**: Never hardcode OAuth secrets in plain text. Use `ldflags` injection in CI or local `~/.gtask/client_secret.json`.
- **File Permissions**: Configuration files (`token.json`, `credentials.json`) are stored with `0600` permissions.
