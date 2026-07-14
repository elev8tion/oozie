package projects

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Surgery: point at a pixel in a screenshot of the running app, say what
// should change, and the agent maps the point back to source and operates.

// surgeryWorkdir resolves the app's project workdir or explains why not.
func (s *Service) surgeryWorkdir(ctx context.Context, appID int64) (StoreApp, string, error) {
	app, err := s.repo.GetStoreApp(ctx, appID)
	if err != nil {
		return StoreApp{}, "", err
	}
	if app.ProjectID == nil {
		return app, "", ErrValidation{"This app has no linked project — nothing to operate on."}
	}
	project, err := s.repo.GetProject(ctx, *app.ProjectID)
	if err != nil {
		return app, "", err
	}
	workdir, err := projectWorkdir(project)
	if err != nil {
		return app, "", ErrValidation{"Project directory unavailable: " + err.Error()}
	}
	return app, workdir, nil
}

// ScreenshotPath returns the project's latest visual-review capture, or ""
// when none exists yet.
func (s *Service) ScreenshotPath(ctx context.Context, appID int64) (StoreApp, string) {
	app, workdir, err := s.surgeryWorkdir(ctx, appID)
	if err != nil {
		return app, ""
	}
	p := filepath.Join(workdir, "review.png")
	if _, err := os.Stat(p); err != nil {
		return app, ""
	}
	return app, p
}

// CaptureScreenshot runs the project's visual-review script: build,
// launch, screenshot to review.png, quit. Slow — the handler owns UX.
func (s *Service) CaptureScreenshot(ctx context.Context, appID int64) error {
	app, workdir, err := s.surgeryWorkdir(ctx, appID)
	if err != nil {
		return err
	}
	script := filepath.Join(workdir, "Tools", "visual-review.sh")
	if _, err := os.Stat(script); err != nil {
		return ErrValidation{"This project has no Tools/visual-review.sh (send the agent one message to seed the project tools)."}
	}
	execName := strings.ReplaceAll(strings.TrimSpace(app.Name), " ", "")
	cctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(cctx, "sh", "Tools/visual-review.sh", execName)
	cmd.Dir = workdir
	if out, err := cmd.CombinedOutput(); err != nil {
		return ErrValidation{"Screenshot capture failed: " + tailStr(string(out), 400)}
	}
	if _, err := os.Stat(filepath.Join(workdir, "review.png")); err != nil {
		return ErrValidation{"The capture ran but produced no review.png — is Screen Recording permission granted?"}
	}
	return nil
}

// FileSurgery turns a click on the screenshot plus a note into an agent
// request that operates on the element at that point.
func (s *Service) FileSurgery(ctx context.Context, appID int64, xPct, yPct float64, note string) error {
	note = strings.TrimSpace(note)
	if note == "" {
		return ErrValidation{"Describe what should change at that spot."}
	}
	if xPct < 0 || xPct > 100 || yPct < 0 || yPct > 100 {
		return ErrValidation{"Click a point on the screenshot first."}
	}
	app, _, err := s.surgeryWorkdir(ctx, appID)
	if err != nil {
		return err
	}
	msg := fmt.Sprintf(`[surgery on %q — point-and-fix]

The project root contains review.png, a screenshot of this app's window. The user clicked the point at %.1f%% from the left edge and %.1f%% from the top edge of that image, and asked:

%s

Read review.png, identify the UI element at that point, find it in the source, and make the change. Then rebuild and re-run the visual review (sh Tools/visual-review.sh) to confirm the result. oozie republishes the app automatically when you finish.`, app.Name, xPct, yPct, note)
	requestID, err := s.sendAgentMessage(ctx, *app.ProjectID, "build", msg)
	if err != nil {
		return err
	}
	s.appendTasteSignal("surgery "+app.Name, note)
	// Surgery rides the improve loop: republish + reinstall on completion.
	return s.repo.InsertImproveRequest(ctx, requestID, app.ID, "[surgery] "+note)
}

func tailStr(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return "…" + s[len(s)-n:]
}
