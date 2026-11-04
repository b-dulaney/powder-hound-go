package monitor

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"powderhoundgo/internal/scraping"
	"powderhoundgo/internal/supabase"

	"github.com/anthropics/anthropic-sdk-go/v2"
	"github.com/anthropics/anthropic-sdk-go/v2/option"
)

const defaultModel = "claude-opus-5-5"

// Claude wraps the two model calls the monitor makes. Both use structured outputs, so the
// response is parsed straight into a Go struct.
type Claude struct {
	client anthropic.Client
	model  string
}

func NewClaude() (*Claude, error) {
	if os.Getenv("ANTHROPIC_API_KEY") == "" {
		return nil, errors.New("ANTHROPIC_API_KEY is not set")
	}
	model := os.Getenv("MONITOR_MODEL")
	if model == "" {
		model = defaultModel
	}
	return &Claude{
		client: anthropic.NewClient(option.WithMaxRetries(3)),
		model:  model,
	}, nil
}

// ExpectedValues are the numbers Claude reads off the page; -1 means the page doesn't show it.
type ExpectedValues struct {
	BaseDepth   int `json:"base_depth" jsonschema_description:"Base depth in inches, or -1 if not shown"`
	Snow24h     int `json:"snow_24h" jsonschema_description:"Snow in the past 24 hours in inches, or -1 if not shown"`
	Snow48h     int `json:"snow_48h" jsonschema_description:"Snow in the past 48 hours in inches, or -1 if not shown"`
	SeasonTotal int `json:"season_total" jsonschema_description:"Season snowfall total in inches, or -1 if not shown"`
	RunsOpen    int `json:"runs_open" jsonschema_description:"Number of runs/trails open, or -1 if not shown"`
	LiftsOpen   int `json:"lifts_open" jsonschema_description:"Number of lifts open, or -1 if not shown"`
}

type FieldEvidence struct {
	Field string `json:"field" jsonschema_description:"One of base_depth, snow_24h, snow_48h, season_total, runs_open, lifts_open"`
	Quote string `json:"quote" jsonschema_description:"The exact page text (with its label) that shows this value"`
}

// ConfigProposal is Claude's proposed replacement scraper config.
type ConfigProposal struct {
	Config   supabase.ScrapingConfig `json:"config"`
	Expected ExpectedValues          `json:"expected"`
	Evidence []FieldEvidence         `json:"evidence"`
	Summary  string                  `json:"summary" jsonschema_description:"One or two sentences: what changed on the site and what the new selectors target"`
	Caveats  string                  `json:"caveats" jsonschema_description:"Anything a reviewer should double-check, such as ambiguous labels; empty if none"`
}

// StatusReading is Claude's read of whether a resort is operating.
type StatusReading struct {
	Status            string `json:"status" jsonschema:"enum=open,enum=closed,enum=unknown"`
	Confidence        string `json:"confidence" jsonschema:"enum=high,enum=medium,enum=low"`
	Quote             string `json:"quote" jsonschema_description:"Exact page text that supports the status; empty if there is none"`
	ExpectedOpenDate  string `json:"expected_open_date" jsonschema_description:"Announced opening date as YYYY-MM-DD, or empty"`
	ExpectedCloseDate string `json:"expected_close_date" jsonschema_description:"Announced closing date as YYYY-MM-DD, or empty"`
	Reasoning         string `json:"reasoning" jsonschema_description:"One or two sentences on how the evidence supports the status"`
}

const configSystemPrompt = `You maintain web-scraping configs for Powder Hound, a ski conditions site. A Go scraper uses headless Chrome (chromedp, 1200x1000 viewport) to read each resort's snow report with CSS selectors from a JSON config. A resort's site changed and the current config no longer works. Propose a corrected config.

How the scraper uses the config:
- It navigates to conditionsURL and waits until conditions.waitForSelector (or, if empty, baseDepthSelector) is visible.
- It finds every element matching conditions.conditionsSelector (the container). baseDepthSelector, snow24Selector, snow48Selector, seasonTotalSelector, snow7DaySelector and snowpackSelector are each queried INSIDE that container with querySelector, and their text is read. Numbers are extracted by dropping everything after the first "." and removing non-digits, so "18\" Base" reads as 18 and "--" reads as 0.
- baseDepthSelector, snow24Selector and snow48Selector are required. seasonTotalSelector, snow7DaySelector and snowpackSelector may be "" when the page doesn't show that value.
- Terrain: unless separateURLs is true (then it navigates to terrainURL first) or terrain.countLifts is true, it finds terrain.terrainSelector containers and reads runsOpenSelector and liftsOpenSelector inside them with textContent. "1 / 147" or "1 of 147" reads as 1.
- clickSelector, when set, is clicked before reading terrain.

Selector rules:
- Prefer stable hooks: ids, data-* attributes, semantic structure, and the stable part of generated class names. Many sites use CSS modules with build hashes like "StatsWidget_statBig__JTduy"; the hash changes on every deploy, so match the prefix with [class*="StatsWidget_statBig"], never the full hashed name.
- Each selector must match exactly the element holding the value, scoped inside its container.
- Chrome supports :has(), :nth-child() and attribute selectors.
- Keep id, name, closingDate and any fields you don't need to change exactly as they are in the current config. Keep URLs unless the data moved to a different page.

Report in "expected" the values you read on the page for each field, and quote the page text for each in "evidence". These are checked against an actual scrape with your config, so read them carefully from the page, not from the config.`

