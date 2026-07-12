# oozie — Current State & Remaining Work

*Assessed 2026-07-12 by exploring the full codebase and smoke-testing every route against a fresh database.*

## TL;DR

The app **builds, runs, and every wired page renders without errors** (verified: `/projects`, `/projects/{id}`, agent page, store, installed apps, publishing jobs, publish form, settings, onboarding — all return 200; project creation and agent requests work end-to-end). What exists today is a **polished, fully functional UI shell with a real SQLite persistence layer, but simulated core behavior**: the "agent" returns a canned reply, "publishing" just inserts a `succeeded` row, and one whole domain (tasks) is coded but never wired up. Whether you can "begin using it" depends entirely on making the agent real.

---

## 1. What's Done and Working

- **Go + HTMX + SQLite stack** — `net/http` with Go 1.22 method routing, `html/template` rendering with a layout/pages/partials system, HTMX fragment endpoints for list refreshes.
- **Projects** — list/search/filter, create (with path defaulting), show, archive. Full repo → service → handler → template chain.
- **Agent session UI** — timeline, build/plan composer, request history, cancel, pending questions (answer/dismiss), permission requests (approve/deny), attachments list, feedback form. All persisted to SQLite.
- **Store** — browse, search, public/org/featured filters, app detail, install, installed-apps page.
- **Publishing** — draft form with validation (name/headline/description required), save draft, publish action, jobs list with status filter.
- **Settings** — general/appearance/agent sections, persisted per-user (hardcoded user 1).
- **Schema + seed data** — `migrations/001_init.sql` covers all 14 tables with sensible indexes, checks, and demo rows.
- **UI polish** — light/dark, toasts, dialogs, focus states, Cmd/Ctrl+Enter (per README; templates and `static/` back this up).

## 2. Core Gap: Everything Interesting Is Simulated

These are the items that stand between "demo" and "usable product":

### 2.1 The agent — ✅ DONE (2026-07-12): real pi integration
The agent is now the user's local **pi** instance via `pi --mode rpc` (`internal/agent/pi/`): one subprocess per project, running in the project's real directory with a persistent pi session (`--session-id`). The model picker mirrors `~/.pi/agent/settings.json` `enabledModels`; switching in the UI issues `set_model` on the live process. Assistant/tool messages, API errors, and settle status persist to SQLite; the timeline polls while streaming; pi's `select/confirm/input` dialogs land in oozie's question/permission panels and answers flow back over RPC; Cancel sends a real `abort`. Verified end-to-end (gpt-5.5 replied through the UI).

Remaining agent polish:
- [ ] `PlanApproval` handler (`handlers.go`) is still a no-op — either remove the route or wire it to send an approval follow-up message to pi.
- [ ] Live token streaming (SSE or finer-grained polling of `message_update` deltas) instead of whole-message updates.
- [ ] Surface pi session stats (`get_session_stats`: tokens, cost, context %) in the inspector panel.
- [ ] Reap idle pi processes / graceful shutdown hook (processes currently live until the server exits).
- [ ] DeepSeek provider currently fails with 402 Insufficient Balance (account funding, not code).

### 2.2 Publishing is fake
- [ ] `Publish` (`repo.go:212`) inserts a `succeeded` job immediately — no validation against the draft, no `store_apps` row created, no job lifecycle (`queued → running → succeeded/failed`). The handler even hardcodes the flash "Publish succeeded."
- [ ] Publishing a project should plausibly create/update its `store_apps` entry so your own apps appear in the store.

### 2.3 Attachments are read-only
- [ ] Seed row displays, but there is **no upload route/handler**, no file storage, and nothing consumes `storage_path`. Either build upload + serving or drop the UI affordance.

## 3. Tasks Domain: Coded but Orphaned

`internal/domain/tasks/` is a complete vertical (model, repo, service, handlers — 258 lines) with matching templates (`pages/tasks/`, `partials/tasks/`), **but**:
- [ ] No routes in `internal/app/routes.go` reference it (visiting `/tasks` silently falls through to the `GET /` catch-all and redirects to `/projects`).
- [ ] There is **no `tasks` table** in `001_init.sql` — wiring it up as-is would 500 at runtime.

Decide: wire it in (add routes + a `002_tasks.sql` migration) or delete the dead code.

## 4. Architecture Cleanup (works, but will bite as you grow)

- [ ] **The `projects` package is a god-package**: all store, publishing, settings, agent, and feedback logic lives in `internal/domain/projects/` (repo/service/handlers). The dedicated packages `internal/domain/{agents,publishing,settings,store}/` exist but each file is a 3-line package stub. Either move the code into them or delete the empty scaffolding.
- [ ] Empty scaffolding with only `.gitkeep`: `internal/platform/{clock,ids,log}`, `internal/web/{httperr,middleware}`, `internal/db/queries`, `templates/components`. Fill or remove.
- [ ] Error handling is thin: many `_ =` swallowed errors in handlers/services, bare `http.Error(w, "projects", 500)` responses, no request logging, no panic-recovery middleware, no graceful shutdown (`http.ListenAndServe` directly in `main.go`).
- [ ] Migration runner exists (`internal/db/migrations.go`) — confirm it tracks applied migrations so a future `002_*.sql` applies cleanly (seed data uses `INSERT OR IGNORE` today, which suggests re-run-everything semantics).

## 5. Project Hygiene (quick wins before you start using it)

- [ ] **Not a git repository.** `git init` + first commit before anything else.
- [ ] Add a `.gitignore` — the 17 MB compiled binary `./app`, `data/app.db`, and `.DS_Store` are sitting in the root and must not be committed.
- [ ] **Zero tests.** `go test ./...` reports "no test files" across all 10 packages; `test/fixtures/` and `test/integration/` are empty. At minimum: service-layer validation tests and one HTTP smoke test against an in-memory SQLite.
- [ ] Seed/demo data lives inside the schema migration — split into a separate optional seed file so a "clean" production DB is possible.
- [ ] `Makefile` is minimal (97 bytes) — add `run`, `test`, `build`, `lint` targets.
- [ ] Static file serving and template paths are relative to CWD (`"templates"`, `http.Dir("static")`) — the binary only works when launched from the repo root. Consider `embed.FS` for a truly portable single binary.

## 6. Suggested Order of Attack

| # | Milestone | Why first |
|---|-----------|-----------|
| 1 | `git init`, `.gitignore`, first commit | Everything after this is safely reversible |
| 2 | Decide tasks domain: wire (routes + migration) or delete | Cheapest open decision; removes a landmine |
| 3 | Real agent backend + async request lifecycle | This *is* the product; everything else is trim |
| 4 | Real publish flow (draft → job lifecycle → store_apps row) | Second core loop |
| 5 | Attachments upload or removal | Closes the last fake UI affordance |
| 6 | Tests for the above + error-handling/middleware pass | Lock in behavior before refactors |
| 7 | Domain package restructure + `embed.FS` packaging | Quality-of-life once behavior is real |

**Definition of "ready to use":** you can point a project at a real directory, send a message, watch a real agent respond (streaming or polled), approve its plan/permissions with those decisions actually mattering, and publish the result into the store — with the whole thing under version control and covered by at least smoke tests.
