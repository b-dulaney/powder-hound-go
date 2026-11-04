package util

import (
	"errors"
	"io/fs"
	"log"
	"os"
	"powderhoundgo/internal/queue"
	"powderhoundgo/internal/supabase"
	"time"

	"github.com/hibiken/asynq"
	"github.com/joho/godotenv"
	"github.com/robfig/cron/v3"
)

// LoadEnvironmentVariables loads ENV_FILE, or ../.env by default. In Docker there is no file:
// docker compose passes the server's .env to the container (env_file), so a missing default
// file just means the environment is already set. Point local runs at the dev branch with
// ENV_FILE=../.env.dev; an ENV_FILE that can't be loaded is fatal.
func LoadEnvironmentVariables() {
	file, explicit := os.Getenv("ENV_FILE"), true
	if file == "" {
		file, explicit = "../.env", false
	}
	if err := godotenv.Load(file); err != nil {
		if explicit || !errors.Is(err, fs.ErrNotExist) {
			log.Fatalf("Error loading %s: %s", file, err)
		}
		log.Printf("No %s; using the process environment", file)
		return
	}
	log.Printf("Loaded environment from %s", file)
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
	addMonitorCronTask(cron, client, ENV)

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

	// 5pm - afternoon update check
	cron.AddFunc("@hourly", func() {
		queue.QueueAvalancheScrapingTasks(client, supabase)
	})
}

func addDevelopmentScrapingCronTasks(cron *cron.Cron, client *asynq.Client, supabase supabase.SupabaseClient) {
	queue.QueueResortWebScrapeTasks(client, supabase)
	queue.QueueAvalancheScrapingTasks(client, supabase)
	cron.AddFunc("@every 5m", func() {
		queue.QueueResortWebScrapeTasks(client, supabase)
		queue.QueueAvalancheScrapingTasks(client, supabase)
	})
}

// addMonitorCronTask schedules the daily scraper health and resort status check.
// MONITOR_SCHEDULE overrides the time (cron spec, America/Denver); "off" disables it.
// Production defaults to 11:00, when lifts are running. Development only runs it when
// MONITOR_SCHEDULE is set.
func addMonitorCronTask(cron *cron.Cron, client *asynq.Client, env string) {
	spec := os.Getenv("MONITOR_SCHEDULE")
	if spec == "" && env == "production" {
		spec = "0 11 * * *"
	}
	if spec == "" || spec == "off" {
		log.Printf("Monitor cron disabled")
		return
	}
	if _, err := cron.AddFunc(spec, func() { queue.QueueMonitorTask(client) }); err != nil {
		log.Printf("Invalid MONITOR_SCHEDULE %q: %v", spec, err)
	}
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
