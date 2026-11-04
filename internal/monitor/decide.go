package monitor

import (
	"fmt"
	"time"

	"powderhoundgo/internal/supabase"
)

// ScrapeResult is the numeric output of one scrape.
type ScrapeResult struct {
	BaseDepth   int
	Snow24h     int
	Snow48h     int
	SeasonTotal int
	RunsOpen    int
	LiftsOpen   int
}

// Observation is everything the monitor knows about one resort at check time.
type Observation struct {
	Slug   string
	Config supabase.ScrapingConfig
	// State is the resort_conditions row; its lift count comes from the last hourly scrape.
	State supabase.ResortState
	// Scrape is the monitor's own scrape just now (nil if it failed, see ScrapeErr).
	Scrape    *ScrapeResult
	ScrapeErr error
}

// Decision says what to do for one resort. More than one can apply.
type Decision struct {
	// AutoOpen: two independent scrapes saw lifts spinning; mark open without approval.
	AutoOpen bool
	// ReadStatus: lift data can't settle the status; ask Claude to read the page and
	// propose a change through a GitHub issue if it disagrees.
	ReadStatus bool
	// FixConfig: the scraper is broken for a resort that should be reporting data.
	FixConfig bool
	Reason    string
}

func intValue(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}

func decide(o Observation, now time.Time) Decision {
	var d Decision
	status := o.State.Status
	manual := o.State.StatusSource == "manual"
	liftsNow := 0
	if o.Scrape != nil {
		liftsNow = o.Scrape.LiftsOpen
	}
	liftsBefore := intValue(o.State.LiftsOpen)

	// Broken scraper: our scrape failed, or returned impossible numbers. A closed resort's
	// page often stops showing data, so only chase fixes for resorts that should report.
	if status != "closed" {
		switch {
		case o.ScrapeErr != nil:
			d.FixConfig = true
			d.Reason = fmt.Sprintf("scrape failed: %v", o.ScrapeErr)
		case o.Scrape != nil && o.State.TotalLifts != nil && *o.State.TotalLifts > 0 && liftsNow > *o.State.TotalLifts:
			d.FixConfig = true
			d.Reason = fmt.Sprintf("scraped %d lifts open but the resort has %d", liftsNow, *o.State.TotalLifts)
		}
	}

	if manual {
		return d
	}

	liftsSpinning := o.Scrape != nil && liftsNow > 0
	switch {
	case liftsSpinning && liftsBefore > 0:
		if status != "open" {
			d.AutoOpen = true
		}
	case liftsSpinning:
		// Only one reading so far; the next hourly scrape plus tomorrow's check will confirm.
	case status == "closed":
		// Closed resorts: read daily in the opening window, weekly otherwise.
		if inOpeningWindow(now) || now.Weekday() == time.Monday {
			d.ReadStatus = true
		}
	default:
		// Open with no lifts running at check time, unknown, or the scrape failed.
		d.ReadStatus = true
	}
	return d
}

// inOpeningWindow covers October through December, when Colorado resorts open.
func inOpeningWindow(now time.Time) bool {
	m := now.Month()
	return m >= time.October && m <= time.December
}

// FieldCheck is one expected-vs-scraped comparison.
type FieldCheck struct {
	Field    string
	Expected int
	Scraped  int
	OK       bool
}

// compareScrape checks a scrape against the values Claude read off the page. A field the
// page doesn't show (-1) or whose selector is empty is skipped.
func compareScrape(expected ExpectedValues, got ScrapeResult, cfg supabase.ScrapingConfig) []FieldCheck {
	type field struct {
		name     string
		expected int
		scraped  int
		enabled  bool
	}
	fields := []field{
		{"base_depth", expected.BaseDepth, got.BaseDepth, true},
		{"snow_24h", expected.Snow24h, got.Snow24h, true},
		{"snow_48h", expected.Snow48h, got.Snow48h, true},
		{"season_total", expected.SeasonTotal, got.SeasonTotal, cfg.Conditions.SeasonTotalSelector != ""},
		{"runs_open", expected.RunsOpen, got.RunsOpen, true},
		{"lifts_open", expected.LiftsOpen, got.LiftsOpen, true},
	}
	var checks []FieldCheck
	for _, f := range fields {
		if !f.enabled || f.expected < 0 {
			continue
		}
		checks = append(checks, FieldCheck{Field: f.name, Expected: f.expected, Scraped: f.scraped, OK: f.expected == f.scraped})
	}
	return checks
}

func allOK(checks []FieldCheck) bool {
	if len(checks) == 0 {
		return false
	}
	for _, c := range checks {
		if !c.OK {
			return false
		}
	}
	return true
}

// toScrapeResult reads the map ScrapeWithConfig returns.
func toScrapeResult(m map[string]interface{}) ScrapeResult {
	get := func(key string) int {
		switch v := m[key].(type) {
		case int:
			return v
		case int64:
			return int(v)
		case float64:
			return int(v)
		}
		return 0
	}
	return ScrapeResult{
		BaseDepth:   get("base_depth"),
		Snow24h:     get("snow_past_24h"),
		Snow48h:     get("snow_past_48h"),
		SeasonTotal: get("snow_total"),
		RunsOpen:    get("runs_open"),
		LiftsOpen:   get("lifts_open"),
	}
}
