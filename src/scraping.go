package main

import (
	"context"
	"log"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/chromedp"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

func clickProvidedSelector(ctx context.Context, config Config) {
	runChromeDP(ctx,
		chromedp.WaitVisible(config.ClickSelector),
		chromedp.Click(config.ClickSelector),
	)
}

func countLiftsAndRuns(ctx context.Context, config Config) (int, int) {
	var openLifts, openRuns []*cdp.Node
	if config.Terrain.RunClickInteraction {
		var buttonsToClick []*cdp.Node
		runChromeDP(ctx,
			chromedp.WaitReady(config.Terrain.LiftsOpenSelector),
			chromedp.Nodes(config.Terrain.LiftStatusSelector, &openLifts, chromedp.ByQueryAll),
			chromedp.Nodes(config.Terrain.RunClickSelector, &buttonsToClick, chromedp.ByQueryAll),
		)
		if len(buttonsToClick) == 0 {
			log.Panic("No buttons provided to click")
		}
		for _, button := range buttonsToClick {
			runChromeDP(ctx, chromedp.Click(button.FullXPath()))
		}
		runChromeDP(ctx,
			chromedp.Nodes(config.Terrain.RunStatusSelector, &openRuns, chromedp.ByQueryAll),
		)
	} else {
		runChromeDP(ctx,
			chromedp.WaitReady(config.Terrain.LiftsOpenSelector),
			chromedp.Nodes(config.Terrain.LiftStatusSelector, &openLifts, chromedp.ByQueryAll),
			chromedp.Nodes(config.Terrain.RunStatusSelector, &openRuns, chromedp.ByQueryAll),
		)
	}
	return len(openLifts), len(openRuns)
}

func processConditions(ctx context.Context, config Config, conditionsNodes []*cdp.Node) (baseDepth, snow24, snow48 int, snow7Days, seasonTotal, snowpack string) {
	var baseDepthText, snow24Text, snow48Text string
	for _, node := range conditionsNodes {
		getTextFromNode(ctx, config.Conditions.SnowpackSelector, node, &snowpack)
		getTextFromNode(ctx, config.Conditions.SeasonTotalSelector, node, &seasonTotal)
		getTextFromNode(ctx, config.Conditions.Snow7DaySelector, node, &snow7Days)
		runChromeDP(ctx,
			chromedp.Text(config.Conditions.BaseDepthSelector, &baseDepthText, chromedp.ByQuery, chromedp.FromNode(node)),
			chromedp.Text(config.Conditions.Snow24Selector, &snow24Text, chromedp.ByQuery, chromedp.FromNode(node)),
			chromedp.Text(config.Conditions.Snow48Selector, &snow48Text, chromedp.ByQuery, chromedp.FromNode(node)),
		)
	}
	var baseDepthInt, snow24Int, snow48Int int = convertStringToInt(baseDepthText), convertStringToInt(snow24Text), convertStringToInt(snow48Text)
	return baseDepthInt, snow24Int, snow48Int, snow7Days, seasonTotal, snowpack
}

func processTerrain(ctx context.Context, config Config, terrainNodes []*cdp.Node) (runsOpen, liftsOpen int) {
	var runsOpenText, liftsOpenText string
	for _, node := range terrainNodes {
		runChromeDP(ctx,
			chromedp.TextContent(config.Terrain.RunsOpenSelector, &runsOpenText, chromedp.ByQuery, chromedp.FromNode(node)),
			chromedp.TextContent(config.Terrain.LiftsOpenSelector, &liftsOpenText, chromedp.ByQuery, chromedp.FromNode(node)),
		)
	}
	runsOpen, liftsOpen = convertStringToInt(removeDenominator(runsOpenText)), convertStringToInt(removeDenominator(liftsOpenText))
	return
}

func navigateToURL(ctx context.Context, url string) {
	runChromeDP(ctx, chromedp.EmulateViewport(1200, 1000), chromedp.Navigate(url))
	log.Printf("Visiting %s", url)
}

func getConditionsNodes(ctx context.Context, config Config) []*cdp.Node {
	var conditionsNodes []*cdp.Node
	runChromeDP(ctx,
		chromedp.WaitVisible(config.Conditions.ConditionsSelector),
		chromedp.Nodes(config.Conditions.ConditionsSelector, &conditionsNodes, chromedp.ByQueryAll),
	)
	return conditionsNodes
}

func getTerrainNodes(ctx context.Context, config Config) []*cdp.Node {
	var terrainNodes []*cdp.Node
	runChromeDP(ctx,
		chromedp.WaitVisible(config.Terrain.TerrainSelector),
		chromedp.Nodes(config.Terrain.TerrainSelector, &terrainNodes, chromedp.ByQueryAll),
	)
	return terrainNodes
}

func getTerrainData(ctx context.Context, config Config) (runsOpen, liftsOpen int) {
	// Handles cases where the site requires a click to load the terrain data
	if config.ClickSelector != "" {
		clickProvidedSelector(ctx, config)
		terrainNodes := getTerrainNodes(ctx, config)
		liftsOpen, runsOpen = processTerrain(ctx, config, terrainNodes)
		return liftsOpen, runsOpen
	}

	// Handles cases where the terrain data is on a separate page
	if config.SeparateURLs {
		navigateToURL(ctx, config.TerrainURL)

		// Handles cases where an exact count of lifts and runs is not provided
		// and we need to count them ourselves
		if config.Terrain.CountLifts {
			liftsOpen, runsOpen = countLiftsAndRuns(ctx, config)
			return liftsOpen, runsOpen
		}

		terrainNodes := getTerrainNodes(ctx, config)
		liftsOpen, runsOpen = processTerrain(ctx, config, terrainNodes)
		return liftsOpen, runsOpen
	}

	// Handles cases where the terrain data is on the same page as the conditions data
	// but the exact count of lifts and runs is not provided
	if config.Terrain.CountLifts {
		liftsOpen, runsOpen = countLiftsAndRuns(ctx, config)
		return runsOpen, liftsOpen
	}

	// Handles cases where the terrain data is on the same page as the conditions data
	// with no special interactions required
	terrainNodes := getTerrainNodes(ctx, config)
	runsOpen, liftsOpen = processTerrain(ctx, config, terrainNodes)
	return runsOpen, liftsOpen
}

func scrapeResortData(configPath *string) (success bool) {
	supabase := initializeSupabase()

	config := fetchConfig(configPath)

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

	ctx, cancel = context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	navigateToURL(ctx, config.ConditionsURL)
	conditionsNodes := getConditionsNodes(ctx, config)
	baseDepth, snow24, snow48, snow7Days, seasonTotal, snowpack := processConditions(ctx, config, conditionsNodes)
	runsOpen, liftsOpen := getTerrainData(ctx, config)

	resortConditions["base_depth"] = baseDepth
	resortConditions["snow_past_24h"] = snow24
	resortConditions["snow_past_48h"] = snow48
	resortConditions["runs_open"] = runsOpen
	resortConditions["lifts_open"] = liftsOpen

	if config.Conditions.SnowpackSelector != "" {
		lowercaseSnowpack := cases.Lower(language.English, cases.Compact).String(snowpack)
		formattedSnowpack := cases.Title(language.English, cases.Compact).String(lowercaseSnowpack)
		resortConditions["snow_type"] = formattedSnowpack
	}
	if config.Conditions.SeasonTotalSelector != "" {
		resortConditions["snow_total"] = convertStringToInt(seasonTotal)
	}
	if config.Conditions.Snow7DaySelector != "" {
		resortConditions["snow_past_week"] = convertStringToInt(snow7Days)
	}

	err := upsertSupabaseData(supabase, resortConditions)
	return err == nil
}
