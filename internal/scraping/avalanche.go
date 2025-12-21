package scraping

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
)

// CSS Selectors for CAIC avalanche forecast page
const (
	// Container selector to wait for - indicates page has loaded
	AvalancheContainerSelector = ".sm\\:pt-4"

	// Summary selectors (relative to page)
	AvalancheSummaryOneSelector = ".sm\\:pt-4 > p:nth-child(1)"
	AvalancheSummaryTwoSelector = ".sm\\:pt-4 > p:nth-child(2)"
	IssueDateSelector           = "span.whitespace-nowrap:nth-child(3)"

	// Day label selectors
	DayOneSelector = "div.mt-4:nth-child(1) > div:nth-child(1) > div:nth-child(1) > div:nth-child(1) > div:nth-child(1)"
	DayTwoSelector = "div.mt-4:nth-child(1) > div:nth-child(1) > div:nth-child(2) > div:nth-child(1) > div:nth-child(1)"

	// Tree line selectors
	AboveTreeLineDayOneSelector = "div.mt-4:nth-child(1) > div:nth-child(1) > div:nth-child(1) > div:nth-child(2) > div:nth-child(1) > div:nth-child(2) > p:nth-child(1) > b:nth-child(1)"
	AboveTreeLineDayTwoSelector = "div.mt-4:nth-child(1) > div:nth-child(1) > div:nth-child(2) > div:nth-child(2) > div:nth-child(1) > div:nth-child(1) > p:nth-child(1) > b:nth-child(1)"
	NearTreeLineDayOneSelector  = "div.mt-4:nth-child(1) > div:nth-child(1) > div:nth-child(1) > div:nth-child(2) > div:nth-child(2) > div:nth-child(2) > p:nth-child(1) > b:nth-child(1)"
	NearTreeLineDayTwoSelector  = "div.mt-4:nth-child(1) > div:nth-child(1) > div:nth-child(2) > div:nth-child(2) > div:nth-child(2) > div:nth-child(1) > p:nth-child(1) > b:nth-child(1)"
	BelowTreeLineDayOneSelector = "div.mt-4:nth-child(1) > div:nth-child(1) > div:nth-child(1) > div:nth-child(2) > div:nth-child(3) > div:nth-child(2) > p:nth-child(1) > b:nth-child(1)"
	BelowTreeLineDayTwoSelector = "div.mt-4:nth-child(1) > div:nth-child(1) > div:nth-child(2) > div:nth-child(2) > div:nth-child(3) > div:nth-child(1) > p:nth-child(1) > b:nth-child(1)"
)

// MountainCoordinates represents a mountain's location for avalanche forecasting
type MountainCoordinates struct {
	MountainID int     `json:"mountain_id"`
	Lat        float64 `json:"lat"`
	Lon        float64 `json:"lon"`
}

// AvalancheRating represents the danger level at a specific elevation
type AvalancheRating struct {
	Level  int    `json:"level"`
	Rating string `json:"rating"`
}

// AvalancheDangerLevel represents danger levels for a specific day
type AvalancheDangerLevel struct {
	Date          string          `json:"date"`
	AboveTreeline AvalancheRating `json:"above_treeline"`
	NearTreeline  AvalancheRating `json:"near_treeline"`
	BelowTreeline AvalancheRating `json:"below_treeline"`
}

// AvalancheForecast represents the complete forecast for a mountain
type AvalancheForecast struct {
	MountainID         int                    `json:"mountain_id"`
	AvalancheSummary   string                 `json:"avalanche_summary"`
	IssueDate          string                 `json:"issue_date"`
	OverallDangerLevel int                    `json:"overall_danger_level"`
	DangerLevels       []AvalancheDangerLevel `json:"danger_levels"`
	ForecastURL        string                 `json:"forecast_url"`
	UpdatedAt          time.Time              `json:"updated_at"`
}

// parseTreeLineData parses the tree line text (e.g., "3-Considerable") into level and rating
func parseTreeLineData(text string) AvalancheRating {
	text = strings.TrimSpace(text)
	if text == "" {
		return AvalancheRating{Level: 0, Rating: "No Rating"}
	}

	parts := strings.SplitN(text, "-", 2)
	if len(parts) < 2 {
		return AvalancheRating{Level: 0, Rating: "No Rating"}
	}

	level, err := convertStringToInt(parts[0])
	if err != nil {
		level = 0
	}

	rating := strings.TrimSpace(parts[1])
	if rating == "" {
		rating = "No Rating"
	}

	return AvalancheRating{Level: level, Rating: rating}
}

