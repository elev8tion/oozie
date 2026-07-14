package projects

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"oozie/internal/db"
)

type fakeBuilder struct {
	fail bool
}

func (f fakeBuilder) Build(workdir, appName, beaconURL string) (string, error) {
	if f.fail {
		return "", os.ErrNotExist
	}
	app := filepath.Join(workdir, "dist", appName+".app")
	if err := os.MkdirAll(app, 0o755); err != nil {
		return "", err
	}
	return app, nil
}

func newTestService(t *testing.T) *Service {
	t.Helper()
	database, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	if err := db.RunMigrations(database, os.DirFS(filepath.Join("..", "..", "..", "migrations"))); err != nil {
		t.Fatal(err)
	}
	tasteDirOverride = t.TempDir()
	t.Cleanup(func() { tasteDirOverride = "" })
	return NewService(NewRepo(database))
}

func TestCreateProjectValidation(t *testing.T) {
	s := newTestService(t)
	if _, err := s.CreateProject(context.Background(), "  ", "", true); err == nil {
		t.Fatal("expected validation error for blank name")
	}
	p, err := s.CreateProject(context.Background(), "My Tool", "", true)
	if err != nil {
		t.Fatal(err)
	}
	if p.ProjectPathDisplay != "~/Projects/my-tool" {
		t.Errorf("default path = %q", p.ProjectPathDisplay)
	}
}

func TestPublishRequiresDraft(t *testing.T) {
	s := newTestService(t)
	p, _ := s.CreateProject(context.Background(), "NoDraft", filepath.Join(t.TempDir(), "p"), true)
	err := s.Publish(context.Background(), p.ID)
	if _, ok := err.(ErrValidation); !ok {
		t.Fatalf("expected validation error without a draft, got %v", err)
	}
}

