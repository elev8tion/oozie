# oozie

A Mac app for building mini Mac apps. oozie runs entirely on your machine: you describe an app, your local **pi** agent builds it in a real project directory, and oozie's publish pipeline compiles it into a `.app` bundle you can install into `~/Applications` and launch from your Dock.

## The loop

1. **Create a project** — pick a name and directory; mark it Trusted (agent works freely) or untrusted (every write/edit/bash call needs your approval in the permission panel).
2. **Talk to the agent** — Plan mode for a proposal, Build mode to implement. The agent runs your local `pi` instance (`pi --mode rpc`) in the project directory with a persistent session, so context survives restarts. The model picker mirrors your terminal pi's `enabledModels`.
3. **Publish** — save a draft (name, headline, description), hit Publish. A real job runs `swift build -c release` and wraps the executable in `dist/<Name>.app` (ad-hoc signed; an `icon.png`/`icon.icns` at the project root becomes the app icon); on success the app appears in your Store.
4. **Install & open** — Install copies the bundle to `~/Applications`; Open launches it.

## Features

- Project dashboard: create/open/archive, search and filter
- Agent sessions with live-streaming responses, plan/build modes, model switching, cancel, question & permission panels; untrusted projects are gated by a fail-closed approval extension; token/cost/context stats in the inspector; idle agent processes are reaped after 30 minutes (sessions persist)
- Real publishing: `queued → running → succeeded/failed` jobs with error output, auto-refreshing job list
- Store of your published apps with install/open/reinstall
- Settings: theme (system/light/dark) and accent style, applied instantly and persisted
- Styled error pages, request logging, panic recovery, graceful shutdown (pi subprocesses are reaped on quit)

## Run

```bash
make run          # dev server on http://127.0.0.1:8080
make app          # build dist/oozie.app (double-clickable)
make install-app  # build and install oozie.app into ~/Applications
make test         # go test ./...
```

The binary is fully self-contained (templates, CSS, JS, and migrations are embedded). Data lives in `~/Library/Application Support/oozie/app.db`; an existing repo-local `data/app.db` is migrated there automatically on first run.

## Requirements

- macOS with the Swift toolchain (Xcode or Command Line Tools) — used to compile published apps
- [pi](https://github.com/) installed and configured (`~/.pi/agent/settings.json`) — the coding agent
- Go 1.24+ to build oozie itself

## Environment variables

- `ADDR` (default `127.0.0.1:8080`)
- `DATABASE_PATH` (default `~/Library/Application Support/oozie/app.db`)
- `PI_BIN` (default `pi`) — path to the pi binary
- `OOZIE_OPEN_BROWSER=1` — open the UI in the default browser on start (the .app bundle sets this)

## Stack

Go `net/http` · HTMX · SQLite (`modernc.org/sqlite`) · `html/template` · embedded assets via `embed.FS`
