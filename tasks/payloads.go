package tasks

import (
	"encoding/json"

	"github.com/hibiken/asynq"
)

const (
	TypeResortWebScrapingJob = "scrape:resort"
	// TypeForecastEmail        = "email:forecast"
	// TypeOvernightEmail       = "email:overnight"
)

type ResortWebScrapePayload struct {
	MountainName string
}

func NewResortWebScrapeTask(name string) (*asynq.Task, error) {
	payload, err := json.Marshal(ResortWebScrapePayload{MountainName: name})
	if err != nil {
		return nil, err
	}

	return asynq.NewTask(TypeResortWebScrapingJob, payload), nil
}
