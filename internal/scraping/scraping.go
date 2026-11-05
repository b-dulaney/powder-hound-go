package scraping

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"powderhoundgo/internal/supabase"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/chromedp"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

func clickProvidedSelector(ctx context.Context, config supabase.ScrapingConfig) {
	runChromeDP(ctx,
		chromedp.WaitVisible(config.ClickSelector),
		chromedp.Click(config.ClickSelector),
	)
}

// terrainCounts is what a terrain scrape reports. A total of 0 means the page didn't give one.
type terrainCounts struct {
	RunsOpen, LiftsOpen, RunsTotal, LiftsTotal int
}

func countLiftsAndRuns(ctx context.Context, config supabase.ScrapingConfig) (terrainCounts, error) {
	tctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	var openLifts, openRuns []*cdp.Node
	if config.Terrain.RunClickInteraction {
		var buttonsToClick []*cdp.Node
		err := runChromeDP(tctx,
			chromedp.WaitReady(config.Terrain.LiftsOpenSelector),
			chromedp.Nodes(config.Terrain.LiftStatusSelector, &openLifts, chromedp.ByQueryAll),
			chromedp.Nodes(config.Terrain.RunClickSelector, &buttonsToClick, chromedp.ByQueryAll),
		)

		if err != nil {
			log.Printf("No open lift nodes found")
			runErr := runChromeDP(ctx,
				chromedp.Nodes(config.Terrain.RunClickSelector, &buttonsToClick, chromedp.ByQueryAll),
			)
			if runErr != nil {
				return terrainCounts{}, runErr
			}
		}

		if len(buttonsToClick) == 0 {
			log.Panic("No buttons provided to click")
		}
		for _, button := range buttonsToClick {
			runChromeDP(ctx, chromedp.Click(button.FullXPath()))
		}
		// Its own short timeout: with no runs open this waits for a match that never comes, and
		// on the scrape's context that used to consume the whole scrape timeout.
		rctx, rcancel := context.WithTimeout(ctx, 10*time.Second)
		runChromeDP(rctx,
			chromedp.Nodes(config.Terrain.RunStatusSelector, &openRuns, chromedp.ByQueryAll),
		)
		rcancel()
	} else {
		err := runChromeDP(tctx,
			chromedp.WaitReady(config.Terrain.LiftsOpenSelector),
			chromedp.Nodes(config.Terrain.LiftStatusSelector, &openLifts, chromedp.ByQueryAll),
			chromedp.Nodes(config.Terrain.RunStatusSelector, &openRuns, chromedp.ByQueryAll),
		)
		if err != nil {
			// Nothing open (the wait for an open lift or run timed out); totals still count.
			openLifts, openRuns = nil, nil
		}
	}
	return terrainCounts{RunsOpen: len(openRuns), LiftsOpen: len(openLifts)}, nil
}

// addTotals fills in totals the open counts didn't include, from the config's total or row
// selectors.
func addTotals(ctx context.Context, t supabase.TerrainConfig, c terrainCounts) terrainCounts {
	if c.RunsTotal == 0 {
		c.RunsTotal = pageTotal(ctx, t.RunTotalSelector, t.RunRowSelector)
	}
	if c.LiftsTotal == 0 {
		c.LiftsTotal = pageTotal(ctx, t.LiftTotalSelector, t.LiftRowSelector)
	}
	return c
}

// pageTotal reads a total from totalSelector's text, or counts rowSelector's matches. 0 means
// unknown.
func pageTotal(ctx context.Context, totalSelector, rowSelector string) int {
	if totalSelector != "" {
		cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		var text string
		if err := runChromeDP(cctx, chromedp.TextContent(totalSelector, &text, chromedp.ByQuery)); err != nil {
			return 0
		}
		if n := parseTotal(text); n > 0 {
			return n
		}
		n, _ := convertStringToInt(text)
		return n
	}
	return countNodes(ctx, rowSelector)
}

// countNodes returns how many elements match selector, or 0 when it's empty or nothing matches.
func countNodes(ctx context.Context, selector string) int {
	if selector == "" {
		return 0
	}
	cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var nodes []*cdp.Node
	if err := runChromeDP(cctx, chromedp.Nodes(selector, &nodes, chromedp.ByQueryAll, chromedp.AtLeast(0))); err != nil {
		return 0
	}
	return len(nodes)
}

func processTextAndConvertToInt(text string, propertyName string) (int, error) {
	log.Print("Converting text to int: ", text)
	log.Print("For property: ", propertyName)
	if text == "" {
		return 0, fmt.Errorf("no value found for %s", propertyName)
	}

	value, err := convertStringToInt(text)
	if err != nil {
		return 0, fmt.Errorf("failed to convert %s to int with value: %s", propertyName, text)
	}

	return value, nil
}

