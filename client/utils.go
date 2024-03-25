package main

import (
	"encoding/json"
	"log"

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

		t1 := asynq.NewTask("scrape:resort", payload)

		info, err := client.Enqueue(t1)

		if err != nil {
			log.Fatal(err)
		}
		log.Printf("[*] Enqueued task: %v", info)
	}
}
