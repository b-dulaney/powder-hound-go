package tasks

import "powderhoundgo/internal/email"

type ResortWebScrapePayload struct {
	MountainName string
}

type AlertEmailPayload struct {
	Email     string
	EmailData []email.EmailData
}
