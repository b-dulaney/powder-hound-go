// Package monitor runs the daily scraper health and resort status check.
//
// For each resort it scrapes with the stored config, then:
//   - marks the resort open automatically when two independent scrapes saw runs or lifts open
//     (for a closed resort, the second scrape is a follow-up task about an hour later);
//   - otherwise asks Claude to read the page (skipped while the page's status text is unchanged
//     since the last read, up to a week), and opens a GitHub issue proposing the status
//     change when Claude disagrees with the database (approved by commenting /approve);
//   - when the scrape is broken, asks Claude for a corrected config, verifies it by scraping
//     with it, and opens a PR with the proof, or an issue if no verified fix was found.
package monitor

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"powderhoundgo/internal/scraping"
	"powderhoundgo/internal/supabase"
)

// Store is the database and storage access the monitor needs (supabase.MonitorStore).
type Store interface {
	ListConfigSlugs() ([]string, error)
	GetConfig(slug string) (supabase.ScrapingConfig, error)
	GetResortStates() (map[int]supabase.ResortState, error)
	SetResortStatus(u supabase.StatusUpdate) (bool, error)
	UploadEvidence(path string, data []byte, contentType string) (string, error)
	GetStatusRead(mountainID int) (*supabase.StatusRead, error)
	RecordJobRun(job string, runErr error)
	RecordStatusRead(mountainID int, fingerprint string, at time.Time) error
	SaveConditions(data map[string]interface{}) error
}

// rereadAfter forces a status read even when the page's status text hasn't changed, in case
// the fingerprint misses a change (or Claude's earlier answer was wrong).
const rereadAfter = 7 * 24 * time.Hour

type ModelClient interface {
	ProposeConfig(ctx context.Context, current supabase.ScrapingConfig, page *scraping.PageCapture, scrapeErr string, previous *ConfigProposal, feedback string) (*ConfigProposal, error)
	ReadStatus(ctx context.Context, resortName string, page *scraping.PageCapture, terrain string) (*StatusReading, error)
}

type Repo interface {
	GetFile(ctx context.Context, path, branch string) ([]byte, string, error)
	OpenFilePR(ctx context.Context, branch, path string, content []byte, commitMsg, title, body string, labels []string) (*PullRequest, error)
	OpenPRsWithMarker(ctx context.Context, marker string) ([]PullRequest, error)
	OpenIssuesWithMarker(ctx context.Context, label, marker string) ([]Issue, error)
	CreateIssue(ctx context.Context, title, body string, labels []string) (*Issue, error)
	BaseBranch() string
}

func (g *GitHub) BaseBranch() string { return g.Base }

type Monitor struct {
	Store  Store
	Claude ModelClient // nil: no status reads or config fixes
	Repo   Repo        // nil: findings are only logged
	Env    string      // "prod" or "dev"; tags evidence, issues and proposals

	Scrape  func(supabase.ScrapingConfig) (map[string]interface{}, error)
	Capture func(url, clickSelector string) (*scraping.PageCapture, error)
	Proof   func(supabase.ScrapingConfig) (*scraping.ProofCapture, error)
	Now     func() time.Time

	// ScheduleConfirmOpen, when set, queues ConfirmOpen for a closed resort whose scrape shows
	// it operating, so it can open about an hour later instead of after the next daily run.
	ScheduleConfirmOpen func(ConfirmOpenRequest) error

	// MaxModelCalls caps Claude requests per run.
	MaxModelCalls int
	// Only, when set, limits the run to one resort slug.
	Only       string
	modelCalls int
}

// New wires the monitor from the environment. Claude and GitHub are optional: without them
// the check still applies lift-based status updates and logs everything else.
func New() (*Monitor, error) {
	env := envName()
	store, err := supabase.NewMonitorStore(env == "prod")
	if err != nil {
		return nil, err
	}
	m := &Monitor{
		Store:         store,
		Env:           env,
		Scrape:        scraping.ScrapeWithConfig,
		Capture:       scraping.CapturePage,
		Proof:         scraping.CaptureProof,
		Now:           time.Now,
		MaxModelCalls: 15,
	}
	if n, err := strconv.Atoi(os.Getenv("MONITOR_MAX_MODEL_CALLS")); err == nil && n >= 0 {
		m.MaxModelCalls = n
	}
	if c, err := NewClaude(); err == nil {
		m.Claude = c
	} else {
		log.Printf("[monitor] Claude disabled: %v", err)
	}
	if g, err := NewGitHub(); err == nil {
		m.Repo = g
	} else {
		log.Printf("[monitor] GitHub disabled: %v", err)
	}
	return m, nil
}

