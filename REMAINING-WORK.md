# oozie — Status

*Updated 2026-07-14 after the "make it legit" overhaul. The two earlier assessments in this file and USER-EXPERIENCE-REPORT.md are historical; everything flagged there as fake, dead, or misleading has been fixed or removed.*

## Done — the product works end-to-end (verified live)

- **Agent**: real local pi per project, persistent sessions, model picker, cancel, question/permission panels, fail-closed approval gating for untrusted projects.
- **Publishing is real**: Publish runs `swift build -c release`, wraps the executable into `dist/<Name>.app`, and the job lifecycle (`queued → running → succeeded/failed`) is genuine, with build errors surfaced in the jobs list (which auto-refreshes while a build runs).
- **Store is real**: published apps appear in the store; Install copies the bundle to `~/Applications` (verified: the installed app launches and runs); Open launches it; Reinstall re-copies.
- **oozie is a Mac app**: `make app` produces `dist/oozie.app` (self-contained binary, embedded templates/CSS/JS/migrations, vendored HTMX, no network dependency); `make install-app` puts it in `~/Applications`. Data lives in `~/Library/Application Support/oozie/`.
- **Safety/hygiene**: binds to 127.0.0.1 only, graceful shutdown reaps pi subprocesses, panic-recovery + request-logging middleware, styled error pages, humanized timestamps, settings (theme + accent style) actually apply, no dead code or fake seed data, tests across build/service/HTTP layers.

## Polish round (2026-07-14, verified live)

- [x] **Live streaming**: pi `message_update` deltas persist as a throttled in-place "loading" assistant message; the timeline polls at 1s, so text appears as it's generated instead of all at once. Verified with a real agent run.
- [x] **Session stats**: after each run oozie queries pi's `get_session_stats` and shows tokens in/out, cost, and context % in the inspector (updates live during polling). Verified live: real token/cost figures rendered.
- [x] **App icons**: drop `icon.png` (or `icon.icns`) at the project root and publish — oozie converts it via sips/iconutil into the bundle's AppIcon. The agent prompt mentions this.
- [x] **Ad-hoc code signing**: published bundles are `codesign -s -` signed (signature verified in tests). Real Developer-ID signing/notarization only matters if apps are shared beyond this machine — out of scope for a personal workspace.
- [x] **Idle reaping**: pi processes idle for 30+ minutes are stopped automatically; sessions persist on disk so the next prompt resumes with full context.

Nothing is known to be missing or fake. Future ideas, not commitments: SSE instead of polling, per-message model attribution, publish-time version bumping.
