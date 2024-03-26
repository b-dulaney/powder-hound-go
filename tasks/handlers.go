package tasks

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/hibiken/asynq"
	"github.com/resend/resend-go/v2"
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

func HandleAlertEmailTask(c context.Context, t *asynq.Task, subject string, buildEmailBody func([]EmailData) string) error {
	resendClient := InitializeResendClient()
	var p AlertEmailPayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return fmt.Errorf("json.Unmarshal failed: %v: %w", err, asynq.SkipRetry)
	}

	emailBody := buildEmailBody(p.EmailData)
	params := &resend.SendEmailRequest{
		From:    "PowderHound <alerts@powderhound.io>",
		To:      []string{p.Email},
		Subject: subject,
		Html:    emailBody,
	}
	sent, err := resendClient.Emails.Send(params)
	if err != nil {
		return fmt.Errorf("failed to send %s email to %s: %w", subject, p.Email, err)
	}

	log.Printf("Finished sending %s email to %s. Resend ID: %s", subject, p.Email, sent.Id)
	return nil
}

func HandleForecastAlertEmailTask(c context.Context, t *asynq.Task) error {
	return HandleAlertEmailTask(c, t, "PowderHound forecast alert", BuildForecastAlertEmail)
}

func HandleOvernightAlertEmailTask(c context.Context, t *asynq.Task) error {
	return HandleAlertEmailTask(c, t, "PowderHound recent snowfall alert", BuildOvernightAlertEmail)
}
