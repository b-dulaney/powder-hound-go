package main

import (
	"encoding/json"
	"log"
	"time"

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

type ResortWebScrapeTaskPayload struct {
	MountainName string
}

func QueueResortWebScrapeTasks(client *asynq.Client) {
	for _, mountain := range MountainNames {
		payload, err := json.Marshal(ResortWebScrapeTaskPayload{MountainName: mountain})
		if err != nil {
			log.Fatal(err)
		}

		task := asynq.NewTask("scrape:resort", payload)

		info, err := client.Enqueue(task, asynq.MaxRetry(2), asynq.Timeout(10*time.Minute), asynq.TaskID(mountain))

		if err != nil {
			log.Fatal(err)
		}
		log.Printf("[*] Enqueued task: %v", info)
	}
}
