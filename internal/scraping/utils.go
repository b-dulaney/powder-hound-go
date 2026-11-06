package scraping

import (
	"context"
	"errors"
	"log"
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

var totalPattern = regexp.MustCompile(`(?i)\d[\d,]*\s*(?:/|\bof\b)\s*(\d[\d,]*)`)

// parseTotal returns the denominator of an "open of total" count ("12 of 147", "12/147",
// "Lifts Open: 0 of 9"), or 0 when the text doesn't include one.
func parseTotal(input string) int {
	m := totalPattern.FindStringSubmatch(input)
	if m == nil {
		return 0
	}
	n, err := strconv.Atoi(strings.ReplaceAll(m[1], ",", ""))
	if err != nil {
		return 0
	}
	return n
}

var firstNumber = regexp.MustCompile(`\d[\d,]*`)

func convertStringToInt(input string) (int, error) {
	// Handle special cases like "--" which should be treated as 0
	trimmedInput := strings.TrimSpace(input)
	if trimmedInput == "" || trimmedInput == "--" || trimmedInput == "—" {
		return 0, nil
	}

	// The first number in the text, so labels that contain digits don't run into the value
	// ("0\" Past 48HRS" is 0, not 48). Anything after a "." is dropped, as before.
	cleanedString := strings.ReplaceAll(firstNumber.FindString(strings.Split(input, ".")[0]), ",", "")
	if cleanedString == "" {
		return 0, nil
	}

	result, err := strconv.Atoi(cleanedString)
	if err != nil {
		errorWithInput := errors.New("Failed to convert string to int: " + input)
		return 0, errorWithInput
	}
	return result, nil
}

func runChromeDP(ctx context.Context, tasks ...chromedp.Action) error {
	log.Printf("Running ChromeDP tasks: %v", tasks)
	err := chromedp.Run(ctx, tasks...)
	if err != nil {
		return err
	}
	return nil
}

func getTextFromNode(ctx context.Context, selector string, node *cdp.Node, result *string) {
	log.Printf("Getting text from node with selector: %s", selector)
	if selector != "" {
		readText(ctx, selector, node, result)
	}
}

// readText reads the rendered text of the first element matching selector inside node. Hidden
// elements (a closed header popup, say) have no rendered text, so for those it falls back to
// textContent.
func readText(ctx context.Context, selector string, node *cdp.Node, result *string) error {
	err := runChromeDP(ctx, chromedp.Text(selector, result, chromedp.ByQuery, chromedp.FromNode(node)))
	if err != nil || strings.TrimSpace(*result) != "" {
		return err
	}
	return runChromeDP(ctx, chromedp.TextContent(selector, result, chromedp.ByQuery, chromedp.FromNode(node)))
}
