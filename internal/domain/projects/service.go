package projects

import (
	"context"
	"crypto/rand"
	"database/sql"
	"embed"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"oozie/internal/agent/pi"
	"oozie/internal/build"
)

// seedsFS is materialized into every project workdir: DESIGN.md (the
// visual/UX standard) and Tools/ (the visual-review helpers). Files are
// only written if missing, so per-project edits stick.
//
//go:embed all:seeds
var seedsFS embed.FS

type Service struct {
	repo    *Repo
	agent   *pi.Manager
	catalog pi.Catalog
	builder build.AppBuilder
	jobs    sync.WaitGroup
}

func NewService(repo *Repo) *Service {
	return &Service{repo: repo, builder: build.SwiftBuilder{}}
}

// SetBuilder swaps the app builder (tests use a fake).
func (s *Service) SetBuilder(b build.AppBuilder) { s.builder = b }

// WaitForJobs blocks until all in-flight publishing jobs settle.
func (s *Service) WaitForJobs() { s.jobs.Wait() }

// SetAgent wires the pi RPC manager after construction (the manager's
// event sink is this service, so the two reference each other).
func (s *Service) SetAgent(agent *pi.Manager, catalog pi.Catalog) {
	s.agent = agent
	s.catalog = catalog
}

func (s *Service) Dashboard(ctx context.Context) (Dashboard, error) {
	ps, err := s.repo.ListProjects(ctx, "", "active")
	if err != nil {
		return Dashboard{}, err
	}
	apps, _ := s.repo.ListStoreApps(ctx, "", "featured")
	jobs, _ := s.repo.ListJobs(ctx, "")
	return Dashboard{Projects: ps, StoreApps: apps, Jobs: jobs}, nil
}
func (s *Service) ListProjects(ctx context.Context, q, filter string) ([]Project, error) {
	return s.repo.ListProjects(ctx, strings.TrimSpace(q), filter)
}
func (s *Service) GetProject(ctx context.Context, id int64) (Project, error) {
	return s.repo.GetProject(ctx, id)
}
func (s *Service) CreateProject(ctx context.Context, name, path string, trusted bool) (Project, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Project{}, ErrValidation{"Project name is required."}
	}
	if path == "" {
		path = "~/Projects/" + strings.ToLower(strings.ReplaceAll(name, " ", "-"))
	}
	return s.repo.CreateProject(ctx, name, path, trusted)
}
func (s *Service) ArchiveProject(ctx context.Context, id int64) error {
	return s.repo.ArchiveProject(ctx, id)
}
func (s *Service) AgentPage(ctx context.Context, projectID int64) (AgentPage, error) {
	p, err := s.repo.GetProject(ctx, projectID)
	if err != nil {
		return AgentPage{}, err
	}
	session, err := s.repo.GetSession(ctx, projectID)
	if err != nil {
		return AgentPage{}, err
	}
	msgs, _ := s.repo.ListMessages(ctx, session.ID)
	reqs, _ := s.repo.ListRequests(ctx, session.ID)
	q, _ := s.repo.PendingQuestion(ctx, projectID)
	perm, _ := s.repo.PendingPermission(ctx, projectID)
	model := session.Model
	if model == "" {
		model = s.catalog.DefaultModel
	}
	streaming := len(reqs) > 0 && (reqs[0].Status == "streaming" || reqs[0].Status == "waiting")
	var stats *pi.SessionStats
	if s.agent != nil {
		stats = s.agent.Stats(projectID)
	}
	return AgentPage{Project: p, Session: session, Requests: reqs, Messages: msgs, Question: q, Permission: perm, Mode: "build", Models: s.models(), Model: model, Streaming: streaming, Stats: stats}, nil
}

func (s *Service) models() []ModelOption {
	out := make([]ModelOption, 0, len(s.catalog.Models))
	for _, m := range s.catalog.Models {
		out = append(out, ModelOption{Provider: m.Provider, ID: m.ID, Full: m.Full})
	}
	return out
}

