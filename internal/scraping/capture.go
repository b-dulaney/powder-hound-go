package scraping

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"powderhoundgo/internal/supabase"

	"github.com/chromedp/chromedp"
)

// PageCapture is what the monitor sends to Claude about a rendered page.
type PageCapture struct {
	URL string
	// TrimmedHTML is the rendered DOM with scripts, styles, SVGs, images and most attributes
	// removed, so selectors can be read without the noise. Capped at maxTrimmedHTML.
	TrimmedHTML string
	// VisibleText is the page's rendered text (innerText), capped at maxVisibleText.
	VisibleText string
	// Screenshot is a JPEG of the top of the page at the scraper's viewport size.
	Screenshot []byte
}

const (
	maxTrimmedHTML = 400_000
	maxVisibleText = 60_000
	// Sites render their widgets after load; give them time before reading the DOM.
	renderWait = 8 * time.Second
)

// trimDOMScript returns the body's HTML without content that never matters for selectors.
// Kept attributes: id, class, aria-label, role, data-* (often stable hooks), href (truncated).
const trimDOMScript = `(() => {
  const root = document.body.cloneNode(true);
  root.querySelectorAll('script,style,noscript,svg,iframe,link,meta,template,video,audio,canvas,picture source')
    .forEach(n => n.remove());
  const keep = a => a === 'id' || a === 'class' || a === 'aria-label' || a === 'role' || a.startsWith('data-') || a === 'href';
  root.querySelectorAll('*').forEach(el => {
    for (const attr of [...el.attributes]) {
      if (!keep(attr.name)) { el.removeAttribute(attr.name); continue; }
      if (attr.value.length > 120) el.setAttribute(attr.name, attr.value.slice(0, 120) + '…');
    }
  });
  root.querySelectorAll('img').forEach(n => n.remove());
  return root.outerHTML.replace(/\s+/g, ' ').replace(/> </g, '><');
})()`

// CapturePage renders url in headless Chrome (same viewport as the scraper) and returns
// the trimmed DOM, visible text and a screenshot. clickSelector, when set, is clicked first,
// matching configs that need a click to reveal data.
func CapturePage(url, clickSelector string) (*PageCapture, error) {
	ctx, cancel := chromedp.NewContext(context.Background())
	defer cancel()
	ctx, cancel = context.WithTimeout(ctx, 90*time.Second)
	defer cancel()

	actions := []chromedp.Action{
		chromedp.EmulateViewport(1200, 1000),
		chromedp.Navigate(url),
		chromedp.Sleep(renderWait),
	}
	if clickSelector != "" {
		actions = append(actions, chromedp.Click(clickSelector, chromedp.ByQuery), chromedp.Sleep(2*time.Second))
	}

	capture := &PageCapture{URL: url}
	actions = append(actions,
		chromedp.Evaluate(trimDOMScript, &capture.TrimmedHTML),
		chromedp.Evaluate(`document.body.innerText`, &capture.VisibleText),
		chromedp.CaptureScreenshot(&capture.Screenshot),
	)
	if err := chromedp.Run(ctx, actions...); err != nil {
		return nil, fmt.Errorf("capture %s: %w", url, err)
	}

	capture.TrimmedHTML = truncate(capture.TrimmedHTML, maxTrimmedHTML)
	capture.VisibleText = truncate(capture.VisibleText, maxVisibleText)
	return capture, nil
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	log.Printf("capture: truncating %d chars to %d", len(s), max)
	return s[:max] + "\n<!-- truncated -->"
}

// highlight is one selector to outline on the proof screenshot. Container scopes the
// query the same way the scraper does (querySelector inside each container match).
type highlight struct {
	Label     string `json:"label"`
	Container string `json:"container"`
	Selector  string `json:"selector"`
}

const highlightScript = `((items) => {
  const found = {};
  for (const item of items) {
    const scopes = item.container ? [...document.querySelectorAll(item.container)] : [document];
    let el = null;
    for (const scope of scopes) { el = scope.querySelector(item.selector); if (el) break; }
    found[item.label] = el ? el.textContent.trim().slice(0, 80) : null;
    if (!el) continue;
    el.style.outline = '3px solid #e11d48';
    el.style.outlineOffset = '2px';
    const tag = document.createElement('span');
    tag.textContent = item.label;
    tag.style.cssText = 'position:absolute;z-index:2147483647;background:#e11d48;color:#fff;font:bold 12px sans-serif;padding:1px 4px;border-radius:3px;pointer-events:none';
    const r = el.getBoundingClientRect();
    tag.style.left = (r.left + window.scrollX) + 'px';
    tag.style.top = Math.max(0, r.top + window.scrollY - 18) + 'px';
    document.body.appendChild(tag);
  }
  return JSON.stringify(found);
})(%s)`

// ProofCapture is an annotated full-page screenshot plus the text each selector matched.
type ProofCapture struct {
	Screenshot []byte
	// Matched maps a field label to the text its selector matched, or nil if nothing matched.
	Matched map[string]*string
}

// CaptureProof outlines each of the config's selectors on the conditions page and takes a
// full-page JPEG, so a reviewer can see exactly which elements a proposed config reads.
func CaptureProof(config supabase.ScrapingConfig) (*ProofCapture, error) {
	items := []highlight{
		{"base depth", config.Conditions.ConditionsSelector, config.Conditions.BaseDepthSelector},
		{"24h", config.Conditions.ConditionsSelector, config.Conditions.Snow24Selector},
		{"48h", config.Conditions.ConditionsSelector, config.Conditions.Snow48Selector},
		{"season total", config.Conditions.ConditionsSelector, config.Conditions.SeasonTotalSelector},
		{"7 day", config.Conditions.ConditionsSelector, config.Conditions.Snow7DaySelector},
		{"snowpack", config.Conditions.ConditionsSelector, config.Conditions.SnowpackSelector},
	}
	// Terrain on a separate page isn't drawn here; the scrape result still checks it.
	if !config.SeparateURLs && !config.Terrain.CountLifts {
		items = append(items,
			highlight{"runs open", config.Terrain.TerrainSelector, config.Terrain.RunsOpenSelector},
			highlight{"lifts open", config.Terrain.TerrainSelector, config.Terrain.LiftsOpenSelector},
		)
	}
	var active []highlight
	for _, item := range items {
		if item.Selector != "" {
			active = append(active, item)
		}
	}
	itemsJSON, err := json.Marshal(active)
	if err != nil {
		return nil, err
	}

	ctx, cancel := chromedp.NewContext(context.Background())
	defer cancel()
	ctx, cancel = context.WithTimeout(ctx, 90*time.Second)
	defer cancel()

	actions := []chromedp.Action{
		chromedp.EmulateViewport(1200, 1000),
		chromedp.Navigate(config.ConditionsURL),
		chromedp.Sleep(renderWait),
	}
	if config.ClickSelector != "" {
		actions = append(actions, chromedp.Click(config.ClickSelector, chromedp.ByQuery), chromedp.Sleep(2*time.Second))
	}

	var matchedJSON string
	proof := &ProofCapture{}
	actions = append(actions,
		chromedp.Evaluate(fmt.Sprintf(highlightScript, itemsJSON), &matchedJSON),
		chromedp.FullScreenshot(&proof.Screenshot, 70),
	)
	if err := chromedp.Run(ctx, actions...); err != nil {
		return nil, fmt.Errorf("proof capture %s: %w", config.ConditionsURL, err)
	}
	if err := json.Unmarshal([]byte(matchedJSON), &proof.Matched); err != nil {
		return nil, fmt.Errorf("proof capture: parse matches: %w", err)
	}
	return proof, nil
}
