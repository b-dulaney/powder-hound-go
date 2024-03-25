package main

import (
	"log"
	"os"
	"powderhoundgo/tasks"
	"time"

	"github.com/hibiken/asynq"
	"github.com/robfig/cron"
)

func main() {
	tasks.LoadEnvironmentVariables()
	redisHost := os.Getenv("REDIS_HOST")
	if redisHost == "" {
		redisHost = "localhost"
	}
	log.Printf("Connecting to Redis at %s", redisHost)
	client := asynq.NewClient(asynq.RedisClientOpt{Addr: redisHost + ":6379", Password: "", DB: 0})
	defer client.Close()

	loc, err := time.LoadLocation("America/Denver")
	if err != nil {
		log.Fatal(err)
	}

	hourlyResortCronJob := cron.New()
	hourlyResortCronJob.AddFunc("@hourly", func() {
		QueueResortWebScrapeTasks(client)
	})
	hourlyResortCronJob.Start()
	defer hourlyResortCronJob.Stop()

	earlyMorningResortCronJob := cron.NewWithLocation(loc)
	earlyMorningResortCronJob.AddFunc("*/10 5-6 * * *", func() {
		QueueResortWebScrapeTasks(client)
	})
	earlyMorningResortCronJob.Start()
	defer earlyMorningResortCronJob.Stop()

	select {}
}