func envName() string {
	if v := os.Getenv("MONITOR_ENV"); v != "" {
		return v
	}
	if os.Getenv("ENV") == "production" {
		return "prod"
	}
	return "dev"
}

// Run checks every resort. Errors for one resort are logged and don't stop the others.
func (m *Monitor) Run(ctx context.Context) error {
	slugs, err := m.Store.ListConfigSlugs()
	if err != nil {
		return err
	}
	states, err := m.Store.GetResortStates()
	if err != nil {
		return err
	}
	log.Printf("[monitor] checking %d resorts (env=%s)", len(slugs), m.Env)
	for _, slug := range slugs {
		if m.Only != "" && slug != m.Only {
			continue
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err := m.checkResort(ctx, slug, states); err != nil {
			log.Printf("[monitor] %s: %v", slug, err)
		}
	}
	log.Printf("[monitor] done (%d model calls)", m.modelCalls)
	return nil
}

func (m *Monitor) checkResort(ctx context.Context, slug string, states map[int]supabase.ResortState) error {
	cfg, err := m.Store.GetConfig(slug)
	if err != nil {
		return err
	}
	state, ok := states[cfg.ID]
	if !ok {
		return fmt.Errorf("no resort_conditions row for mountain_id %d", cfg.ID)
	}

	o := Observation{Slug: slug, Config: cfg, State: state}
	result, err := m.safeScrape(cfg)
	if err != nil {
		o.ScrapeErr = err
	} else {
		r := toScrapeResult(result)
		o.Scrape = &r
		o.Raw = result
	}

	d := decide(o, m.Now())
	scraped := "scrape failed"
	if o.Scrape != nil {
		scraped = fmt.Sprintf("runs=%d lifts=%d", o.Scrape.RunsOpen, o.Scrape.LiftsOpen)
	}
	log.Printf("[monitor] %s: status=%s/%s now(%s) hourly(runs=%d lifts=%d) scrapeErr=%v decision=%+v",
		slug, state.Status, state.StatusSource, scraped, intValue(state.RunsOpen), intValue(state.LiftsOpen), o.ScrapeErr, d)

	if d.AutoOpen {
		evidence := fmt.Sprintf("%d runs / %d lifts open at %s; %d runs / %d lifts at the previous hourly scrape",
			o.Scrape.RunsOpen, o.Scrape.LiftsOpen, m.Now().Format(time.RFC3339), intValue(state.RunsOpen), intValue(state.LiftsOpen))
		changed, err := m.Store.SetResortStatus(supabase.StatusUpdate{
			MountainID: cfg.ID, Status: "open", Source: "auto", Evidence: evidence,
		})
		if err != nil {
			return fmt.Errorf("auto-open: %w", err)
		}
		log.Printf("[monitor] %s: auto-open applied=%v (%s)", slug, changed, evidence)
	}
	if d.SaveScrape {
		if err := m.Store.SaveConditions(o.Raw); err != nil {
			log.Printf("[monitor] %s: saving scrape: %v", slug, err)
		}
		if m.ScheduleConfirmOpen != nil {
			req := ConfirmOpenRequest{Slug: slug, RunsOpen: o.Scrape.RunsOpen, LiftsOpen: o.Scrape.LiftsOpen, ScrapedAt: m.Now()}
			if err := m.ScheduleConfirmOpen(req); err != nil {
				log.Printf("[monitor] %s: scheduling the confirm-open check: %v", slug, err)
			} else {
				log.Printf("[monitor] %s: looks open; confirm-open check scheduled", slug)
			}
		}
	}
	if d.SaveTotals && !d.SaveScrape { // a saved scrape already carries the totals
		totals := map[string]interface{}{"mountain_id": cfg.ID}
		if o.Scrape.LiftsTotal > 0 {
			totals["total_lifts"] = o.Scrape.LiftsTotal
		}
		if o.Scrape.RunsTotal > 0 {
			totals["total_runs"] = o.Scrape.RunsTotal
		}
		if err := m.Store.SaveConditions(totals); err != nil {
			log.Printf("[monitor] %s: saving totals: %v", slug, err)
		} else {
			log.Printf("[monitor] %s: totals now %v (were %d lifts, %d runs)", slug, totals, intValue(state.TotalLifts), intValue(state.TotalRuns))
		}
	}
	if d.FixConfig {
		if err := m.fixConfig(ctx, o, d.Reason); err != nil {
			log.Printf("[monitor] %s: config fix: %v", slug, err)
		}
	}
	if d.ReadStatus {
		if err := m.readStatus(ctx, o); err != nil {
			log.Printf("[monitor] %s: status read: %v", slug, err)
		}
	}
	return nil
}

// safeScrape turns panics from the scrape path (it panics on some config shapes) into errors.
func (m *Monitor) safeScrape(cfg supabase.ScrapingConfig) (result map[string]interface{}, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("scrape panicked: %v", r)
		}
	}()
	return m.Scrape(cfg)
}

