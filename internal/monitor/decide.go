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
	// State is the resort_conditions row. Its lift count comes from the last hourly scrape or,
	// for a closed resort (which the hourly queue skips), from a scrape the monitor saved.
	State supabase.ResortState
	// Scrape is the monitor's own scrape just now (nil if it failed, see ScrapeErr), and Raw is
	// the same scrape in the shape the hourly scraper writes to resort_conditions.
	Scrape    *ScrapeResult
	Raw       map[string]interface{}
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
	// SaveScrape: write the monitor's scrape to resort_conditions. Used for a closed resort
	// that looks operating, since the hourly queue skips closed resorts and would never
	// provide the second reading AutoOpen needs.
	SaveScrape bool
	Reason     string
}

func intValue(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}

// operating reports whether a scrape shows the resort running. Open runs/trails is the main
// signal: many sites (Vail Resorts in particular) zero out lifts after hours but keep showing
// open trails. Lifts count too, for sites that only report lifts.
func operating(runs, lifts int) bool {
	return runs > 0 || lifts > 0
}

func decide(o Observation, now time.Time) Decision {
	var d Decision
	status := o.State.Status
	manual := o.State.StatusSource == "manual"
	var runsNow, liftsNow int
	if o.Scrape != nil {
		runsNow, liftsNow = o.Scrape.RunsOpen, o.Scrape.LiftsOpen
	}

	// Impossible numbers mean the selectors read the wrong elements; they're never evidence.
	impossible := ""
	switch {
	case o.Scrape != nil && exceeds(liftsNow, o.State.TotalLifts):
		impossible = fmt.Sprintf("scraped %d lifts open but the resort has %d", liftsNow, *o.State.TotalLifts)
	case o.Scrape != nil && exceeds(runsNow, o.State.TotalRuns):
		impossible = fmt.Sprintf("scraped %d runs open but the resort has %d", runsNow, *o.State.TotalRuns)
	}

	// Broken scraper. A closed resort's page often stops showing data, so only chase fixes
	// for resorts that should be reporting.
	if status != "closed" {
		switch {
		case o.ScrapeErr != nil:
			d.FixConfig = true
			d.Reason = fmt.Sprintf("scrape failed: %v", o.ScrapeErr)
		case impossible != "":
			d.FixConfig = true
			d.Reason = impossible
		}
	}

	// Manual statuses are never changed automatically.
	if manual || impossible != "" {
		return d
	}

	operatingNow := o.Scrape != nil && operating(runsNow, liftsNow)
	operatingBefore := operating(intValue(o.State.RunsOpen), intValue(o.State.LiftsOpen))
	switch {
	case operatingNow && operatingBefore:
		if status != "open" {
			d.AutoOpen = true
		}
	case operatingNow && status == "closed":
		// Hourly scrapes skip closed resorts, so save this reading for tomorrow's check to
		// confirm, and have Claude read the page in the meantime.
		d.SaveScrape = true
		d.ReadStatus = true
	case operatingNow:
		// Only one reading so far; the next hourly scrape plus tomorrow's check will confirm.
	case status == "closed":
		// Closed resorts: read daily in the opening window, weekly otherwise.
		if inOpeningWindow(now) || now.Weekday() == time.Monday {
			d.ReadStatus = true
		}
	default:
		// Open with no terrain reported, unknown, or the scrape failed.
		d.ReadStatus = true
	}
	return d
}

// exceeds reports whether n is more than a known, positive total.
func exceeds(n int, total *int) bool {
	return total != nil && *total > 0 && n > *total
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
