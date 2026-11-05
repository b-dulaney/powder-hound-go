package monitor

import (
	"context"
	"fmt"
	"log"
	"time"

	"powderhoundgo/internal/supabase"
)

// confirmOpenDelay is how long after the daily check the second scrape of a closed resort
// that looked open runs. Long enough to be an independent reading, short enough that an
// opening shows up the same day.
const confirmOpenDelay = time.Hour

// ConfirmOpenRequest is the payload of the follow-up check (tasks.TypeMonitorConfirmOpen):
// the first scrape that saw a closed resort operating.
type ConfirmOpenRequest struct {
	Slug      string    `json:"slug"`
	RunsOpen  int       `json:"runs_open"`
	LiftsOpen int       `json:"lifts_open"`
	ScrapedAt time.Time `json:"scraped_at"`
}

// ConfirmOpen scrapes a closed resort again and marks it open automatically if it still shows
// runs or lifts open. It does nothing if the status changed in the meantime, was set by hand,
// or the scrape fails or reads impossible numbers; the next daily check tries again.
func (m *Monitor) ConfirmOpen(ctx context.Context, req ConfirmOpenRequest) error {
	cfg, err := m.Store.GetConfig(req.Slug)
	if err != nil {
		return err
	}
	states, err := m.Store.GetResortStates()
	if err != nil {
		return err
	}
	state, ok := states[cfg.ID]
	if !ok {
		return fmt.Errorf("no resort_conditions row for mountain_id %d", cfg.ID)
	}
	if state.Status != "closed" || state.StatusSource == "manual" {
		log.Printf("[monitor] %s: confirm-open: status is now %s/%s, nothing to do", req.Slug, state.Status, state.StatusSource)
		return nil
	}

	result, err := m.safeScrape(cfg)
	if err != nil {
		log.Printf("[monitor] %s: confirm-open: scrape failed, leaving it for the daily check: %v", req.Slug, err)
		return nil
	}
	r := toScrapeResult(result)
	if why := impossibleReading(r, state); why != "" {
		log.Printf("[monitor] %s: confirm-open: not evidence: %s", req.Slug, why)
		return nil
	}
	if !operating(r.RunsOpen, r.LiftsOpen) {
		log.Printf("[monitor] %s: confirm-open: no runs or lifts open now, still closed", req.Slug)
		return nil
	}

	if err := m.Store.SaveConditions(result); err != nil {
		log.Printf("[monitor] %s: confirm-open: saving scrape: %v", req.Slug, err)
	}
	evidence := fmt.Sprintf("%d runs / %d lifts open at %s; %d runs / %d lifts at %s",
		r.RunsOpen, r.LiftsOpen, m.Now().Format(time.RFC3339),
		req.RunsOpen, req.LiftsOpen, req.ScrapedAt.Format(time.RFC3339))
	changed, err := m.Store.SetResortStatus(supabase.StatusUpdate{
		MountainID: cfg.ID, Status: "open", Source: "auto", Evidence: evidence,
	})
	if err != nil {
		return fmt.Errorf("confirm-open: %w", err)
	}
	log.Printf("[monitor] %s: confirm-open applied=%v (%s)", req.Slug, changed, evidence)
	return nil
}
