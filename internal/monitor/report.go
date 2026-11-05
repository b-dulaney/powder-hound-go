package monitor

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"powderhoundgo/internal/supabase"
)

const (
	LabelScraperFix    = "scraper-fix"
	LabelScraperBroken = "scraper-broken"
	LabelResortStatus  = "resort-status"
)

func fixMarker(env, slug string) string {
	return fmt.Sprintf("<!-- powderhound:scraper-fix env=%s slug=%s -->", env, slug)
}

func brokenMarker(env, slug string) string {
	return fmt.Sprintf("<!-- powderhound:scraper-broken env=%s slug=%s -->", env, slug)
}

func statusMarker(env string, mountainID int) string {
	return fmt.Sprintf("<!-- powderhound:resort-status env=%s mountain_id=%d -->", env, mountainID)
}

// StatusProposal is embedded in a resort-status issue; the approval workflow reads it and
// applies it with set_resort_status when the repo owner comments /approve.
type StatusProposal struct {
	Env               string `json:"env"`
	MountainID        int    `json:"mountain_id"`
	Status            string `json:"status"`
	Evidence          string `json:"evidence"`
	ExpectedOpenDate  string `json:"expected_open_date"`
	ExpectedCloseDate string `json:"expected_close_date"`
}

// proposalComment renders the machine-readable block. encoding/json escapes <, > and &,
// so page text can't close the HTML comment early.
func proposalComment(p StatusProposal) (string, error) {
	b, err := json.Marshal(p)
	if err != nil {
		return "", err
	}
	return "<!-- powderhound-proposal " + string(b) + " -->", nil
}

func mdEscape(s string) string {
	return strings.NewReplacer("|", "\\|", "\n", " ").Replace(s)
}

func quoteBlock(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	for i, l := range lines {
		lines[i] = "> " + l
	}
	return strings.Join(lines, "\n")
}

func fixPRBody(env, slug string, cfg supabase.ScrapingConfig, p *ConfigProposal, checks []FieldCheck, proofURL, reason string, attempts int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "The daily scraper check found **%s** broken (%s) and proposes this config. It was verified by scraping %s with the proposed config using the production scrape code.\n\n", cfg.Name, reason, cfg.ConditionsURL)
	fmt.Fprintf(&b, "**What changed:** %s\n\n", p.Summary)
	b.WriteString("| Field | Page shows | Scraped with new config | |\n|---|---|---|---|\n")
	evidence := map[string]string{}
	for _, e := range p.Evidence {
		evidence[e.Field] = e.Quote
	}
	for _, c := range checks {
		mark := "✅"
		if !c.OK {
			mark = "❌"
		}
		page := fmt.Sprintf("%d", c.Expected)
		if q := evidence[c.Field]; q != "" {
			page += " (“" + mdEscape(q) + "”)"
		}
		fmt.Fprintf(&b, "| %s | %s | %d | %s |\n", c.Field, page, c.Scraped, mark)
	}
	if proofURL != "" {
		fmt.Fprintf(&b, "\n**Proof:** each selector's match is outlined in red.\n\n![annotated snow report](%s)\n", proofURL)
	}
	if p.Caveats != "" {
		fmt.Fprintf(&b, "\n**Double-check:** %s\n", p.Caveats)
	}
	fmt.Fprintf(&b, "\nProposed by %s after %d attempt(s). Merging updates `scraping-config/%s.json` in Storage.\n\n%s\n", "Claude", attempts, slug, fixMarker(env, slug))
	return b.String()
}

func brokenIssueBody(env, slug string, cfg supabase.ScrapingConfig, reason string, attempts []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "The daily scraper check found **%s** broken and couldn't produce a verified fix.\n\n**Problem:** %s\n\n**Page:** %s\n\n", cfg.Name, reason, cfg.ConditionsURL)
	if len(attempts) > 0 {
		b.WriteString("**Attempts:**\n")
		for i, a := range attempts {
			fmt.Fprintf(&b, "%d. %s\n", i+1, a)
		}
	}
	fmt.Fprintf(&b, "\nThe config is `supabase/storage/scraping-config/%s.json`. Close this issue once it's fixed.\n\n%s\n", slug, brokenMarker(env, slug))
	return b.String()
}

func statusIssueBody(o Observation, r *StatusReading, proposal StatusProposal, pageURL, screenshotURL string) (string, error) {
	block, err := proposalComment(proposal)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "The daily status check thinks **%s** is now **%s** (currently `%s`).\n\n", o.Config.Name, r.Status, o.State.Status)
	fmt.Fprintf(&b, "**Evidence** from %s:\n\n%s\n\n", pageURL, quoteBlock(r.Quote))
	fmt.Fprintf(&b, "**Reasoning:** %s (confidence: %s)\n\n", r.Reasoning, r.Confidence)
	now := "scrape failed"
	if o.Scrape != nil {
		now = fmt.Sprintf("%d runs / %d lifts open", o.Scrape.RunsOpen, o.Scrape.LiftsOpen)
	}
	fmt.Fprintf(&b, "**Terrain data:** %s now; %d runs / %d lifts at the last hourly scrape.\n\n", now, intValue(o.State.RunsOpen), intValue(o.State.LiftsOpen))
	if r.ExpectedOpenDate != "" || r.ExpectedCloseDate != "" {
		fmt.Fprintf(&b, "**Announced dates:** opening %s, closing %s\n\n", orDash(r.ExpectedOpenDate), orDash(r.ExpectedCloseDate))
	}
	if screenshotURL != "" {
		fmt.Fprintf(&b, "![snow report](%s)\n\n", screenshotURL)
	}
	b.WriteString("Comment `/approve` to apply this status, or close the issue to reject it.\n\n")
	b.WriteString(statusMarker(proposal.Env, proposal.MountainID) + "\n" + block + "\n")
	return b.String(), nil
}

func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

// mergeConfigJSON overlays the proposed config onto the original file, so keys the Go struct
// doesn't model (and nulls) survive, and formats it like the repo files.
func mergeConfigJSON(original []byte, proposed supabase.ScrapingConfig) ([]byte, error) {
	var base map[string]any
	if err := json.Unmarshal(original, &base); err != nil {
		return nil, fmt.Errorf("parse original config: %w", err)
	}
	raw, err := json.Marshal(proposed)
	if err != nil {
		return nil, err
	}
	var overlay map[string]any
	if err := json.Unmarshal(raw, &overlay); err != nil {
		return nil, err
	}
	mergeInto(base, overlay)

	var out bytes.Buffer
	enc := json.NewEncoder(&out)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(base); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func isZero(v any) bool {
	switch x := v.(type) {
	case string:
		return x == ""
	case bool:
		return !x
	case nil:
		return true
	}
	return false
}

func mergeInto(base, overlay map[string]any) {
	for k, v := range overlay {
		if vm, ok := v.(map[string]any); ok {
			if bm, ok := base[k].(map[string]any); ok {
				mergeInto(bm, vm)
				continue
			}
		}
		// The struct can't express null or absence: don't turn an original null into "", or
		// add zero-valued keys the original file never had.
		if isZero(v) && base[k] == nil {
			continue
		}
		base[k] = v
	}
}