func (m *Monitor) takeModelCall() bool {
	if m.Claude == nil || m.modelCalls >= m.MaxModelCalls {
		return false
	}
	m.modelCalls++
	return true
}

func (m *Monitor) evidencePath(slug, kind, ext string) string {
	return fmt.Sprintf("%s/%s/%s-%s.%s", m.Env, slug, m.Now().UTC().Format("20060102T150405Z"), kind, ext)
}

func (m *Monitor) fixConfig(ctx context.Context, o Observation, reason string) error {
	if m.Repo != nil {
		if prs, err := m.Repo.OpenPRsWithMarker(ctx, fixMarker(m.Env, o.Slug)); err == nil && len(prs) > 0 {
			log.Printf("[monitor] %s: fix PR already open: %s", o.Slug, prs[0].HTMLURL)
			return nil
		}
		if issues, err := m.Repo.OpenIssuesWithMarker(ctx, LabelScraperBroken, brokenMarker(m.Env, o.Slug)); err == nil && len(issues) > 0 {
			log.Printf("[monitor] %s: broken-scraper issue already open: %s", o.Slug, issues[0].HTMLURL)
			return nil
		}
	}

	page, err := m.Capture(o.Config.ConditionsURL, o.Config.ClickSelector)
	if err != nil {
		return err
	}

	var proposal *ConfigProposal
	var checks []FieldCheck
	var attempts []string
	feedback := ""
	verified := false
	for attempt := 1; attempt <= 2 && !verified; attempt++ {
		if !m.takeModelCall() {
			attempts = append(attempts, "Skipped: Claude unavailable or the per-run call budget is used up.")
			break
		}
		next, err := m.Claude.ProposeConfig(ctx, o.Config, page, reason, proposal, feedback)
		if err != nil {
			attempts = append(attempts, "Claude request failed: "+err.Error())
			break
		}
		proposal = next
		result, scrapeErr := m.safeScrape(proposal.Config)
		if scrapeErr != nil {
			feedback = "The scrape failed: " + scrapeErr.Error()
			attempts = append(attempts, fmt.Sprintf("Attempt %d: %s", attempt, feedback))
			continue
		}
		checks = compareScrape(proposal.Expected, toScrapeResult(result), proposal.Config)
		if allOK(checks) {
			verified = true
			break
		}
		feedback = describeMismatches(checks)
		attempts = append(attempts, fmt.Sprintf("Attempt %d: %s", attempt, feedback))
	}

	if !verified {
		return m.reportBroken(ctx, o, reason, attempts)
	}
	return m.openFixPR(ctx, o, proposal, checks, reason, len(attempts)+1)
}

func describeMismatches(checks []FieldCheck) string {
	if len(checks) == 0 {
		return "No fields could be compared (all expected values were -1)."
	}
	var parts []string
	for _, c := range checks {
		if !c.OK {
			parts = append(parts, fmt.Sprintf("%s: page shows %d, scrape returned %d", c.Field, c.Expected, c.Scraped))
		}
	}
	return strings.Join(parts, "; ")
}

