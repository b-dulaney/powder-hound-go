package main

import (
	"log"
	"os"
	"powderhoundgo/internal/monitor"
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
	})

	// Lets the daily monitor queue its follow-up confirm-open checks.
	client := asynq.NewClient(redisOpts)
	defer client.Close()

	mux := asynq.NewServeMux()
	mux.HandleFunc(tasks.TypeResortWebScrapingJob, tasks.HandleResortWebScrapeTask)
	mux.HandleFunc(tasks.TypeAvalancheScrapingJob, tasks.HandleAvalancheScrapingTask)
	mux.HandleFunc(tasks.TypeMonitorDaily, monitor.DailyTask(client))
	mux.HandleFunc(tasks.TypeMonitorConfirmOpen, monitor.HandleConfirmOpenTask)

	if err := srv.Run(mux); err != nil {
		log.Fatal(err)
	}
}