func (s *Service) SendAgentMessage(ctx context.Context, projectID int64, mode, message string) error {
	message = strings.TrimSpace(message)
	if message == "" {
		return ErrValidation{"Message is required."}
	}
	if s.agent == nil {
		return ErrValidation{"The pi agent is not configured."}
	}
	project, err := s.repo.GetProject(ctx, projectID)
	if err != nil {
		return err
	}
	session, err := s.repo.GetSession(ctx, projectID)
	if err != nil {
		return err
	}
	if mode != "plan" {
		mode = "build"
	}
	model := session.Model
	if model == "" {
		model = s.catalog.DefaultModel
	}
	if session.PiSessionID == "" {
		session.PiSessionID = newPiSessionID(projectID)
		if err := s.repo.SetPiSessionID(ctx, session.ID, session.PiSessionID); err != nil {
			return err
		}
	}
	workdir, err := projectWorkdir(project)
	if err != nil {
		return ErrValidation{"Project directory unavailable: " + err.Error()}
	}
	requestID, err := s.repo.CreateAgentRequest(ctx, session.ID, mode, message)
	if err != nil {
		return err
	}
	opts := pi.StartOptions{
		ProjectID:    projectID,
		Workdir:      workdir,
		Model:        model,
		PiSessionID:  session.PiSessionID,
		SystemPrompt: oozieSystemPrompt(project, workdir),
		Trusted:      project.Trusted,
	}
	if err := s.agent.Prompt(opts, requestID, wrapModeMessage(mode, message)); err != nil {
		_ = s.repo.InsertMessage(ctx, requestID, "system", "error", "Failed to start pi: "+err.Error())
		_ = s.repo.CompleteRequest(ctx, requestID, "failed")
		return ErrValidation{"Could not reach the pi agent: " + err.Error()}
	}
	return nil
}

func (s *Service) SelectModel(ctx context.Context, projectID int64, model string) error {
	found := false
	for _, m := range s.catalog.Models {
		if m.Full == model {
			found = true
			break
		}
	}
	if !found {
		return ErrValidation{"Unknown model: " + model}
	}
	session, err := s.repo.GetSession(ctx, projectID)
	if err != nil {
		return err
	}
	if err := s.repo.SetSessionModel(ctx, session.ID, model); err != nil {
		return err
	}
	if s.agent != nil {
		if err := s.agent.SetModel(projectID, model); err != nil {
			return ErrValidation{"Saved, but the running agent rejected the model: " + err.Error()}
		}
	}
	return nil
}

func (s *Service) CancelRequest(ctx context.Context, projectID, id int64) error {
	if s.agent != nil {
		_ = s.agent.Abort(projectID)
	}
	return s.repo.CancelRequest(ctx, id)
}
func (s *Service) AnswerQuestion(ctx context.Context, id int64, answer string) error {
	q, err := s.repo.GetQuestion(ctx, id)
	if err != nil {
		return err
	}
	if q.RPCID != "" && s.agent != nil {
		if err := s.agent.RespondValue(q.ProjectID, q.RPCID, answer); err != nil {
			return ErrValidation{err.Error()}
		}
	}
	return s.repo.ResolveQuestion(ctx, id, "answered")
}
func (s *Service) DismissQuestion(ctx context.Context, id int64) error {
	q, err := s.repo.GetQuestion(ctx, id)
	if err != nil {
		return err
	}
	if q.RPCID != "" && s.agent != nil {
		_ = s.agent.RespondCancel(q.ProjectID, q.RPCID)
	}
	return s.repo.ResolveQuestion(ctx, id, "dismissed")
}
func (s *Service) ResolvePermission(ctx context.Context, id int64, approved bool) error {
	p, err := s.repo.GetPermission(ctx, id)
	if err != nil {
		return err
	}
	if p.RPCID != "" && s.agent != nil {
		if err := s.agent.RespondConfirm(p.ProjectID, p.RPCID, approved); err != nil {
			return ErrValidation{err.Error()}
		}
	}
	if approved {
		return s.repo.ResolvePermission(ctx, id, "approved")
	}
	return s.repo.ResolvePermission(ctx, id, "denied")
}

// --- pi.Sink implementation: events from the agent process land here. ---

