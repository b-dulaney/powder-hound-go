package scraping

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/chromedp"
)

func fetchConfig(configPath *string) Config {
	if *configPath == "" {
		log.Fatal("Config path is required")
	}
	configFile, configErr := os.Open(*configPath)

	if configErr != nil {
		log.Fatal(configErr)
	}

	byteValue, _ := io.ReadAll(configFile)
	var config Config
	json.Unmarshal(byteValue, &config)

	defer configFile.Close()
	return config
}

func removeNonNumericCharacters(input string) (string, error) {
	input = strings.Split(input, ".")[0]
	reg, err := regexp.Compile("[^0-9]+")
	if err != nil {
		return "", err
	}
	processedString := reg.ReplaceAllString(input, "")

	return processedString, nil
}

// Removes the denominator from a string (e.g. "12/24" -> "12")
//
// Also removes the "of" keyword (e.g. "12 of 24" -> "12")
func removeDenominator(input string) (string, error) {
	input = strings.Split(input, "/")[0]
	input = strings.Split(input, "of")[0]
	result, err := removeNonNumericCharacters(input)
	return result, err
}

func convertStringToInt(input string) (int, error) {
	cleanedString, err := removeNonNumericCharacters(input)
	if err != nil {
		return 0, err
	}

	result, err := strconv.Atoi(cleanedString)
	if err != nil {
		return 0, err
	}
	return result, nil
}

func runChromeDP(ctx context.Context, tasks ...chromedp.Action) error {
	err := chromedp.Run(ctx, tasks...)
	if err != nil {
		return err
	}
	return nil
}

func getTextFromNode(ctx context.Context, selector string, node *cdp.Node, result *string) {
	if selector != "" {
		runChromeDP(ctx, chromedp.Text(selector, result, chromedp.ByQuery, chromedp.FromNode(node)))
	}
}