func processConditions(ctx context.Context, config supabase.ScrapingConfig, conditionsNodes []*cdp.Node) (baseDepth, snow24, snow48 int, snow7Days, seasonTotal, snowpack string, err error) {
	var baseDepthText, snow24Text, snow48Text string

	// Wait for the elements to be visible first
	if config.Conditions.WaitForSelector != "" {
		runChromeDP(ctx, chromedp.WaitVisible(config.Conditions.WaitForSelector))
	} else {
		runChromeDP(ctx, chromedp.WaitVisible(config.Conditions.BaseDepthSelector))
	}

	log.Printf("Conditions nodes received, processing %d nodes", len(conditionsNodes))

	// Wait for actual data to be hydrated with retries
	// Check within the node context to match the extraction context
	maxRetries := 10
	retryDelay := 500 * time.Millisecond

	for _, node := range conditionsNodes {
		log.Printf("Waiting for data to be hydrated in base depth selector...")
		dataFound := false

		for i := 0; i < maxRetries; i++ {
			var testText string
			err := chromedp.Run(ctx,
				chromedp.Text(config.Conditions.BaseDepthSelector, &testText, chromedp.ByQuery, chromedp.FromNode(node)),
			)

			trimmedText := strings.TrimSpace(testText)
			// Check that the text contains at least one digit (actual data)
			hasDigit := false
			for _, char := range trimmedText {
				if char >= '0' && char <= '9' {
					hasDigit = true
					break
				}
			}

			if err == nil && hasDigit && trimmedText != "--" && trimmedText != "—" {
				log.Printf("Data detected after %d attempts: '%s'", i+1, trimmedText)
				dataFound = true
				break
			}

			if i < maxRetries-1 {
				log.Printf("Attempt %d: No numeric data yet (text: '%s'), retrying in %v...", i+1, trimmedText, retryDelay)
				time.Sleep(retryDelay)
			} else {
				log.Printf("Warning: Max retries reached without finding numeric data, proceeding anyway")
			}
		}

		if !dataFound {
			log.Printf("Warning: Proceeding without confirmed data hydration")
		}

		getTextFromNode(ctx, config.Conditions.SnowpackSelector, node, &snowpack)
		getTextFromNode(ctx, config.Conditions.SeasonTotalSelector, node, &seasonTotal)
		getTextFromNode(ctx, config.Conditions.Snow7DaySelector, node, &snow7Days)
		runChromeDP(ctx,
			chromedp.Text(config.Conditions.BaseDepthSelector, &baseDepthText, chromedp.ByQuery, chromedp.FromNode(node)),
			chromedp.Text(config.Conditions.Snow24Selector, &snow24Text, chromedp.ByQuery, chromedp.FromNode(node)),
			chromedp.Text(config.Conditions.Snow48Selector, &snow48Text, chromedp.ByQuery, chromedp.FromNode(node)),
		)
	}

	log.Printf("Node processing completed.")

	baseDepth, err = processTextAndConvertToInt(baseDepthText, "base depth")
	log.Printf("Base Depth: %d", baseDepth)
	if err != nil {
		return 0, 0, 0, "", "", "", err
	}

	snow24, err = processTextAndConvertToInt(snow24Text, "snow past 24 hours")
	if err != nil {
		return 0, 0, 0, "", "", "", err
	}

	snow48, err = processTextAndConvertToInt(snow48Text, "snow past 48 hours")
	if err != nil {
		return 0, 0, 0, "", "", "", err
	}

	log.Printf("Base Depth: %d, Snow 24: %d, Snow 48: %d", baseDepth, snow24, snow48)
	return baseDepth, snow24, snow48, snow7Days, seasonTotal, snowpack, nil
}

func processTerrain(ctx context.Context, config supabase.ScrapingConfig, terrainNodes []*cdp.Node) (counts terrainCounts, err error) {
	var runsOpenText, liftsOpenText string
	tctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	for _, node := range terrainNodes {
		err := runChromeDP(tctx,
			chromedp.TextContent(config.Terrain.RunsOpenSelector, &runsOpenText, chromedp.ByQuery, chromedp.FromNode(node)),
			chromedp.TextContent(config.Terrain.LiftsOpenSelector, &liftsOpenText, chromedp.ByQuery, chromedp.FromNode(node)),
		)
		if err != nil {
			log.Printf("Error getting terrain data: %v", err)
			liftsOpenText = "0"
		}
	}
	runsOpenFormatted, err := removeDenominator(runsOpenText)
	liftsOpenFormatted, err := removeDenominator(liftsOpenText)

	counts.RunsOpen, err = convertStringToInt(runsOpenFormatted)
	counts.LiftsOpen, err = convertStringToInt(liftsOpenFormatted)
	counts.RunsTotal = parseTotal(runsOpenText)
	counts.LiftsTotal = parseTotal(liftsOpenText)

	return counts, err
}

func navigateToURL(ctx context.Context, url string) {
	runChromeDP(ctx, chromedp.EmulateViewport(1200, 1000), chromedp.Navigate(url))
}

func getConditionsNodes(ctx context.Context, config supabase.ScrapingConfig) []*cdp.Node {
	log.Print("Getting conditions nodes")
	var conditionsNodes []*cdp.Node
	runChromeDP(ctx,
		chromedp.WaitVisible(config.Conditions.ConditionsSelector),
		chromedp.Nodes(config.Conditions.ConditionsSelector, &conditionsNodes, chromedp.ByQueryAll),
	)
	return conditionsNodes
}

