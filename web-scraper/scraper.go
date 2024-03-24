package main

import (
	"context"
	"flag"
	"log"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/chromedp"
)

func runChromeDP(ctx context.Context, tasks ...chromedp.Action) {
	if err := chromedp.Run(ctx, tasks...); err != nil {
		log.Fatal(err)
	}
}

func getTextFromNode(ctx context.Context, selector string, node *cdp.Node, result *string) {
	if selector != "" {
		runChromeDP(ctx, chromedp.Text(selector, result, chromedp.ByQuery, chromedp.FromNode(node)))
	}
}

func processConditionsForSeparateURLs(ctx context.Context, config Config, conditionsNodes []*cdp.Node) (baseDepth, snowpack, seasonTotal, snow24, snow48, snow7Days string) {
	for _, node := range conditionsNodes {
		getTextFromNode(ctx, config.Conditions.SnowpackSelector, node, &snowpack)
		getTextFromNode(ctx, config.Conditions.SeasonTotalSelector, node, &seasonTotal)
		getTextFromNode(ctx, config.Conditions.Snow7DaySelector, node, &snow7Days)
		runChromeDP(ctx,
			chromedp.Text(config.Conditions.BaseDepthSelector, &baseDepth, chromedp.ByQuery, chromedp.FromNode(node)),
			chromedp.Text(config.Conditions.Snow24Selector, &snow24, chromedp.ByQuery, chromedp.FromNode(node)),
			chromedp.Text(config.Conditions.Snow48Selector, &snow48, chromedp.ByQuery, chromedp.FromNode(node)),
			chromedp.Navigate(config.TerrainURL),
			chromedp.WaitVisible(config.Terrain.LiftsOpenSelector),
		)
	}
	return
}

func processTerrainForSeparateURLs(ctx context.Context, config Config, terrainNodes []*cdp.Node) (runsOpen, liftsOpen string) {
	for _, node := range terrainNodes {
		runChromeDP(ctx,
			chromedp.TextContent(config.Terrain.RunsOpenSelector, &runsOpen, chromedp.ByQuery, chromedp.FromNode(node)),
			chromedp.TextContent(config.Terrain.LiftsOpenSelector, &liftsOpen, chromedp.ByQuery, chromedp.FromNode(node)),
		)
	}
	return
}

func processConditions(ctx context.Context, config Config, conditionsNodes []*cdp.Node) (baseDepth, snowpack, seasonTotal, snow24, snow48, snow7Days string) {
	for _, node := range conditionsNodes {
		getTextFromNode(ctx, config.Conditions.SnowpackSelector, node, &snowpack)
		getTextFromNode(ctx, config.Conditions.SeasonTotalSelector, node, &seasonTotal)
		getTextFromNode(ctx, config.Conditions.Snow7DaySelector, node, &snow7Days)
		runChromeDP(ctx,
			chromedp.Text(config.Conditions.BaseDepthSelector, &baseDepth, chromedp.ByQuery, chromedp.FromNode(node)),
			chromedp.Text(config.Conditions.Snow24Selector, &snow24, chromedp.ByQuery, chromedp.FromNode(node)),
			chromedp.Text(config.Conditions.Snow48Selector, &snow48, chromedp.ByQuery, chromedp.FromNode(node)),
		)
	}
	return
}

func processTerrain(ctx context.Context, config Config, terrainNodes []*cdp.Node) (runsOpen, liftsOpen string) {
	for _, node := range terrainNodes {
		runChromeDP(ctx,
			chromedp.TextContent(config.Terrain.RunsOpenSelector, &runsOpen, chromedp.ByQuery, chromedp.FromNode(node)),
			chromedp.TextContent(config.Terrain.LiftsOpenSelector, &liftsOpen, chromedp.ByQuery, chromedp.FromNode(node)),
		)
	}
	return
}

