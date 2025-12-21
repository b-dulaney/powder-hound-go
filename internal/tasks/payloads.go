package tasks

import (
	"encoding/json"
	"powderhoundgo/internal/email"

	"github.com/hibiken/asynq"
)

const (
	TypeResortWebScrapingJob    = "scrape:resort"
	TypeAvalancheScrapingJob    = "scrape:avalanche"
	TypeForecastAlertEmail      = "email:forecast"
	TypeOvernightEmail          = "email:overnight"
)

func NewResortWebScrapeTask(name string) (*asynq.Task, error) {
	payload, err := json.Marshal(ResortWebScrapePayload{MountainName: name})
	if err != nil {
		return nil, err
	}

	return asynq.NewTask(TypeResortWebScrapingJob, payload), nil
}

func NewAvalancheScrapingTask(mountainID int, lat, lon float64) (*asynq.Task, error) {
	payload, err := json.Marshal(AvalancheScrapingPayload{
		MountainID: mountainID,
		Lat:        lat,
		Lon:        lon,
	})
	if err != nil {
		return nil, err
	}

	return asynq.NewTask(TypeAvalancheScrapingJob, payload), nil
}

func NewAlertEmailTask(email string, emailData []email.EmailData, taskType string) (*asynq.Task, error) {
	payload, err := json.Marshal(AlertEmailPayload{Email: email, EmailData: emailData})
	if err != nil {
		return nil, err
	}

	return asynq.NewTask(taskType, payload), nil
}

func NewForecastAlertEmailTask(email string, emailData []email.EmailData) (*asynq.Task, error) {
	return NewAlertEmailTask(email, emailData, TypeForecastAlertEmail)
}

func NewOvernightAlertEmailTask(email string, emailData []email.EmailData) (*asynq.Task, error) {
	return NewAlertEmailTask(email, emailData, TypeOvernightEmail)
}
