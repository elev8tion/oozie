package projects

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"oozie/internal/db"
)

type fakeBuilder struct {
	fail bool
}

func (f fakeBuilder) Build(workdir, appName string) (string, error) {
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

func TestInstallRequiresArtifact(t *testing.T) {
	ctx := context.Background()
	s := newTestService(t)
	p, _ := s.CreateProject(ctx, "NoArt", filepath.Join(t.TempDir(), "n"), true)
	id, err := s.repo.UpsertStoreApp(ctx, p.ID, PublishDraft{AppName: "NoArt", Headline: "h", Description: "d"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.InstallApp(ctx, id); err == nil {
		t.Fatal("expected validation error installing an app with no artifact")
	}
}
