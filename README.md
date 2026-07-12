# oozie

A server-rendered Go + HTMX + SQLite desktop-style workspace inspired by the scanned Glaze-like flows, rebuilt for solo local use.

## Features

- Project dashboard, create/open/archive flows, trusted display path flag
- Project-bound agent sessions powered by your local **pi** instance (`pi --mode rpc`): real build/plan requests run in the project's directory, streaming into the timeline, with pi's dialogs surfaced as oozie question/permission panels
- Model picker mirrors the models enabled in your terminal pi (`~/.pi/agent/settings.json` → `enabledModels`); switching models in the UI switches the live pi process
- Each project gets a persistent pi session (`--session-id`), so the agent keeps its context across app restarts; the project's Trusted flag maps to pi's `--approve`
- Store browsing, public/org/featured filters, install state, installed apps page
- Publish draft form and simulated publishing jobs
- Local settings for general, appearance, and agent shortcuts
- Dense macOS-inspired UI with light/dark support, toasts, panels, compact tables, dialogs/confirm prompts, accessible labels and focus states

## Stack

- Go `net/http`
- HTMX fragments for normal UI updates
- SQLite via `database/sql`
- `html/template`
- Minimal JavaScript for Cmd/Ctrl+Enter and toast polish

## Run

```bash
go run ./cmd/app
```

Then open <http://localhost:8080>.

Environment variables:

- `ADDR` (default `:8080`)
- `DATABASE_PATH` (default `data/app.db`)
- `MIGRATIONS_DIR` (default `migrations`)
- `PI_BIN` (default `pi`) — path to the pi binary used for agent sessions

## Checks

```bash
go test ./...
go build ./cmd/app
```