func (m *Monitor) openFixPR(ctx context.Context, o Observation, p *ConfigProposal, checks []FieldCheck, reason string, attempts int) error {
	proofURL := ""
	if proof, err := m.Proof(p.Config); err != nil {
		log.Printf("[monitor] %s: proof screenshot failed: %v", o.Slug, err)
	} else if url, err := m.Store.UploadEvidence(m.evidencePath(o.Slug, "proof", "jpg"), proof.Screenshot, "image/jpeg"); err != nil {
		log.Printf("[monitor] %s: proof upload failed: %v", o.Slug, err)
	} else {
		proofURL = url
	}

	if m.Repo == nil {
		log.Printf("[monitor] %s: verified fix (GitHub disabled, not opening a PR): %+v", o.Slug, p.Config)
		return nil
	}
	path := fmt.Sprintf("supabase/storage/scraping-config/%s.json", o.Slug)
	original, _, err := m.Repo.GetFile(ctx, path, m.Repo.BaseBranch())
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	content, err := mergeConfigJSON(original, p.Config)
	if err != nil {
		return err
	}
	branch := fmt.Sprintf("scraper-fix/%s-%s", o.Slug, m.Now().UTC().Format("20060102-1504"))
	title := fmt.Sprintf("Scraper fix: %s", o.Config.Name)
	body := fixPRBody(m.Env, o.Slug, o.Config, p, checks, proofURL, reason, attempts)
	pr, err := m.Repo.OpenFilePR(ctx, branch, path, content,
		fmt.Sprintf("fix: update %s scraper config", o.Config.Name), title, body, []string{LabelScraperFix})
	if err != nil {
		return fmt.Errorf("open PR: %w", err)
	}
	log.Printf("[monitor] %s: opened %s", o.Slug, pr.HTMLURL)
	return nil
}

func (m *Monitor) reportBroken(ctx context.Context, o Observation, reason string, attempts []string) error {
	if m.Repo == nil {
		log.Printf("[monitor] %s: broken, no verified fix: %s %v", o.Slug, reason, attempts)
		return nil
	}
	issue, err := m.Repo.CreateIssue(ctx, fmt.Sprintf("Scraper broken: %s", o.Config.Name),
		brokenIssueBody(m.Env, o.Slug, o.Config, reason, attempts), []string{LabelScraperBroken})
	if err != nil {
		return err
	}
	log.Printf("[monitor] %s: opened %s", o.Slug, issue.HTMLURL)
	return nil
}

