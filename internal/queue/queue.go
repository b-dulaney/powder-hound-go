package queue

import (
	"encoding/json"
	"log"
	"os"
	"sort"
	"time"

	"powderhoundgo/internal/email"
	"powderhoundgo/internal/supabase"
	"powderhoundgo/internal/tasks"

	"github.com/hibiken/asynq"
)

func QueueResortWebScrapeTasks(client *asynq.Client, supabase supabase.SupabaseClient) {
	mountainNames := supabase.GetAllMountainObjectNames()
	for _, mountain := range mountainNames {
		if isResortClosed(mountain, supabase) {
			log.Printf("[*] Resort %s is closed - skipping job", mountain)
		} else {
			payload, err := json.Marshal(tasks.ResortWebScrapePayload{MountainName: mountain})
			if err != nil {
				log.Fatal(err)
			}

			task := buildTask(tasks.TypeResortWebScrapingJob, payload)

			info, err := client.Enqueue(task, asynq.MaxRetry(3), asynq.Timeout(5*time.Minute))

			if err != nil {
				log.Printf("[*] Error enqueuing task: %v", err)
			}
			log.Printf("[*] Enqueued task: %v", info)
		}
	}
}

func QueueForecastAlertEmailTasks(client *asynq.Client, supabase supabase.SupabaseClient) {
	userAlerts := supabase.GetUserForecastAlerts()

	for _, user := range userAlerts {
		var emailData []email.EmailData
		for _, alert := range user.Alerts {
			emailData = append(emailData, email.EmailData{Location: alert.Location, Snowfall: alert.Snowfall})
		}

		sort.Slice(emailData, func(i, j int) bool {
			return emailData[i].Snowfall > emailData[j].Snowfall
		})

		payload, err := json.Marshal(tasks.AlertEmailPayload{Email: user.Email, EmailData: emailData})
		if err != nil {
			log.Fatal(err)
		}

		task := buildTask(tasks.TypeForecastAlertEmail, payload)

		info, err := client.Enqueue(task)

		if err != nil {
			log.Printf("[*] Error enqueuing task: %v", err)
		}
		log.Printf("[*] Enqueued task: %v", info)
	}
}

func QueueOvernightAlertEmailTasks(client *asynq.Client, supabase supabase.SupabaseClient) {
	userAlerts := supabase.GetUserOvernightAlerts()

	for _, user := range userAlerts {
		var emailData []email.EmailData
		for _, alert := range user.Alerts {
			emailData = append(emailData, email.EmailData{Location: alert.Location, Snowfall: alert.Snowfall})
		}

		sort.Slice(emailData, func(i, j int) bool {
			return emailData[i].Snowfall > emailData[j].Snowfall
		})

		payload, err := json.Marshal(tasks.AlertEmailPayload{Email: user.Email, EmailData: emailData})
		if err != nil {
			log.Fatal(err)
		}

		task := buildTask(tasks.TypeOvernightEmail, payload)

		info, err := client.Enqueue(task)

		if err != nil {
			log.Printf("[*] Error enqueuing task: %v", err)
		}
		log.Printf("[*] Enqueued task: %v", info)
	}
}

func buildTask(taskType string, payload []byte) *asynq.Task {
	var task *asynq.Task
	ENV := os.Getenv("ENV")

	if ENV == "production" {
		task = asynq.NewTask(taskType, payload, asynq.Retention(24*time.Hour))
	} else {
		task = asynq.NewTask(taskType, payload, asynq.Retention(5*time.Minute))
	}

	return task
}

// Used to determine whether or not the web scraping task should be queued
// If the config's closing date is in the past, we should not queue this task or collect any data
func isResortClosed(mountain string, supabase supabase.SupabaseClient) bool {
	config := supabase.GetConfigByName(mountain)

	if config.ClosingDate != "" {
		const layout = "2006-01-02 5:00pm (MST)"
		closingDate, err := time.Parse(layout, config.ClosingDate)
		if err != nil {
			log.Printf("Error parsing closing date: %v", err)
		}
		return time.Now().After(closingDate)
	}
	return false
}
