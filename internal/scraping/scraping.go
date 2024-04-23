package scraping

import (
	"context"
	"fmt"
	"log"
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

func countLiftsAndRuns(ctx context.Context, config supabase.ScrapingConfig) (runsOpen, liftsOpen int, err error) {
	tctx, cancel := context.WithTimeout(ctx, time.Second)
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
				return 0, 0, runErr
			}
		}

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
		err := runChromeDP(tctx,
			chromedp.WaitReady(config.Terrain.LiftsOpenSelector),
			chromedp.Nodes(config.Terrain.LiftStatusSelector, &openLifts, chromedp.ByQueryAll),
			chromedp.Nodes(config.Terrain.RunStatusSelector, &openRuns, chromedp.ByQueryAll),
		)
		if err != nil {
			return 0, 0, nil
		}
	}
	return len(openRuns), len(openLifts), nil
}

func processTextAndConvertToInt(text string, propertyName string) (int, error) {
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
	if config.Conditions.WaitForSelector != "" {
		runChromeDP(ctx, chromedp.WaitReady(config.Conditions.WaitForSelector))
	}
	for _, node := range conditionsNodes {
		getTextFromNode(ctx, config.Conditions.SnowpackSelector, node, &snowpack)
		getTextFromNode(ctx, config.Conditions.SeasonTotalSelector, node, &seasonTotal)
		getTextFromNode(ctx, config.Conditions.Snow7DaySelector, node, &snow7Days)
		runChromeDP(ctx,
			chromedp.WaitEnabled(config.Conditions.BaseDepthSelector),
			chromedp.Text(config.Conditions.BaseDepthSelector, &baseDepthText, chromedp.ByQuery, chromedp.FromNode(node)),
			chromedp.Text(config.Conditions.Snow24Selector, &snow24Text, chromedp.ByQuery, chromedp.FromNode(node)),
			chromedp.Text(config.Conditions.Snow48Selector, &snow48Text, chromedp.ByQuery, chromedp.FromNode(node)),
		)
	}

	baseDepth, err = processTextAndConvertToInt(baseDepthText, "base depth")
	if err != nil {
		return 0, 0, 0, "", "", "", err
	}

	snow24, err = processTextAndConvertToInt(snow24Text, "no 24 hour snowfall data found")
	if err != nil {
		return 0, 0, 0, "", "", "", err
	}

	snow48, err = processTextAndConvertToInt(snow48Text, "no 48 hour snowfall data found")
	if err != nil {
		return 0, 0, 0, "", "", "", err
	}
	return baseDepth, snow24, snow48, snow7Days, seasonTotal, snowpack, nil
}

func processTerrain(ctx context.Context, config supabase.ScrapingConfig, terrainNodes []*cdp.Node) (runsOpen, liftsOpen int, err error) {
	var runsOpenText, liftsOpenText string
	tctx, cancel := context.WithTimeout(ctx, time.Second)
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

	runsOpen, err = convertStringToInt(runsOpenFormatted)
	liftsOpen, err = convertStringToInt(liftsOpenFormatted)

	return runsOpen, liftsOpen, err
}

func navigateToURL(ctx context.Context, url string) {
	runChromeDP(ctx, chromedp.EmulateViewport(1200, 1000), chromedp.Navigate(url))
}

func getConditionsNodes(ctx context.Context, config supabase.ScrapingConfig) []*cdp.Node {
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

func getTerrainData(ctx context.Context, config supabase.ScrapingConfig) (runsOpen, liftsOpen int, err error) {
	// Handles cases where the site requires a click to load the terrain data
	if config.ClickSelector != "" {
		clickProvidedSelector(ctx, config)
		terrainNodes := getTerrainNodes(ctx, config)
		runsOpen, liftsOpen, err = processTerrain(ctx, config, terrainNodes)
		return runsOpen, liftsOpen, err
	}

	// Handles cases where the terrain data is on a separate page
	if config.SeparateURLs {
		navigateToURL(ctx, config.TerrainURL)

		// Handles cases where an exact count of lifts and runs is not provided
		// and we need to count them ourselves
		if config.Terrain.CountLifts {
			runsOpen, liftsOpen, err = countLiftsAndRuns(ctx, config)
			return runsOpen, liftsOpen, err
		}

		terrainNodes := getTerrainNodes(ctx, config)
		runsOpen, liftsOpen, err = processTerrain(ctx, config, terrainNodes)
		return runsOpen, liftsOpen, err
	}

	// Handles cases where the terrain data is on the same page as the conditions data
	// but the exact count of lifts and runs is not provided
	if config.Terrain.CountLifts {
		runsOpen, liftsOpen, err = countLiftsAndRuns(ctx, config)
		return runsOpen, liftsOpen, err
	}

	// Handles cases where the terrain data is on the same page as the conditions data
	// with no special interactions required
	terrainNodes := getTerrainNodes(ctx, config)
	runsOpen, liftsOpen, err = processTerrain(ctx, config, terrainNodes)
	return runsOpen, liftsOpen, err
}

func ScrapeResortData(mountainName *string) (map[string]interface{}, error) {
	supabaseClient := supabase.NewSupabaseService()
	config := supabaseClient.GetConfigByName(*mountainName)

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
	runsOpen, liftsOpen, err := getTerrainData(ctx, config)
	if err != nil {
		return nil, err
	}

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
		result, err := convertStringToInt(seasonTotal)
		if err != nil {
			return nil, fmt.Errorf("failed to convert season total to int: %w", err)
		}
		resortConditions["snow_total"] = result
	}
	if config.Conditions.Snow7DaySelector != "" {
		result, err := convertStringToInt(snow7Days)
		if err != nil {
			return nil, fmt.Errorf("failed to convert 7 day snowfall to int: %w", err)
		}
		resortConditions["snow_past_week"] = result
	}

	return resortConditions, err
}
