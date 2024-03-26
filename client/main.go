package main

import (
	"log"
	"os"
	"os/signal"
	"powderhoundgo/tasks"
	"syscall"
	"time"

	"github.com/hibiken/asynq"
	"github.com/robfig/cron/v3"
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

	cron := cron.New(cron.WithLocation(loc))
	cron.AddFunc("@hourly", func() {
		QueueResortWebScrapeTasks(client)
	})

	cron.AddFunc("*/10 5-6 * * *", func() {
		QueueResortWebScrapeTasks(client)
	})

	go cron.Start()
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	<-sig

	printCronEntries(cron.Entries())

	select {}
}

func printCronEntries(cronEntries []cron.Entry) {
	log.Printf("Cron Info: %+v\n", cronEntries)
}
