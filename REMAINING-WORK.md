# oozie — Status

*Updated 2026-07-14 after the "make it legit" overhaul. The two earlier assessments in this file and USER-EXPERIENCE-REPORT.md are historical; everything flagged there as fake, dead, or misleading has been fixed or removed.*

## Done — the product works end-to-end (verified live)

- **Agent**: real local pi per project, persistent sessions, model picker, cancel, question/permission panels, fail-closed approval gating for untrusted projects.
- **Publishing is real**: Publish runs `swift build -c release`, wraps the executable into `dist/<Name>.app`, and the job lifecycle (`queued → running → succeeded/failed`) is genuine, with build errors surfaced in the jobs list (which auto-refreshes while a build runs).
- **Store is real**: published apps appear in the store; Install copies the bundle to `~/Applications` (verified: the installed app launches and runs); Open launches it; Reinstall re-copies.
- **oozie is a Mac app**: `make app` produces `dist/oozie.app` (self-contained binary, embedded templates/CSS/JS/migrations, vendored HTMX, no network dependency); `make install-app` puts it in `~/Applications`. Data lives in `~/Library/Application Support/oozie/`.
- **Safety/hygiene**: binds to 127.0.0.1 only, graceful shutdown reaps pi subprocesses, panic-recovery + request-logging middleware, styled error pages, humanized timestamps, settings (theme + accent style) actually apply, no dead code or fake seed data, tests across build/service/HTTP layers.

## Remaining (optional polish, nothing blocking)

- [ ] Live token streaming in the agent timeline (currently 1.5s polling of whole messages).
- [ ] Surface pi session stats (tokens/cost/context %) in the inspector panel.
- [ ] App icons for published bundles (currently default macOS icon).
- [ ] Code-signing/notarization story if apps are ever shared beyond this machine.
- [ ] Reap idle pi processes during long sessions (they currently live until oozie quits).