const statusSystemPrompt = `You check whether ski resorts are operating for Powder Hound, a ski conditions site. You are given a resort's snow report page (rendered text and a screenshot) and the scraper's latest lift counts. Decide the resort's operating status:
- "open": the resort is operating this season (open today, or in season with a temporary closure such as wind holds or a midweek closure).
- "closed": closed for the season (not yet opened, or closed for the season after spring).
- "unknown": the page doesn't say clearly.
Quote the exact page text your answer rests on. Report announced opening or closing dates if the page gives them. Use "high" confidence only when the page states the status explicitly.`

// ProposeConfig asks Claude for a corrected config. previous and feedback are set on a retry,
// after a proposal failed verification.
func (c *Claude) ProposeConfig(ctx context.Context, current supabase.ScrapingConfig, page *scraping.PageCapture, scrapeErr string, previous *ConfigProposal, feedback string) (*ConfigProposal, error) {
	currentJSON, err := json.MarshalIndent(current, "", "  ")
	if err != nil {
		return nil, err
	}
	user := fmt.Sprintf("Current config:\n```json\n%s\n```\n\nThe scrape with this config failed or produced bad data: %s\n\nRendered DOM of %s (scripts, styles, SVGs and most attributes removed):\n```html\n%s\n```",
		currentJSON, scrapeErr, page.URL, page.TrimmedHTML)

	// A retry is a fresh single-turn request that includes the failed attempt, rather than a
	// replayed conversation, so no earlier thinking blocks need to be carried along.
	if previous != nil {
		previousJSON, err := json.MarshalIndent(previous.Config, "", "  ")
		if err != nil {
			return nil, err
		}
		user += fmt.Sprintf("\n\nA previous attempt proposed this config:\n```json\n%s\n```\nScraping with it did not match the page:\n%s\nPropose a config that fixes this.", previousJSON, feedback)
	}
	messages := []anthropic.BetaMessageParam{
		anthropic.NewBetaUserMessage(
			anthropic.NewBetaImageBlock(anthropic.BetaBase64ImageSourceParam{
				Data:      base64.StdEncoding.EncodeToString(page.Screenshot),
				MediaType: anthropic.BetaBase64ImageSourceMediaTypeImagePNG,
			}),
			anthropic.NewBetaTextBlock(user),
		),
	}

	var proposal ConfigProposal
	if err := c.call(ctx, configSystemPrompt, messages, &proposal, anthropic.BetaOutputConfigEffortHigh); err != nil {
		return nil, err
	}
	return &proposal, nil
}

// ReadStatus asks Claude whether the resort on this page is operating.
func (c *Claude) ReadStatus(ctx context.Context, resortName string, page *scraping.PageCapture, liftsOpen, totalLifts string) (*StatusReading, error) {
	user := fmt.Sprintf("Resort: %s\nScraper's latest lift count: %s open of %s\nPage: %s\n\nRendered page text:\n<page_text>\n%s\n</page_text>",
		resortName, liftsOpen, totalLifts, page.URL, page.VisibleText)
	messages := []anthropic.BetaMessageParam{
		anthropic.NewBetaUserMessage(
			anthropic.NewBetaImageBlock(anthropic.BetaBase64ImageSourceParam{
				Data:      base64.StdEncoding.EncodeToString(page.Screenshot),
				MediaType: anthropic.BetaBase64ImageSourceMediaTypeImagePNG,
			}),
			anthropic.NewBetaTextBlock(user),
		),
	}
	var reading StatusReading
	if err := c.call(ctx, statusSystemPrompt, messages, &reading, anthropic.BetaOutputConfigEffortLow); err != nil {
		return nil, err
	}
	return &reading, nil
}

func (c *Claude) call(ctx context.Context, system string, messages []anthropic.BetaMessageParam, dest any, effort anthropic.BetaOutputConfigEffort) error {
	resp, err := c.client.Beta.Messages.New(ctx, anthropic.BetaMessageNewParams{
		Model:     c.model,
		MaxTokens: 16000,
		// Static instructions first, cached; the page content varies per call.
		System: []anthropic.BetaTextBlockParam{{
			Text:         system,
			CacheControl: anthropic.NewBetaCacheControlEphemeralParam(),
		}},
		Messages: messages,
		OutputConfig: anthropic.BetaOutputConfigParam{
			Effort: effort,
			Format: anthropic.BetaJSONOutputFormatParam{Schema: dest},
		},
		// If a safety classifier declines, let the API re-serve the request on its default fallback.
		Fallbacks: anthropic.BetaFallbacksParamOfDefault(),
		Betas:     []anthropic.AnthropicBeta{anthropic.AnthropicBetaServerSideFallback2026_07_01},
	})
	if err != nil {
		var apiErr *anthropic.Error
		if errors.As(err, &apiErr) {
			return fmt.Errorf("claude API error %d: %w", apiErr.StatusCode, err)
		}
		return fmt.Errorf("claude request: %w", err)
	}
	switch resp.StopReason {
	case anthropic.BetaStopReasonRefusal:
		return fmt.Errorf("claude declined the request (%s)", resp.StopDetails.Category)
	case anthropic.BetaStopReasonMaxTokens:
		return errors.New("claude response hit max_tokens")
	}
	return nil
}
