package scraping

import (
	"context"
	"errors"
	"regexp"
	"strconv"
	"strings"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/chromedp"
)

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
		errorWithInput := errors.New("Failed to convert string to int: " + input)
		return 0, errorWithInput
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
