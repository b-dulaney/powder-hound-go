package tasks

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"powderhoundgo/internal/email"
	"powderhoundgo/internal/scraping"
	"powderhoundgo/internal/supabase"

	"github.com/hibiken/asynq"
)

func HandleResortWebScrapeTask(c context.Context, t *asynq.Task) error {
	supabaseClient := supabase.NewSupabaseService()
	var p ResortWebScrapePayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return fmt.Errorf("json.Unmarshal failed: %v: %w", err, asynq.SkipRetry)
	}
	configPath := fmt.Sprintf("../../config/%s.json", p.MountainName)

	resortData, err := scraping.ScrapeResortData(&configPath)
	if err != nil {
		scrapingData := supabase.ScrapingStatusData{MountainName: p.MountainName, Success: false, Error: err.Error()}
		err := supabaseClient.InsertScrapingStatus(scrapingData)
		if err != nil {
			log.Printf("failed to insert scraping status: %s", err)
		}
		return fmt.Errorf("failed to scrape %s: %w", p.MountainName, err)
	}

	err = supabaseClient.UpsertResortConditionsData(resortData)
	if err != nil {
		return fmt.Errorf("failed to upsert conditions data %s", p.MountainName)
	}

	scrapingData := supabase.ScrapingStatusData{MountainName: p.MountainName, Success: true}
	err = supabaseClient.InsertScrapingStatus(scrapingData)
	if err != nil {
		log.Printf("failed to insert scraping status: %s", err)
	}

	log.Printf("Finished web scraping job for %s", p.MountainName)
	return nil
}

func HandleAlertEmailTask(c context.Context, t *asynq.Task, subject string, buildEmailBody func([]email.EmailData) string) error {
	var p AlertEmailPayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return fmt.Errorf("json.Unmarshal failed: %v: %w", err, asynq.SkipRetry)
	}

	emailBody := buildEmailBody(p.EmailData)

	resend := email.NewResendService()
	err := resend.SendEmail(subject, emailBody, p.Email)
	if err != nil {
		return fmt.Errorf("failed to send email: %w", err)
	}

	return nil
}

func HandleForecastAlertEmailTask(c context.Context, t *asynq.Task) error {
	return HandleAlertEmailTask(c, t, "PowderHound forecast alert", email.BuildForecastAlertEmail)
}

func HandleOvernightAlertEmailTask(c context.Context, t *asynq.Task) error {
	return HandleAlertEmailTask(c, t, "PowderHound recent snowfall alert", email.BuildOvernightAlertEmail)
}
