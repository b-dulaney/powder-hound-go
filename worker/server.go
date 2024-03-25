package main

import (
	"log"
	"os"
	"powderhoundgo/tasks"

	"github.com/hibiken/asynq"
)

func main() {
	tasks.LoadEnvironmentVariables()
	redisHost := os.Getenv("REDIS_HOST")
	if redisHost == "" {
		redisHost = "localhost"
	}
	srv := asynq.NewServer(
		asynq.RedisClientOpt{Addr: redisHost + ":6379", Password: "", DB: 0},
		asynq.Config{Concurrency: 10},
	)

	mux := asynq.NewServeMux()
	mux.HandleFunc(tasks.TypeResortWebScrapingJob, tasks.HandleResortWebScrapeTask)

	if err := srv.Run(mux); err != nil {
		log.Fatal(err)
	}
}
