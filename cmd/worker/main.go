package main

import (
	"log"
	"os"
	"powderhoundgo/internal/tasks"
	"powderhoundgo/internal/util"

	"github.com/hibiken/asynq"
)

func main() {
	util.LoadEnvironmentVariables()
	redisHost := os.Getenv("REDIS_HOST")
	if redisHost == "" {
		redisHost = "localhost"
	}

	redisOpts := asynq.RedisClientOpt{Addr: redisHost + ":6379", Password: "", DB: 0}
	srv := asynq.NewServer(redisOpts, asynq.Config{
		Concurrency: 0,
		Queues: map[string]int{
			"high":    3,
			"default": 2,
			"low":     1,
		},
		StrictPriority: true,
	})

	mux := asynq.NewServeMux()
	mux.HandleFunc(tasks.TypeResortWebScrapingJob, tasks.HandleResortWebScrapeTask)
	mux.HandleFunc(tasks.TypeForecastAlertEmail, tasks.HandleForecastAlertEmailTask)
	mux.HandleFunc(tasks.TypeOvernightEmail, tasks.HandleOvernightAlertEmailTask)

	if err := srv.Run(mux); err != nil {
		log.Fatal(err)
	}
}