func getTerrainNodes(ctx context.Context, config supabase.ScrapingConfig) []*cdp.Node {
	var terrainNodes []*cdp.Node
	runChromeDP(ctx,
		chromedp.WaitVisible(config.Terrain.TerrainSelector),
		chromedp.Nodes(config.Terrain.TerrainSelector, &terrainNodes, chromedp.ByQueryAll),
	)
	return terrainNodes
}

func getTerrainData(ctx context.Context, config supabase.ScrapingConfig) (terrainCounts, error) {
	// Handles cases where the site requires a click to load the terrain data
	if config.ClickSelector != "" {
		clickProvidedSelector(ctx, config)
		terrainNodes := getTerrainNodes(ctx, config)
		return processTerrain(ctx, config, terrainNodes)
	}

	// Handles cases where the terrain data is on a separate page
	if config.SeparateURLs {
		navigateToURL(ctx, config.TerrainURL)

		// Handles cases where an exact count of lifts and runs is not provided
		// and we need to count them ourselves
		if config.Terrain.CountLifts {
			return countLiftsAndRuns(ctx, config)
		}

		terrainNodes := getTerrainNodes(ctx, config)
		return processTerrain(ctx, config, terrainNodes)
	}

	// Handles cases where the terrain data is on the same page as the conditions data
	// but the exact count of lifts and runs is not provided
	if config.Terrain.CountLifts {
		return countLiftsAndRuns(ctx, config)
	}

	// Handles cases where the terrain data is on the same page as the conditions data
	// with no special interactions required
	terrainNodes := getTerrainNodes(ctx, config)
	return processTerrain(ctx, config, terrainNodes)
}

func ScrapeResortData(mountainName *string) (map[string]interface{}, error) {
	supabaseClient := supabase.NewSupabaseService()
	config := supabaseClient.GetConfigByName(*mountainName)
	return ScrapeWithConfig(config)
}

// ScrapeWithConfig scrapes a resort with the given config instead of the stored one.
// The monitor uses it to test a proposed config with exactly the production scrape path.
func ScrapeWithConfig(config supabase.ScrapingConfig) (map[string]interface{}, error) {
	resortConditions := map[string]interface{}{
		"mountain_id":   config.ID,
		"display_name":  config.Name,
		"base_depth":    0,
		"snow_past_24h": 0,
		"snow_past_48h": 0,
		"lifts_open":    0,
		"runs_open":     0,
		"updated_at":    time.Now(),
	}

	ctx, cancel := chromedp.NewContext(context.Background(), chromedp.WithLogf(log.Printf))

	defer cancel()

	ctx, cancel = context.WithTimeout(ctx, 180*time.Second)
	defer cancel()

	navigateToURL(ctx, config.ConditionsURL)
	conditionsNodes := getConditionsNodes(ctx, config)
	baseDepth, snow24, snow48, snow7Days, seasonTotal, snowpack, err := processConditions(ctx, config, conditionsNodes)
	if err != nil {
		return nil, err
	}
	terrain, err := getTerrainData(ctx, config)
	if err != nil {
		return nil, err
	}
	terrain = addTotals(ctx, config.Terrain, terrain)

	resortConditions["base_depth"] = baseDepth
	resortConditions["snow_past_24h"] = snow24
	resortConditions["snow_past_48h"] = snow48
	resortConditions["runs_open"] = terrain.RunsOpen
	resortConditions["lifts_open"] = terrain.LiftsOpen
	// Totals only when the page gives them; the database keeps the last plausible value
	// (see the resort_conditions totals trigger in powder-hound).
	if terrain.RunsTotal > 0 {
		resortConditions["total_runs"] = terrain.RunsTotal
	}
	if terrain.LiftsTotal > 0 {
		resortConditions["total_lifts"] = terrain.LiftsTotal
	}

	if config.Conditions.SnowpackSelector != "" {
		lowercaseSnowpack := cases.Lower(language.English, cases.Compact).String(snowpack)
		formattedSnowpack := cases.Title(language.English, cases.Compact).String(lowercaseSnowpack)
		resortConditions["snow_type"] = formattedSnowpack
	}
	if config.Conditions.SeasonTotalSelector != "" {
		result, err := convertStringToInt(seasonTotal)
		if err != nil {
			log.Printf("Warning: failed to convert season total to int: %v, using 0", err)
			resortConditions["snow_total"] = 0
		} else {
			resortConditions["snow_total"] = result
		}
	}
	if config.Conditions.Snow7DaySelector != "" {
		result, err := convertStringToInt(snow7Days)
		if err != nil {
			log.Printf("Warning: failed to convert 7 day snowfall to int: %v, using 0", err)
			resortConditions["snow_past_week"] = 0
		} else {
			resortConditions["snow_past_week"] = result
		}
	}

	return resortConditions, err
}
