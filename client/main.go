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
	supabase := tasks.InitializeSupabase()

	redisHost := os.Getenv("REDIS_HOST")
	if redisHost == "" {
		redisHost = "localhost"
	}

	client := asynq.NewClient(asynq.RedisClientOpt{Addr: redisHost + ":6379", Password: "", DB: 0})
	defer client.Close()

	loc, err := time.LoadLocation("America/Denver")
	if err != nil {
		log.Fatal(err)
	}

	cron := cron.New(cron.WithLocation(loc))
	// Regular hourly web scraping jobs
	cron.AddFunc("@hourly", func() {
		QueueResortWebScrapeTasks(client)
	})

	// Early morning web scraping jobs - checking for overnight snowfall - 5:00am - 6:00am
	cron.AddFunc("*/10 5-6 * * *", func() {
		QueueResortWebScrapeTasks(client)
	})

	// Daily forecast alert emails - 4:30pm
	cron.AddFunc("30 16 * * *", func() {
		QueueForecastAlertEmailTasks(client, supabase)
	})

	// Overnight alert emails - 6:05am
	cron.AddFunc("5 6 * * *", func() {
		QueueOvernightAlertEmailTasks(client, supabase)
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
