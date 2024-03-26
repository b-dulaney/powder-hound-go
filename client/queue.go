package main

import (
	"encoding/json"
	"fmt"
	"log"
	"powderhoundgo/tasks"
	"sort"
	"time"

	"github.com/hibiken/asynq"
	"github.com/supabase-community/supabase-go"
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

type OvernightAlert struct {
	Location string `json:"display_name"`
	Snowfall int    `json:"snow_past_24h"`
}

type ForecastAlert struct {
	Location string `json:"display_name"`
	Snowfall int    `json:"snow_next_24h"`
}

type UserOvernightAlert struct {
	Email  string           `json:"email"`
	Alerts []OvernightAlert `json:"alerts"`
}

type UserForecastAlert struct {
	Email  string          `json:"email"`
	Alerts []ForecastAlert `json:"alerts"`
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

func QueueForecastAlertEmailTasks(client *asynq.Client, supabase *supabase.Client) {
	userAlerts := GetUserForecastAlerts(supabase)

	for _, user := range userAlerts {
		var emailData []tasks.EmailData
		for _, alert := range user.Alerts {
			emailData = append(emailData, tasks.EmailData{Location: alert.Location, Snowfall: alert.Snowfall})
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

func QueueOvernightAlertEmailTasks(client *asynq.Client, supabase *supabase.Client) {
	userAlerts := GetUserOvernightAlerts(supabase)

	for _, user := range userAlerts {
		var emailData []tasks.EmailData
		for _, alert := range user.Alerts {
			emailData = append(emailData, tasks.EmailData{Location: alert.Location, Snowfall: alert.Snowfall})
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

func GetUserOvernightAlerts(supabase *supabase.Client) []UserOvernightAlert {
	userAlertsResponse := supabase.Rpc("group_overnight_snowfall_alert_data", "", nil)

	var userAlerts []UserOvernightAlert
	err := json.Unmarshal([]byte(userAlertsResponse), &userAlerts)
	if err != nil {
		log.Fatal(err)
	}

	return userAlerts
}

func GetUserForecastAlerts(supabase *supabase.Client) []UserForecastAlert {
	userAlertsResponse := supabase.Rpc("group_24h_forecast_alert_data", "", nil)

	var userAlerts []UserForecastAlert
	err := json.Unmarshal([]byte(userAlertsResponse), &userAlerts)
	if err != nil {
		log.Fatal(err)
	}

	return userAlerts
}