func (s *Service) AssistantMessage(projectID, requestID int64, content string) {
	if err := s.repo.FinalizeAssistant(context.Background(), requestID, content); err != nil {
		log.Printf("persist assistant message (project %d): %v", projectID, err)
	}
}
func (s *Service) AssistantPartial(projectID, requestID int64, content string) {
	if err := s.repo.UpsertPartialAssistant(context.Background(), requestID, content); err != nil {
		log.Printf("persist streaming text (project %d): %v", projectID, err)
	}
}
func (s *Service) ToolStarted(projectID, requestID int64, callID, content string) {
	if err := s.repo.InsertToolStart(context.Background(), requestID, callID, content); err != nil {
		log.Printf("persist tool start (project %d): %v", projectID, err)
	}
}
func (s *Service) ToolFinished(projectID, requestID int64, callID, content, body string) {
	if err := s.repo.FinalizeTool(context.Background(), requestID, callID, content, body); err != nil {
		log.Printf("persist tool result (project %d): %v", projectID, err)
	}
}
func (s *Service) RequestSettled(projectID, requestID int64, status string) {
	if err := s.repo.CompleteRequest(context.Background(), requestID, status); err != nil {
		log.Printf("settle request %d (project %d): %v", requestID, projectID, err)
	}
}
func (s *Service) Question(projectID, requestID int64, rpcID, prompt, optionsJSON string) {
	if err := s.repo.InsertQuestion(context.Background(), projectID, requestID, rpcID, prompt, optionsJSON); err != nil {
		log.Printf("persist question (project %d): %v", projectID, err)
	}
}
func (s *Service) Permission(projectID, requestID int64, rpcID, name, reason string) {
	if err := s.repo.InsertPermission(context.Background(), projectID, requestID, rpcID, name, reason); err != nil {
		log.Printf("persist permission (project %d): %v", projectID, err)
	}
}
func (s *Service) AgentError(projectID, requestID int64, message string) {
	if err := s.repo.InsertMessage(context.Background(), requestID, "system", "error", message); err != nil {
		log.Printf("persist agent error (project %d): %v", projectID, err)
	}
}

func projectWorkdir(p Project) (string, error) {
	path := strings.TrimSpace(p.ProjectPathDisplay)
	if path == "" {
		path = "~/Projects/" + strings.ToLower(strings.ReplaceAll(p.Name, " ", "-"))
	}
	if path == "~" || strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		path = filepath.Join(home, strings.TrimPrefix(path, "~"))
	}
	if err := os.MkdirAll(path, 0o755); err != nil {
		return "", err
	}
	seedProject(path)
	return path, nil
}

// seedProject writes DESIGN.md and Tools/ into the workdir, skipping any
// file that already exists so per-project edits stick.
func seedProject(workdir string) {
	_ = fs.WalkDir(seedsFS, "seeds", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel("seeds", p)
		dest := filepath.Join(workdir, rel)
		if _, err := os.Stat(dest); err == nil {
			return nil
		}
		body, err := seedsFS.ReadFile(p)
		if err != nil {
			return nil
		}
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return nil
		}
		mode := os.FileMode(0o644)
		if strings.HasSuffix(dest, ".sh") {
			mode = 0o755
		}
		_ = os.WriteFile(dest, body, mode)
		return nil
	})
}

func newPiSessionID(projectID int64) string {
	buf := make([]byte, 8)
	_, _ = rand.Read(buf)
	return fmt.Sprintf("oozie-p%d-%s", projectID, hex.EncodeToString(buf))
}

