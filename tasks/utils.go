package tasks

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
	"github.com/joho/godotenv"
	"github.com/supabase-community/supabase-go"
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

func removeNonNumericCharacters(input string) string {
	input = strings.Split(input, ".")[0]
	reg, err := regexp.Compile("[^0-9]+")
	if err != nil {
		log.Panicf("Failed compiling regex: %s", err)
	}
	processedString := reg.ReplaceAllString(input, "")

	return processedString
}

// Removes the denominator from a string (e.g. "12/24" -> "12")
//
// Also removes the "of" keyword (e.g. "12 of 24" -> "12")
func removeDenominator(input string) string {
	input = strings.Split(input, "/")[0]
	input = strings.Split(input, "of")[0]
	return removeNonNumericCharacters(input)
}

func convertStringToInt(input string) int {
	var cleanedString = removeNonNumericCharacters(input)
	result, err := strconv.Atoi(cleanedString)
	if err != nil {
		log.Panicf("Failed converting value to int: %s", err)
	}
	return result
}

func initializeSupabase() *supabase.Client {
	SUPABASE_URL := os.Getenv("SUPABASE_URL")
	SUPABASE_SERVICE_ROLE_KEY := os.Getenv("SUPABASE_SERVICE_ROLE_KEY")
	client, clientErr := supabase.NewClient(SUPABASE_URL, SUPABASE_SERVICE_ROLE_KEY, nil)
	if clientErr != nil {
		log.Fatalf("Error creating supabase client: %s", clientErr)
	}
	return client
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

func LoadEnvironmentVariables() {
	err := godotenv.Load(".env")
	if err != nil {
		log.Fatalf("Error loading .env file: %s", err)
	}
}

func upsertSupabaseData(client *supabase.Client, data map[string]interface{}) error {
	_, _, err := client.From("resort_conditions").Upsert(data, "mountain_id", "*", "estimated").Execute()
	if err != nil {
		log.Printf("Failed to upsert data: %s", err)
	}
	return err
}
