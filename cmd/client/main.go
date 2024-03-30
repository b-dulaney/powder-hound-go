package main

import (
	"log"
	"os"
	"os/signal"
	"powderhoundgo/internal/queue"
	"powderhoundgo/internal/supabase"
	"powderhoundgo/internal/util"
	"syscall"
	"time"

	"github.com/hibiken/asynq"
	"github.com/robfig/cron/v3"
)

func main() {
	util.LoadEnvironmentVariables()
	supabase := supabase.NewSupabaseService()

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
		queue.QueueResortWebScrapeTasks(client)
	})

	// Early morning web scraping jobs - checking for overnight snowfall - 5:00am - 6:00am
	cron.AddFunc("*/10 5-6 * * *", func() {
		queue.QueueResortWebScrapeTasks(client)
	})

	// Daily forecast alert emails - 4:30pm
	cron.AddFunc("30 16 * * *", func() {
		queue.QueueForecastAlertEmailTasks(client, supabase)
	})

	// Overnight alert emails - 6:05am
	cron.AddFunc("5 6 * * *", func() {
		queue.QueueOvernightAlertEmailTasks(client, supabase)
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
