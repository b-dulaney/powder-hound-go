package queue

import (
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"time"

	"powderhoundgo/internal/email"
	"powderhoundgo/internal/supabase"
	"powderhoundgo/internal/tasks"

	"github.com/hibiken/asynq"
)

var MountainNames []string = []string{
	"copper-mountain",
	"aspen-mountain",
	"aspen-highlands",
	"powderhorn",
	"vail",
	"breckenridge",
	"keystone",
	"beaver-creek",
	"aspen-snowmass",
	"steamboat",
	"telluride",
	"winter-park",
	"crested-butte",
	"a-basin",
	"eldora",
	"loveland",
	"monarch",
	"purgatory",
	"sunlight-mountain",
}

func QueueResortWebScrapeTasks(client *asynq.Client) {
	for _, mountain := range MountainNames {
		payload, err := json.Marshal(tasks.ResortWebScrapePayload{MountainName: mountain})
		if err != nil {
			log.Fatal(err)
		}

		task := asynq.NewTask(tasks.TypeResortWebScrapingJob, payload)

		info, err := client.Enqueue(task, asynq.MaxRetry(2), asynq.Timeout(10*time.Minute), asynq.TaskID(mountain), asynq.Queue("default"))

		if err != nil {
			log.Fatal(err)
		}
		log.Printf("[*] Enqueued task: %v", info)
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

		task := asynq.NewTask(tasks.TypeForecastAlertEmail, payload)

		info, err := client.Enqueue(task, asynq.TaskID(fmt.Sprintf("forecast-%s", user.Email)), asynq.Queue("high"))

		if err != nil {
			log.Fatal(err)
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

		task := asynq.NewTask(tasks.TypeOvernightEmail, payload)

		info, err := client.Enqueue(task, asynq.TaskID(fmt.Sprintf("overnight-%s", user.Email)), asynq.Queue("high"))

		if err != nil {
			log.Fatal(err)
		}
		log.Printf("[*] Enqueued task: %v", info)
	}
}