func main() {
	configPath := flag.String("c", "./scraper-configs/a-basin.json", "Path to config file")
	flag.Parse()

	supabase := initializeSupabase()

	config := fetchConfig(configPath)

	resortConditions := map[string]interface{}{
		"id":         config.ID,
		"name":       config.Name,
		"baseDepth":  0,
		"snow24":     0,
		"snow48":     0,
		"liftsOpen":  0,
		"runsOpen":   0,
		"updated_at": time.Now(),
	}

	ctx, cancel := chromedp.NewContext(context.Background(), chromedp.WithLogf(log.Printf))
	defer cancel()

	ctx, cancel = context.WithTimeout(ctx, 30*time.Second)

	var conditionsNodes []*cdp.Node
	var terrainNodes []*cdp.Node

	log.Printf("Visiting %s", config.ConditionsURL)

	if config.SeparateURLs {
		runChromeDP(ctx,
			chromedp.EmulateViewport(1200, 1000),
			chromedp.Navigate(config.ConditionsURL),
			chromedp.WaitVisible(config.Conditions.BaseDepthSelector),
			chromedp.Nodes(config.Conditions.ConditionsSelector, &conditionsNodes, chromedp.ByQueryAll),
		)

		baseDepth, snowpack, seasonTotal, snow24, snow48, snow7Days := processConditionsForSeparateURLs(ctx, config, conditionsNodes)
		runsOpen, liftsOpen := processTerrainForSeparateURLs(ctx, config, terrainNodes)

		resortConditions["baseDepth"] = convertStringToInt(baseDepth)
		resortConditions["snow24"] = convertStringToInt(snow24)
		resortConditions["snow48"] = convertStringToInt(snow48)
		resortConditions["runsOpen"] = convertStringToInt(removeDenominator(runsOpen))
		resortConditions["liftsOpen"] = convertStringToInt(removeDenominator(liftsOpen))

		if config.Conditions.SnowpackSelector != "" {
			resortConditions["snowpack"] = snowpack
		}
		if config.Conditions.SeasonTotalSelector != "" {
			resortConditions["seasonTotal"] = convertStringToInt(seasonTotal)
		}
		if config.Conditions.Snow7DaySelector != "" {
			resortConditions["snow7Days"] = convertStringToInt(snow7Days)
		}

		_, _, insertErr := supabase.From("golang-test").Insert(resortConditions, true, "id", "test", "1").Execute()
		if insertErr != nil {
			log.Printf("Error: %v", insertErr)
		}
		log.Printf("%s updated successfully", config.Name)

	} else {
		runChromeDP(ctx,
			chromedp.EmulateViewport(1200, 1000),
			chromedp.Navigate(config.ConditionsURL),
			chromedp.WaitVisible(config.Conditions.BaseDepthSelector),
			chromedp.WaitVisible(config.Terrain.LiftsOpenSelector),
			chromedp.Nodes(config.Conditions.ConditionsSelector, &conditionsNodes, chromedp.ByQueryAll),
			chromedp.Nodes(config.Terrain.TerrainSelector, &terrainNodes, chromedp.ByQueryAll),
		)

		baseDepth, snowpack, seasonTotal, snow24, snow48, snow7Days := processConditions(ctx, config, conditionsNodes)
		runsOpen, liftsOpen := processTerrain(ctx, config, terrainNodes)

		resortConditions["baseDepth"] = convertStringToInt(baseDepth)
		resortConditions["snow24"] = convertStringToInt(snow24)
		resortConditions["snow48"] = convertStringToInt(snow48)
		resortConditions["runsOpen"] = convertStringToInt(removeDenominator(runsOpen))
		resortConditions["liftsOpen"] = convertStringToInt(removeDenominator(liftsOpen))

		if config.Conditions.SnowpackSelector != "" {
			resortConditions["snowpack"] = snowpack
		}
		if config.Conditions.SeasonTotalSelector != "" {
			resortConditions["seasonTotal"] = convertStringToInt(seasonTotal)
		}
		if config.Conditions.Snow7DaySelector != "" {
			resortConditions["snow7Days"] = convertStringToInt(snow7Days)
		}

		_, _, insertErr := supabase.From("golang-test").Insert(resortConditions, true, "id", "test", "1").Execute()
		if insertErr != nil {
			log.Printf("Error: %v", insertErr)
		}
		log.Printf("%s updated successfully", config.Name)
	}

}