func oozieSystemPrompt(p Project, workdir string) string {
	return fmt.Sprintf(`You are running inside oozie, a local macOS workspace whose purpose is building small personal Mac apps, as the agent for the project %q (working directory: %s).

How to behave in oozie:
- Requests arrive in one of two modes, stated at the top of each message.
- PLAN mode: produce a concise, numbered implementation plan. Do not create, modify, or delete any files. End by asking whether to proceed.
- BUILD mode: implement the request directly in the working directory, verifying your work as you go (build/tests where applicable).
- Your responses are rendered in a compact web timeline; keep them focused and skip decorative preamble.
- The user approves questions and permission dialogs through the oozie side panel; when you ask via a dialog, wait for that response.

Producing Mac apps (oozie's publish pipeline):
- When the user asks for an app, scaffold a Swift package at the project root: a Package.swift with a single executable target, macOS platform .macOS(.v13) or later, sources under Sources/.
- For GUI apps, use SwiftUI with an explicit AppDelegate-free entry (@main struct conforming to App) and call NSApplication.shared.setActivationPolicy(.regular) plus NSApp.activate(ignoringOtherApps: true) at launch so the window appears when run from a bundle.
- Verify with 'swift build' before declaring the work done.
- Place a 1024x1024 icon.png (or icon.icns) at the project root; oozie converts it into the app icon when publishing.

Design quality (non-negotiable):
- The project root contains DESIGN.md — read it before any UI work and follow it. It is the visual/UX standard for every app built here: native macOS feel, HIG-aligned layout on an 8pt grid, semantic system colors with full dark-mode support, system text styles, SF Symbols (never emoji as icons), designed empty/loading/error states, keyboard shortcuts, confirmation for destructive actions, and accessibility labels.
- "It compiles" is not done. Done means: build passes AND every screen looks intentional in light and dark mode with no default-looking, cramped, or misaligned UI.

Visual review (run after any build that changes UI):
1. Run: sh Tools/visual-review.sh <ExecutableName> — it builds, launches the app, screenshots its window to review.png, and quits it.
2. Read review.png with your read tool and critique the actual pixels against DESIGN.md: spacing, alignment, hierarchy, empty states, anything default-looking.
3. Fix what fails, rebuild, and re-run the review. Ship only when the screenshot would pass DESIGN.md.
If the screenshot comes back black/empty, the user needs to grant Screen Recording permission (System Settings → Privacy & Security) — tell them, and continue without the review rather than looping.
- oozie's Publish action runs 'swift build -c release' and wraps the executable in a .app bundle, so the package MUST build cleanly from the project root with no extra steps. If you instead produce an .xcodeproj, place the final built .app in the project root or dist/.`, p.Name, workdir)
}

func wrapModeMessage(mode, message string) string {
	if mode == "plan" {
		return "[oozie mode: PLAN — plan only, do not modify files]\n\n" + message
	}
	return "[oozie mode: BUILD — implement directly]\n\n" + message
}
func (s *Service) SaveFeedback(ctx context.Context, projectID int64, typ, reason, extra string) error {
	if strings.TrimSpace(typ) == "" {
		return ErrValidation{"Feedback type is required."}
	}
	return s.repo.SaveFeedback(ctx, projectID, typ, reason, extra)
}
func (s *Service) GetDraft(ctx context.Context, projectID int64) (PublishDraft, error) {
	d, err := s.repo.GetDraft(ctx, projectID)
	if err != nil {
		p, _ := s.repo.GetProject(ctx, projectID)
		return PublishDraft{ProjectID: projectID, AppName: p.Name, Headline: "A focused project workspace", PublishTarget: "public", Visibility: "unlisted", ScreenshotManifest: "[]"}, nil
	}
	return d, err
}
func (s *Service) SaveDraft(ctx context.Context, d PublishDraft) error {
	if strings.TrimSpace(d.AppName) == "" || strings.TrimSpace(d.Headline) == "" || strings.TrimSpace(d.Description) == "" {
		return ErrValidation{"App name, headline, and description are required."}
	}
	if d.PublishTarget == "" {
		d.PublishTarget = "public"
	}
	if d.Visibility == "" {
		d.Visibility = "unlisted"
	}
	return s.repo.SaveDraft(ctx, d)
}

// Publish builds the project into a .app bundle asynchronously: a job is
// queued immediately, a background worker runs the build, and on success
// the app appears in (or updates) the store with its artifact path.
func (s *Service) Publish(ctx context.Context, projectID int64) error {
	draft, err := s.repo.GetDraft(ctx, projectID)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrValidation{"Save a publish draft (name, headline, description) before publishing."}
	}
	if err != nil {
		return err
	}
	project, err := s.repo.GetProject(ctx, projectID)
	if err != nil {
		return err
	}
	workdir, err := projectWorkdir(project)
	if err != nil {
		return ErrValidation{"Project directory unavailable: " + err.Error()}
	}
	jobID, err := s.repo.CreateJob(ctx, projectID)
	if err != nil {
		return err
	}
	s.jobs.Add(1)
	go s.runPublishJob(jobID, projectID, draft, workdir)
	return nil
}

