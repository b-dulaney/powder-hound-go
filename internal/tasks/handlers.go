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
	resortData, err := scraping.ScrapeResortData(&p.MountainName)
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

func HandleAvalancheScrapingTask(c context.Context, t *asynq.Task) error {
	supabaseClient := supabase.NewSupabaseService()
	var p AvalancheScrapingPayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return fmt.Errorf("json.Unmarshal failed: %v: %w", err, asynq.SkipRetry)
	}

	mountain := scraping.MountainCoordinates{
		MountainID: p.MountainID,
		Lat:        p.Lat,
		Lon:        p.Lon,
	}

	forecast, err := scraping.ScrapeAvalancheForecast(mountain)
	if err != nil {
		scrapingData := supabase.ScrapingStatusData{
			MountainName: fmt.Sprintf("avalanche-%d", p.MountainID),
			Success:      false,
			Error:        err.Error(),
		}
		supabaseClient.InsertScrapingStatus(scrapingData)
		return fmt.Errorf("failed to scrape avalanche forecast for mountain %d: %w", p.MountainID, err)
	}

	// Convert forecast to map for upsert
	forecastData := map[string]interface{}{
		"mountain_id":          forecast.MountainID,
		"avalanche_summary":    forecast.AvalancheSummary,
		"issue_date":           forecast.IssueDate,
		"overall_danger_level": forecast.OverallDangerLevel,
		"danger_levels":        forecast.DangerLevels,
		"forecast_url":         forecast.ForecastURL,
		"updated_at":           forecast.UpdatedAt,
	}

	err = supabaseClient.UpsertAvalancheForecast(forecastData)
	if err != nil {
		return fmt.Errorf("failed to upsert avalanche forecast for mountain %d: %w", p.MountainID, err)
	}

	scrapingData := supabase.ScrapingStatusData{
		MountainName: fmt.Sprintf("avalanche-%d", p.MountainID),
		Success:      true,
	}
	supabaseClient.InsertScrapingStatus(scrapingData)

	log.Printf("Finished avalanche scraping job for mountain %d", p.MountainID)
	return nil
}

// HandleAlertEmailTask sends one user's alert email. A send failure is also recorded against
// the job's heartbeat so the health check reports it (the daily queue step records success).
func HandleAlertEmailTask(c context.Context, t *asynq.Task, job, subject string, buildEmailBody func([]email.EmailData) string) error {
	var p AlertEmailPayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return fmt.Errorf("json.Unmarshal failed: %v: %w", err, asynq.SkipRetry)
	}

	emailBody := buildEmailBody(p.EmailData)

	resend := email.NewResendService()
	err := resend.SendEmail(subject, emailBody, p.Email)
	if err != nil {
		err = fmt.Errorf("failed to send email: %w", err)
		supabase.NewSupabaseService().RecordJobRun(job, err)
		return err
	}

	return nil
}

func HandleForecastAlertEmailTask(c context.Context, t *asynq.Task) error {
	return HandleAlertEmailTask(c, t, "forecast-alert-email", "PowderHound forecast alert", email.BuildForecastAlertEmail)
}

func HandleOvernightAlertEmailTask(c context.Context, t *asynq.Task) error {
	return HandleAlertEmailTask(c, t, "overnight-alert-email", "PowderHound recent snowfall alert", email.BuildOvernightAlertEmail)
}