func (m *Monitor) readStatus(ctx context.Context, o Observation) error {
	if m.Repo != nil {
		if issues, err := m.Repo.OpenIssuesWithMarker(ctx, LabelResortStatus, statusMarker(m.Env, o.Config.ID)); err == nil && len(issues) > 0 {
			log.Printf("[monitor] %s: status issue already open: %s", o.Slug, issues[0].HTMLURL)
			return nil
		}
	}
	if m.Claude == nil {
		return fmt.Errorf("skipped: Claude unavailable")
	}
	page, err := m.Capture(o.Config.ConditionsURL, o.Config.ClickSelector)
	if err != nil {
		return err
	}
	fingerprint := pageFingerprint(page.VisibleText)
	if last, err := m.Store.GetStatusRead(o.Config.ID); err != nil {
		log.Printf("[monitor] %s: can't load the last status read, reading anyway: %v", o.Slug, err)
	} else if last != nil && last.PageFingerprint == fingerprint && m.Now().Sub(last.ReadAt) < rereadAfter {
		log.Printf("[monitor] %s: status text unchanged since %s, skipping the read", o.Slug, last.ReadAt.Format(time.RFC3339))
		return nil
	}
	if !m.takeModelCall() {
		return fmt.Errorf("skipped: the per-run call budget is used up")
	}
	terrain := "unknown (scrape failed)"
	if o.Scrape != nil {
		terrain = fmt.Sprintf("%d of %s runs open, %d of %s lifts open",
			o.Scrape.RunsOpen, totalOrUnknown(o.State.TotalRuns), o.Scrape.LiftsOpen, totalOrUnknown(o.State.TotalLifts))
	}
	r, err := m.Claude.ReadStatus(ctx, o.Config.Name, page, terrain)
	if err != nil {
		return err
	}
	log.Printf("[monitor] %s: Claude reads status=%s confidence=%s quote=%q", o.Slug, r.Status, r.Confidence, r.Quote)
	r.ExpectedOpenDate = dropStaleDate(o.Slug, "open", r.ExpectedOpenDate, m.Now())
	r.ExpectedCloseDate = dropStaleDate(o.Slug, "close", r.ExpectedCloseDate, m.Now())
	// Remember what was read whatever the outcome: an unchanged page gets the same answer, and
	// a proposal that was rejected (issue closed without /approve) isn't re-raised daily.
	if err := m.Store.RecordStatusRead(o.Config.ID, fingerprint, m.Now()); err != nil {
		log.Printf("[monitor] %s: %v", o.Slug, err)
	}

	if r.Status == o.State.Status || r.Status == "unknown" {
		// No status change. Record newly announced dates; they only affect display.
		if r.Confidence != "low" && (r.ExpectedOpenDate != "" || r.ExpectedCloseDate != "") && o.State.Status != "unknown" {
			if _, err := m.Store.SetResortStatus(supabase.StatusUpdate{
				MountainID: o.Config.ID, Status: o.State.Status, Source: o.State.StatusSource,
				Evidence: r.Quote, ExpectedOpenDate: r.ExpectedOpenDate, ExpectedCloseDate: r.ExpectedCloseDate,
			}); err != nil {
				return fmt.Errorf("record dates: %w", err)
			}
		}
		return nil
	}
	// Only propose changes the page states outright. Medium readings tend to rest on indirect
	// hints ("Early Season", pass sale banners); they're re-read when the page's status text changes.
	if r.Confidence != "high" || strings.TrimSpace(r.Quote) == "" {
		log.Printf("[monitor] %s: not proposing %s (confidence %s, quote %q)", o.Slug, r.Status, r.Confidence, r.Quote)
		return nil
	}

	proposal := StatusProposal{
		Env: m.Env, MountainID: o.Config.ID, Status: r.Status, Evidence: r.Quote,
		ExpectedOpenDate: r.ExpectedOpenDate, ExpectedCloseDate: r.ExpectedCloseDate,
	}
	screenshotURL := ""
	if url, err := m.Store.UploadEvidence(m.evidencePath(o.Slug, "status", "png"), page.Screenshot, "image/png"); err == nil {
		screenshotURL = url
	} else {
		log.Printf("[monitor] %s: screenshot upload failed: %v", o.Slug, err)
	}
	if m.Repo == nil {
		log.Printf("[monitor] %s: would propose %+v (GitHub disabled)", o.Slug, proposal)
		return nil
	}
	body, err := statusIssueBody(o, r, proposal, page.URL, screenshotURL)
	if err != nil {
		return err
	}
	title := fmt.Sprintf("Resort status: %s → %s", o.Config.Name, r.Status)
	if m.Env != "prod" {
		title = "[" + m.Env + "] " + title
	}
	issue, err := m.Repo.CreateIssue(ctx, title, body, []string{LabelResortStatus})
	if err != nil {
		return err
	}
	log.Printf("[monitor] %s: opened %s", o.Slug, issue.HTMLURL)
	return nil
}

func totalOrUnknown(total *int) string {
	if total == nil {
		return "?"
	}
	return strconv.Itoa(*total)
}

// dropStaleDate discards an "expected" date that is more than a week past (or unparseable):
// it's almost always a wrong year guessed for a date the page gave without one.
func dropStaleDate(slug, kind, date string, now time.Time) string {
	if date == "" {
		return ""
	}
	d, err := time.Parse("2006-01-02", date)
	if err != nil || d.Before(now.AddDate(0, 0, -7)) {
		log.Printf("[monitor] %s: ignoring expected %s date %q", slug, kind, date)
		return ""
	}
	return date
}