func (s *Service) runPublishJob(jobID, projectID int64, draft PublishDraft, workdir string) {
	defer s.jobs.Done()
	ctx := context.Background()
	if err := s.repo.SetJobRunning(ctx, jobID); err != nil {
		log.Printf("publish job %d: %v", jobID, err)
	}
	appPath, err := s.builder.Build(workdir, draft.AppName)
	if err != nil {
		_ = s.repo.FinishJob(ctx, jobID, "failed", err.Error(), nil)
		return
	}
	appID, err := s.repo.UpsertStoreApp(ctx, projectID, draft, appPath)
	if err != nil {
		_ = s.repo.FinishJob(ctx, jobID, "failed", "app built but store update failed: "+err.Error(), nil)
		return
	}
	_ = s.repo.FinishJob(ctx, jobID, "succeeded", "", &appID)
}

func (s *Service) ListJobs(ctx context.Context, status string) ([]PublishingJob, error) {
	return s.repo.ListJobs(ctx, status)
}
func (s *Service) ListStoreApps(ctx context.Context, q, filter string) ([]StoreApp, error) {
	return s.repo.ListStoreApps(ctx, strings.TrimSpace(q), filter)
}
func (s *Service) GetStoreApp(ctx context.Context, id int64) (StoreApp, error) {
	return s.repo.GetStoreApp(ctx, id)
}

// InstallApp copies the built .app bundle into ~/Applications.
func (s *Service) InstallApp(ctx context.Context, id int64) error {
	app, err := s.repo.GetStoreApp(ctx, id)
	if err != nil {
		return err
	}
	if app.ArtifactPath == "" {
		return ErrValidation{"This app has no built artifact. Publish its project first."}
	}
	if _, err := os.Stat(app.ArtifactPath); err != nil {
		return ErrValidation{"The built app is missing on disk (" + app.ArtifactPath + "). Publish the project again."}
	}
	dest, err := installedAppPath(app)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	if err := os.RemoveAll(dest); err != nil {
		return err
	}
	if out, err := exec.CommandContext(ctx, "ditto", app.ArtifactPath, dest).CombinedOutput(); err != nil {
		return ErrValidation{"Install failed: " + strings.TrimSpace(string(out))}
	}
	return s.repo.InstallApp(ctx, id)
}

// OpenApp launches the installed app from ~/Applications.
func (s *Service) OpenApp(ctx context.Context, id int64) error {
	app, err := s.repo.GetStoreApp(ctx, id)
	if err != nil {
		return err
	}
	if app.ArtifactPath == "" {
		return ErrValidation{"This app has no built artifact. Publish its project first."}
	}
	dest, err := installedAppPath(app)
	if err != nil {
		return err
	}
	if _, err := os.Stat(dest); err != nil {
		return ErrValidation{"App not found in ~/Applications — install it first."}
	}
	if out, err := exec.CommandContext(ctx, "open", dest).CombinedOutput(); err != nil {
		return ErrValidation{"Open failed: " + strings.TrimSpace(string(out))}
	}
	return nil
}

func installedAppPath(app StoreApp) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Applications", filepath.Base(app.ArtifactPath)), nil
}

func (s *Service) InstalledApps(ctx context.Context) ([]StoreApp, error) {
	return s.repo.InstalledApps(ctx)
}
func (s *Service) GetSettings(ctx context.Context) (Settings, error) { return s.repo.GetSettings(ctx) }
func (s *Service) SaveSettings(ctx context.Context, settings Settings) error {
	if settings.Appearance == "" {
		settings.Appearance = "system"
	}
	if settings.StyleProfile == "" {
		settings.StyleProfile = "graphite"
	}
	return s.repo.SaveSettings(ctx, settings)
}
