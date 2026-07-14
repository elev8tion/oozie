package projects

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"
	"unicode"
)

// The wish inbox and its nightly build fairy: ideas dropped in during the
// day become working, published apps overnight. Wishes build in trusted
// projects — an unattended run can't answer permission prompts.

func (s *Service) AddWish(ctx context.Context, text string) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return ErrValidation{"Describe the app you wish existed."}
	}
	return s.repo.AddWish(ctx, text)
}

func (s *Service) DeleteWish(ctx context.Context, id int64) error {
	return s.repo.DeleteWish(ctx, id)
}

func (s *Service) ListWishes(ctx context.Context) ([]Wish, error) {
	return s.repo.ListWishes(ctx)
}

// BuildWish grants a wish now: project, agent build, and — when the agent
// finishes — an automatic publish, so the app lands in the store by itself.
func (s *Service) BuildWish(ctx context.Context, id int64) error {
	wish, err := s.repo.GetWish(ctx, id)
	if err != nil {
		return err
	}
	if wish.Status == "building" {
		return ErrValidation{"This wish is already being built."}
	}
	name := wishProjectName(wish.Text)
	project, err := s.CreateProject(ctx, name, "", true)
	if err != nil {
		return err
	}
	// Auto-install so overnight prototypes are in /Applications by morning.
	draft := PublishDraft{ProjectID: project.ID, AppName: name, Headline: wishHeadline(wish.Text), Description: wish.Text, PublishTarget: "public", Visibility: "unlisted", ScreenshotManifest: "[]", AutoInstall: true}
	if err := s.repo.SaveDraft(ctx, draft); err != nil {
		return err
	}
	msg := fmt.Sprintf("This project grants a wish from the oozie wish inbox. Build the complete app it describes — scaffold, implement, icon, visual review — without asking questions (make sensible choices yourself; this may run unattended overnight). The wish:\n\n%s", wish.Text)
	requestID, err := s.sendAgentMessage(ctx, project.ID, "build", msg)
	if err != nil {
		_ = s.repo.SettleWish(ctx, id, "failed", err.Error())
		return err
	}
	s.wishByRequest.Store(requestID, id)
	return s.repo.SetWishBuilding(ctx, id, project.ID)
}

// settleWish closes the fairy loop: agent done → publish → wish granted.
func (s *Service) settleWish(projectID, requestID int64, status string) {
	v, ok := s.wishByRequest.LoadAndDelete(requestID)
	if !ok {
		return
	}
	wishID := v.(int64)
	ctx := context.Background()
	if status != "completed" {
		_ = s.repo.SettleWish(ctx, wishID, "failed", "the agent run ended with status "+status)
		return
	}
	err := s.publish(ctx, projectID, func(_ int64, buildErr error) {
		if buildErr != nil {
			_ = s.repo.SettleWish(ctx, wishID, "failed", "built by the agent but publish failed: "+buildErr.Error())
			return
		}
		_ = s.repo.SettleWish(ctx, wishID, "built", "")
		log.Printf("wish %d granted: app published", wishID)
	})
	if err != nil {
		_ = s.repo.SettleWish(ctx, wishID, "failed", "publish could not start: "+err.Error())
	}
}

// fairyLoop wakes every minute; at the configured hour it takes up to
// three pending wishes and starts granting them.
func (s *Service) fairyLoop(ctx context.Context) {
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	lastRun := ""
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-t.C:
			settings, err := s.repo.GetSettings(ctx)
			if err != nil || !settings.FairyEnabled || now.Local().Hour() != settings.FairyHour {
				continue
			}
			day := now.Local().Format("2006-01-02")
			if day == lastRun {
				continue
			}
			lastRun = day
			s.runNightShift(ctx)
		}
	}
}

func (s *Service) runNightShift(ctx context.Context) {
	wishes, err := s.repo.PendingWishes(ctx, 3)
	if err != nil {
		log.Printf("night shift: %v", err)
		return
	}
	if len(wishes) == 0 {
		return
	}
	log.Printf("night shift: granting %d wish(es)", len(wishes))
	for _, w := range wishes {
		if err := s.BuildWish(ctx, w.ID); err != nil {
			log.Printf("night shift: wish %d: %v", w.ID, err)
		}
	}
}

// wishProjectName distills a wish into a short project/app name.
func wishProjectName(text string) string {
	words := strings.FieldsFunc(text, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	})
	stop := map[string]bool{"a": true, "an": true, "the": true, "that": true, "for": true, "app": true, "me": true, "my": true, "i": true, "want": true, "wish": true, "wished": true, "build": true, "make": true, "to": true, "of": true, "with": true, "and": true, "had": true, "have": true, "has": true}
	var kept []string
	for _, w := range words {
		if !stop[strings.ToLower(w)] {
			kept = append(kept, strings.Title(strings.ToLower(w)))
		}
		if len(kept) == 3 {
			break
		}
	}
	if len(kept) == 0 {
		return "Wish " + time.Now().Format("Jan 2")
	}
	return strings.Join(kept, " ")
}

// wishHeadline is the wish's first sentence, capped for the store card.
func wishHeadline(text string) string {
	text = strings.TrimSpace(text)
	if i := strings.IndexAny(text, ".!?\n"); i > 0 {
		text = text[:i]
	}
	if len(text) > 80 {
		text = text[:77] + "…"
	}
	if text == "" {
		return "An overnight prototype"
	}
	return text
}
