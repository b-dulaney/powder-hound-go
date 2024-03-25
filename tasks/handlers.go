package tasks

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/hibiken/asynq"
)

func HandleResortWebScrapeTask(c context.Context, t *asynq.Task) error {
	var p ResortWebScrapePayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return fmt.Errorf("json.Unmarshal failed: %v: %w", err, asynq.SkipRetry)
	}
	log.Printf("Starting web scraping job for %s", p.MountainName)

	configPath := fmt.Sprintf("./config/%s.json", p.MountainName)

	err := scrapeResortData(&configPath)
	if err != nil {
		return fmt.Errorf("failed to scrape %s", p.MountainName)
	}

	log.Printf("Finished web scraping job for %s", p.MountainName)
	return nil
}
