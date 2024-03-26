package tasks

import (
	"encoding/json"

	"github.com/hibiken/asynq"
)

const (
	TypeResortWebScrapingJob = "scrape:resort"
	TypeForecastAlertEmail   = "email:forecast"
	TypeOvernightEmail       = "email:overnight"
)

func NewResortWebScrapeTask(name string) (*asynq.Task, error) {
	payload, err := json.Marshal(ResortWebScrapePayload{MountainName: name})
	if err != nil {
		return nil, err
	}

	return asynq.NewTask(TypeResortWebScrapingJob, payload), nil
}

func NewAlertEmailTask(email string, emailData []EmailData, taskType string) (*asynq.Task, error) {
	payload, err := json.Marshal(AlertEmailPayload{Email: email, EmailData: emailData})
	if err != nil {
		return nil, err
	}

	return asynq.NewTask(taskType, payload), nil
}

func NewForecastAlertEmailTask(email string, emailData []EmailData) (*asynq.Task, error) {
	return NewAlertEmailTask(email, emailData, TypeForecastAlertEmail)
}

func NewOvernightAlertEmailTask(email string, emailData []EmailData) (*asynq.Task, error) {
	return NewAlertEmailTask(email, emailData, TypeOvernightEmail)
}
