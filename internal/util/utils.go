package util

import (
	"log"
	"os"
	"powderhoundgo/internal/queue"
	"powderhoundgo/internal/supabase"
	"time"

	"github.com/hibiken/asynq"
	"github.com/joho/godotenv"
	"github.com/robfig/cron/v3"
)

func LoadEnvironmentVariables() {
	err := godotenv.Load("../.env")
	if err != nil {
		log.Fatalf("Error loading .env file: %s", err)
	}
}

func InitializeEmailCronTasks(client *asynq.Client, supabase supabase.SupabaseClient) *cron.Cron {
	ENV := os.Getenv("ENV")
	loc, err := time.LoadLocation("America/Denver")
	if err != nil {
		log.Fatal(err)
	}

	cron := cron.New(cron.WithLocation(loc))

	if ENV == "production" {
		addProductionEmailCronTasks(cron, client, supabase)
	} else {
		addDevelopmentEmailCronTasks(cron, client, supabase)
	}

	printCronEntries(cron.Entries())

	return cron
}

func InitializeScrapingCronTasks(client *asynq.Client, supabase supabase.SupabaseClient) *cron.Cron {
	ENV := os.Getenv("ENV")
	log.Printf("ENV is %s", ENV)
	loc, err := time.LoadLocation("America/Denver")
	if err != nil {
		log.Fatal(err)
	}

	cron := cron.New(cron.WithLocation(loc))

	if ENV == "production" {
		addProductionScrapingCronTasks(cron, client, supabase)
	} else {
		addDevelopmentScrapingCronTasks(cron, client, supabase)
	}

	printCronEntries(cron.Entries())

	return cron
}

func addProductionScrapingCronTasks(cron *cron.Cron, client *asynq.Client, supabase supabase.SupabaseClient) {
	// Regular hourly web scraping jobs
	cron.AddFunc("@hourly", func() {
		queue.QueueResortWebScrapeTasks(client, supabase)
	})

	// Early morning web scraping jobs - checking for overnight snowfall - 5:00am - 6:00am
	cron.AddFunc("*/10 5-6 * * *", func() {
		queue.QueueResortWebScrapeTasks(client, supabase)
	})
}

func addDevelopmentScrapingCronTasks(cron *cron.Cron, client *asynq.Client, supabase supabase.SupabaseClient) {
	queue.QueueResortWebScrapeTasks(client, supabase)
	cron.AddFunc("@every 5m", func() {
		queue.QueueResortWebScrapeTasks(client, supabase)
	})
}

func addProductionEmailCronTasks(cron *cron.Cron, client *asynq.Client, supabase supabase.SupabaseClient) {
	// Daily forecast alert emails - 4:30pm
	cron.AddFunc("30 16 * * *", func() {
		queue.QueueForecastAlertEmailTasks(client, supabase)
	})

	// Overnight alert emails - 6:05am
	cron.AddFunc("5 6 * * *", func() {
		queue.QueueOvernightAlertEmailTasks(client, supabase)
	})
}

func addDevelopmentEmailCronTasks(cron *cron.Cron, client *asynq.Client, supabase supabase.SupabaseClient) {
	cron.AddFunc("@every 1m", func() {
		queue.QueueForecastAlertEmailTasks(client, supabase)
	})

	cron.AddFunc("@every 1m", func() {
		queue.QueueOvernightAlertEmailTasks(client, supabase)
	})
}

func printCronEntries(cronEntries []cron.Entry) {
	log.Printf("Cron Info: %+v\n", cronEntries)
}