// ScrapeAvalancheForecast scrapes avalanche forecast data for a given mountain
func ScrapeAvalancheForecast(mountain MountainCoordinates) (*AvalancheForecast, error) {
	ctx, cancel := chromedp.NewContext(context.Background(), chromedp.WithLogf(log.Printf))
	defer cancel()

	ctx, cancel = context.WithTimeout(ctx, 120*time.Second)
	defer cancel()

	forecastURL := fmt.Sprintf("https://avalanche.state.co.us/?lat=%f&lng=%f", mountain.Lat, mountain.Lon)
	log.Printf("Navigating to: %s", forecastURL)

	// Navigate to the page
	runChromeDP(ctx, chromedp.EmulateViewport(1200, 1000), chromedp.Navigate(forecastURL))

	// Wait for the main content container to be visible (indicates JS has loaded)
	log.Printf("Waiting for content to load...")
	runChromeDP(ctx, chromedp.WaitVisible(AvalancheContainerSelector))
	log.Printf("Content container visible, extracting data...")

	// Extract all data in batched operations
	var avalancheSummaryOne, avalancheSummaryTwo string
	var issueDate, dayOne, dayTwo string
	var aboveTreeLineDayOneText, nearTreeLineDayOneText, belowTreeLineDayOneText string
	var aboveTreeLineDayTwoText, nearTreeLineDayTwoText, belowTreeLineDayTwoText string

	// Batch 1: Get summary and issue date
	runChromeDP(ctx,
		chromedp.InnerHTML(AvalancheSummaryOneSelector, &avalancheSummaryOne, chromedp.ByQuery),
	)
	// Second summary paragraph is optional - use a short timeout
	tctx, tcancel := context.WithTimeout(ctx, 2*time.Second)
	chromedp.Run(tctx, chromedp.InnerHTML(AvalancheSummaryTwoSelector, &avalancheSummaryTwo, chromedp.ByQuery))
	tcancel()

	runChromeDP(ctx,
		chromedp.Text(IssueDateSelector, &issueDate, chromedp.ByQuery),
	)

	// Batch 2: Get day labels
	runChromeDP(ctx,
		chromedp.Text(DayOneSelector, &dayOne, chromedp.ByQuery),
		chromedp.Text(DayTwoSelector, &dayTwo, chromedp.ByQuery),
	)

	// Batch 3: Get tree line ratings for day one
	runChromeDP(ctx,
		chromedp.Text(AboveTreeLineDayOneSelector, &aboveTreeLineDayOneText, chromedp.ByQuery),
		chromedp.Text(NearTreeLineDayOneSelector, &nearTreeLineDayOneText, chromedp.ByQuery),
		chromedp.Text(BelowTreeLineDayOneSelector, &belowTreeLineDayOneText, chromedp.ByQuery),
	)

	// Batch 4: Get tree line ratings for day two
	runChromeDP(ctx,
		chromedp.Text(AboveTreeLineDayTwoSelector, &aboveTreeLineDayTwoText, chromedp.ByQuery),
		chromedp.Text(NearTreeLineDayTwoSelector, &nearTreeLineDayTwoText, chromedp.ByQuery),
		chromedp.Text(BelowTreeLineDayTwoSelector, &belowTreeLineDayTwoText, chromedp.ByQuery),
	)

	// Build the summary
	var avalancheSummary string
	avalancheSummaryTwo = strings.TrimSpace(avalancheSummaryTwo)
	if avalancheSummaryTwo != "" {
		avalancheSummary = fmt.Sprintf("%s<br><br>%s", avalancheSummaryOne, avalancheSummaryTwo)
	} else {
		avalancheSummary = avalancheSummaryOne
	}

	// Parse tree line data
	aboveTreeLineDayOne := parseTreeLineData(aboveTreeLineDayOneText)
	nearTreeLineDayOne := parseTreeLineData(nearTreeLineDayOneText)
	belowTreeLineDayOne := parseTreeLineData(belowTreeLineDayOneText)

	aboveTreeLineDayTwo := parseTreeLineData(aboveTreeLineDayTwoText)
	nearTreeLineDayTwo := parseTreeLineData(nearTreeLineDayTwoText)
	belowTreeLineDayTwo := parseTreeLineData(belowTreeLineDayTwoText)

	// Build danger levels
	dangerDayOne := AvalancheDangerLevel{
		Date:          dayOne,
		AboveTreeline: aboveTreeLineDayOne,
		NearTreeline:  nearTreeLineDayOne,
		BelowTreeline: belowTreeLineDayOne,
	}

	dangerDayTwo := AvalancheDangerLevel{
		Date:          dayTwo,
		AboveTreeline: aboveTreeLineDayTwo,
		NearTreeline:  nearTreeLineDayTwo,
		BelowTreeline: belowTreeLineDayTwo,
	}

	// Calculate overall danger level (max of all day one levels)
	overallDangerLevel := max(
		dangerDayOne.AboveTreeline.Level,
		dangerDayOne.NearTreeline.Level,
		dangerDayOne.BelowTreeline.Level,
	)

	forecast := &AvalancheForecast{
		MountainID:         mountain.MountainID,
		AvalancheSummary:   avalancheSummary,
		IssueDate:          issueDate,
		OverallDangerLevel: overallDangerLevel,
		DangerLevels:       []AvalancheDangerLevel{dangerDayOne, dangerDayTwo},
		ForecastURL:        forecastURL,
		UpdatedAt:          time.Now(),
	}

	log.Printf("Successfully scraped avalanche forecast for mountain %d", mountain.MountainID)
	return forecast, nil
}