func TestPublishLifecycle(t *testing.T) {
	ctx := context.Background()
	s := newTestService(t)
	s.SetBuilder(fakeBuilder{})
	workdir := filepath.Join(t.TempDir(), "proj")
	p, _ := s.CreateProject(ctx, "Shiny", workdir, true)
	draft := PublishDraft{ProjectID: p.ID, AppName: "Shiny", Headline: "h", Description: "d"}
	if err := s.SaveDraft(ctx, draft); err != nil {
		t.Fatal(err)
	}

	if err := s.Publish(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	s.WaitForJobs()

	for _, seed := range []string{"DESIGN.md", "Tools/visual-review.sh", "Tools/WindowID.swift", "Tools/generate-icon.sh", "Tools/IconGen.swift"} {
		if _, err := os.Stat(filepath.Join(workdir, seed)); err != nil {
			t.Errorf("%s was not seeded into the project workdir: %v", seed, err)
		}
	}

	jobs, err := s.ListJobs(ctx, "")
	if err != nil || len(jobs) != 1 {
		t.Fatalf("jobs = %v (err %v)", jobs, err)
	}
	if jobs[0].Status != "succeeded" {
		t.Fatalf("job status = %s (%s)", jobs[0].Status, jobs[0].ErrorMessage)
	}
	if jobs[0].ProjectName != "Shiny" {
		t.Errorf("job project name = %q", jobs[0].ProjectName)
	}

	apps, err := s.ListStoreApps(ctx, "", "")
	if err != nil || len(apps) != 1 {
		t.Fatalf("store apps = %v (err %v)", apps, err)
	}
	if apps[0].ArtifactPath == "" {
		t.Error("store app has no artifact path")
	}

	// Republishing updates the same store app instead of duplicating it.
	if err := s.Publish(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	s.WaitForJobs()
	apps, _ = s.ListStoreApps(ctx, "", "")
	if len(apps) != 1 {
		t.Fatalf("after republish, store apps = %d, want 1", len(apps))
	}
}

func TestPublishFailureRecordsError(t *testing.T) {
	ctx := context.Background()
	s := newTestService(t)
	s.SetBuilder(fakeBuilder{fail: true})
	p, _ := s.CreateProject(ctx, "Broken", filepath.Join(t.TempDir(), "b"), true)
	_ = s.SaveDraft(ctx, PublishDraft{ProjectID: p.ID, AppName: "Broken", Headline: "h", Description: "d"})

	if err := s.Publish(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	s.WaitForJobs()

	jobs, _ := s.ListJobs(ctx, "failed")
	if len(jobs) != 1 || jobs[0].ErrorMessage == "" {
		t.Fatalf("expected one failed job with an error message, got %v", jobs)
	}
	if apps, _ := s.ListStoreApps(ctx, "", ""); len(apps) != 0 {
		t.Errorf("failed publish must not create a store app, got %d", len(apps))
	}
}

func TestUninstallAndRemoveStoreApp(t *testing.T) {
	ctx := context.Background()
	s := newTestService(t)
	p, _ := s.CreateProject(ctx, "Gone Soon", filepath.Join(t.TempDir(), "g"), true)
	id, err := s.repo.UpsertStoreApp(ctx, p.ID, PublishDraft{AppName: "Gone Soon", Headline: "h", Description: "d"}, "", "gone-soon")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.repo.InstallApp(ctx, id); err != nil {
		t.Fatal(err)
	}

	if err := s.UninstallApp(ctx, id); err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	app, _ := s.repo.GetStoreApp(ctx, id)
	if app.Installed || app.InstallCount != 0 {
		t.Errorf("after uninstall: installed=%v count=%d", app.Installed, app.InstallCount)
	}

	if err := s.RemoveStoreApp(ctx, id); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if apps, _ := s.ListStoreApps(ctx, "", ""); len(apps) != 0 {
		t.Errorf("store should be empty after remove, has %d", len(apps))
	}
}

func TestInstallRequiresArtifact(t *testing.T) {
	ctx := context.Background()
	s := newTestService(t)
	p, _ := s.CreateProject(ctx, "NoArt", filepath.Join(t.TempDir(), "n"), true)
	id, err := s.repo.UpsertStoreApp(ctx, p.ID, PublishDraft{AppName: "NoArt", Headline: "h", Description: "d"}, "", "noart")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.InstallApp(ctx, id); err == nil {
		t.Fatal("expected validation error installing an app with no artifact")
	}
}

// TestImproveLoopRepublishes proves the fix-me wormhole closes itself: an
// improve-linked agent request that settles 'completed' republishes the
// project and marks the improvement done.
func TestImproveLoopRepublishes(t *testing.T) {
	ctx := context.Background()
	s := newTestService(t)
	s.SetBuilder(fakeBuilder{})
	p, _ := s.CreateProject(ctx, "Fixable", filepath.Join(t.TempDir(), "f"), true)
	_ = s.SaveDraft(ctx, PublishDraft{ProjectID: p.ID, AppName: "Fixable", Headline: "h", Description: "d"})
	if err := s.Publish(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	s.WaitForJobs()
	app, err := s.AppBySlug(ctx, "fixable")
	if err != nil {
		t.Fatalf("published app not found by slug: %v", err)
	}

	session, _ := s.repo.GetSession(ctx, p.ID)
	reqID, err := s.repo.CreateAgentRequest(ctx, session.ID, "build", "make it better")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.repo.InsertImproveRequest(ctx, reqID, app.ID, "make it better"); err != nil {
		t.Fatal(err)
	}

	s.RequestSettled(p.ID, reqID, "completed")
	s.WaitForJobs()

	imp, err := s.repo.ImproveByRequest(ctx, reqID)
	if err != nil || imp == nil {
		t.Fatalf("improve request lookup: %v", err)
	}
	if imp.Status != "done" {
		t.Fatalf("improve status = %q, want done", imp.Status)
	}
	jobs, _ := s.ListJobs(ctx, "succeeded")
	if len(jobs) != 2 {
		t.Fatalf("expected the improvement to trigger a second publish job, got %d succeeded", len(jobs))
	}
}

// TestImproveLoopFailedAgent marks the improvement failed when the agent
// request settles unsuccessfully, without republishing.
func TestImproveLoopFailedAgent(t *testing.T) {
	ctx := context.Background()
	s := newTestService(t)
	s.SetBuilder(fakeBuilder{})
	p, _ := s.CreateProject(ctx, "Unfixable", filepath.Join(t.TempDir(), "u"), true)
	id, _ := s.repo.UpsertStoreApp(ctx, p.ID, PublishDraft{AppName: "Unfixable", Headline: "h", Description: "d"}, "", "unfixable")
	session, _ := s.repo.GetSession(ctx, p.ID)
	reqID, _ := s.repo.CreateAgentRequest(ctx, session.ID, "build", "impossible")
	_ = s.repo.InsertImproveRequest(ctx, reqID, id, "impossible")

	s.RequestSettled(p.ID, reqID, "failed")
	s.WaitForJobs()

	imp, _ := s.repo.ImproveByRequest(ctx, reqID)
	if imp == nil || imp.Status != "failed" {
		t.Fatalf("improve status = %v, want failed", imp)
	}
	if jobs, _ := s.ListJobs(ctx, ""); len(jobs) != 0 {
		t.Fatalf("failed agent run must not publish, got %d jobs", len(jobs))
	}
}

// TestReaperRemovesExpiredApps proves disposable apps self-destruct: an
// app published with a past expiry is uninstalled and delisted by the
// reaper sweep, while permanent apps survive.
func TestReaperRemovesExpiredApps(t *testing.T) {
	ctx := context.Background()
	s := newTestService(t)
	p, _ := s.CreateProject(ctx, "Ephemeral", filepath.Join(t.TempDir(), "e"), true)
	id, err := s.repo.UpsertStoreApp(ctx, p.ID, PublishDraft{AppName: "Ephemeral", Headline: "h", Description: "d", ExpiresDays: 1}, "", "ephemeral")
	if err != nil {
		t.Fatal(err)
	}
	p2, _ := s.CreateProject(ctx, "Forever", filepath.Join(t.TempDir(), "f2"), true)
	if _, err := s.repo.UpsertStoreApp(ctx, p2.ID, PublishDraft{AppName: "Forever", Headline: "h", Description: "d"}, "", "forever"); err != nil {
		t.Fatal(err)
	}

	// Not yet expired: sweep must keep it.
	s.reapExpiredApps(ctx)
	if _, err := s.GetStoreApp(ctx, id); err != nil {
		t.Fatalf("unexpired app was reaped: %v", err)
	}

	// Force the expiry into the past and sweep again.
	if _, err := s.repo.db.ExecContext(ctx, `UPDATE store_apps SET expires_at=datetime('now','-1 hour') WHERE id=?`, id); err != nil {
		t.Fatal(err)
	}
	s.reapExpiredApps(ctx)
	if _, err := s.GetStoreApp(ctx, id); err == nil {
		t.Fatal("expired app still in store after reap")
	}
	apps, _ := s.ListStoreApps(ctx, "", "")
	if len(apps) != 1 || apps[0].Name != "Forever" {
		t.Fatalf("permanent app should survive the reap, got %v", apps)
	}
}

// TestRemixCopiesSource proves remixing forks the workdir (minus build
// products) into a new project. Agent send fails (no pi in tests) but the
// fork itself must be complete by then.
func TestRemixCopiesSource(t *testing.T) {
	ctx := context.Background()
	s := newTestService(t)
	srcDir := filepath.Join(t.TempDir(), "orig")
	p, _ := s.CreateProject(ctx, "Origin", srcDir, true)
	id, _ := s.repo.UpsertStoreApp(ctx, p.ID, PublishDraft{AppName: "Origin", Headline: "h", Description: "d"}, "", "origin")
	writeTestFile(t, filepath.Join(srcDir, "Package.swift"), "// package")
	writeTestFile(t, filepath.Join(srcDir, "Sources", "main.swift"), "print(1)")
	writeTestFile(t, filepath.Join(srcDir, ".build", "junk"), "x")
	writeTestFile(t, filepath.Join(srcDir, "dist", "junk"), "x")

	if _, err := s.RemixApp(ctx, id, ""); err == nil {
		t.Fatal("empty mutation must be rejected")
	}
	remix, err := s.RemixApp(ctx, id, "make it purple")
	if _, ok := err.(ErrValidation); !ok {
		t.Fatalf("expected agent-unavailable validation error, got %v", err)
	}
	if remix.ID == 0 {
		t.Fatal("remix project was not created")
	}
	home, _ := os.UserHomeDir()
	dstDir := filepath.Join(home, "Projects", "origin-remix")
	t.Cleanup(func() {
		// Only clean what this test created — never a real user directory.
		if _, err := os.Stat(filepath.Join(dstDir, "Sources", "main.swift")); err == nil {
			os.RemoveAll(dstDir)
		}
	})
	if _, err := os.Stat(filepath.Join(dstDir, "Package.swift")); err != nil {
		t.Errorf("source not copied: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dstDir, "Sources", "main.swift")); err != nil {
		t.Errorf("nested source not copied: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dstDir, ".build")); err == nil {
		t.Error(".build must not be copied")
	}
	if _, err := os.Stat(filepath.Join(dstDir, "dist")); err == nil {
		t.Error("dist must not be copied")
	}
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestRecipeRoundTrip exports a published app as a recipe and imports it
// back into a fresh project.
func TestRecipeRoundTrip(t *testing.T) {
	ctx := context.Background()
	s := newTestService(t)
	dir := filepath.Join(t.TempDir(), "r")
	p, _ := s.CreateProject(ctx, "Recipe App", dir, true)
	id, _ := s.repo.UpsertStoreApp(ctx, p.ID, PublishDraft{AppName: "Recipe App", Headline: "hl", Description: "de"}, "", "recipe-app")
	session, _ := s.repo.GetSession(ctx, p.ID)
	_, _ = s.repo.CreateAgentRequest(ctx, session.ID, "build", "build me a timer")
	_, _ = s.repo.CreateAgentRequest(ctx, session.ID, "build", "make it purple")

	rec, err := s.ExportRecipe(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if rec.Kind != "oozie-recipe/v1" || len(rec.Prompts) != 2 || rec.Prompts[1] != "make it purple" {
		t.Fatalf("recipe = %+v", rec)
	}

	// Import: agent send fails without pi, but the project must exist and
	// carry the recipe's identity.
	raw, _ := json.Marshal(rec)
	imported, err := s.ImportRecipe(ctx, string(raw))
	if _, ok := err.(ErrValidation); !ok {
		t.Fatalf("expected agent-unavailable validation error, got %v", err)
	}
	if imported.ID == 0 || imported.Name != "Recipe App" {
		t.Fatalf("imported project = %+v", imported)
	}
	home, _ := os.UserHomeDir()
	if _, err := os.Stat(filepath.Join(home, "Projects", "recipe-app")); err == nil {
		os.RemoveAll(filepath.Join(home, "Projects", "recipe-app"))
	}

	if _, err := s.ImportRecipe(ctx, `{"kind":"nope"}`); err == nil {
		t.Fatal("wrong kind must be rejected")
	}
}

// TestWishLifecycle covers the fairy loop's plumbing without pi: a wish is
// added, a build attempt fails cleanly (no agent), and the settle path
// publishes and marks a wish built when the agent completes.
func TestWishLifecycle(t *testing.T) {
	ctx := context.Background()
	s := newTestService(t)
	s.SetBuilder(fakeBuilder{})

	if err := s.AddWish(ctx, "   "); err == nil {
		t.Fatal("blank wish must be rejected")
	}
	if err := s.AddWish(ctx, "I wish I had an app that tracks my sourdough starter feedings"); err != nil {
		t.Fatal(err)
	}
	wishes, _ := s.ListWishes(ctx)
	if len(wishes) != 1 || wishes[0].Status != "pending" {
		t.Fatalf("wishes = %+v", wishes)
	}

	// No pi in tests: BuildWish must fail the wish honestly.
	if err := s.BuildWish(ctx, wishes[0].ID); err == nil {
		t.Fatal("expected agent-unavailable error")
	}
	w, _ := s.repo.GetWish(ctx, wishes[0].ID)
	if w.Status != "failed" {
		t.Fatalf("wish status = %q, want failed", w.Status)
	}
	// Clean up the wish project's default ~/Projects/<slug> directory.
	if ps, _ := s.ListProjects(ctx, "", ""); len(ps) > 0 {
		home, _ := os.UserHomeDir()
		dir := filepath.Join(home, strings.TrimPrefix(ps[0].ProjectPathDisplay, "~/"))
		if strings.HasPrefix(dir, filepath.Join(home, "Projects")+string(filepath.Separator)) {
			os.RemoveAll(dir)
		}
	}

	// Simulate the agent finishing: settleWish must publish and grant.
	p, _ := s.CreateProject(ctx, "Granted", filepath.Join(t.TempDir(), "gr"), true)
	_ = s.repo.SaveDraft(ctx, PublishDraft{ProjectID: p.ID, AppName: "Granted", Headline: "h", Description: "d"})
	_ = s.AddWish(ctx, "another wish")
	wishes, _ = s.ListWishes(ctx)
	var wid int64
	for _, x := range wishes {
		if x.Status == "pending" {
			wid = x.ID
		}
	}
	_ = s.repo.SetWishBuilding(ctx, wid, p.ID)
	s.wishByRequest.Store(int64(4242), wid)
	s.RequestSettled(p.ID, 4242, "completed")
	s.WaitForJobs()
	w, _ = s.repo.GetWish(ctx, wid)
	if w.Status != "built" {
		t.Fatalf("wish status = %q (%s), want built", w.Status, w.Error)
	}
	if apps, _ := s.ListStoreApps(ctx, "", ""); len(apps) != 1 {
		t.Fatalf("granted wish should have published an app, got %d", len(apps))
	}
}

// TestTasteAccumulatesSignals proves remix mutations land in TASTE.md and
// that the taste file is materialized into project workdirs.
func TestTasteAccumulatesSignals(t *testing.T) {
	ctx := context.Background()
	s := newTestService(t)
	srcDir := filepath.Join(t.TempDir(), "t")
	p, _ := s.CreateProject(ctx, "Tasteful", srcDir, true)
	id, _ := s.repo.UpsertStoreApp(ctx, p.ID, PublishDraft{AppName: "Tasteful", Headline: "h", Description: "d"}, "", "tasteful")
	writeTestFile(t, filepath.Join(srcDir, "Package.swift"), "// p")

	_, _ = s.RemixApp(ctx, id, "menu-bar apps over windows, always")
	home, _ := os.UserHomeDir()
	t.Cleanup(func() { os.RemoveAll(filepath.Join(home, "Projects", "tasteful-remix")) })

	taste := s.LoadTaste()
	if !strings.Contains(taste, "menu-bar apps over windows") || !strings.Contains(taste, "[remix Tasteful]") {
		t.Fatalf("remix signal missing from taste:\n%s", taste)
	}
	dir := filepath.Join(t.TempDir(), "mat")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	s.materializeTaste(dir)
	body, err := os.ReadFile(filepath.Join(dir, "TASTE.md"))
	if err != nil || !strings.Contains(string(body), "menu-bar apps") {
		t.Fatalf("taste not materialized: %v", err)
	}
}
