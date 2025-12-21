package tasks

import "powderhoundgo/internal/email"

type ResortWebScrapePayload struct {
	MountainName string
}

type AvalancheScrapingPayload struct {
	MountainID int
	Lat        float64
	Lon        float64
}

type AlertEmailPayload struct {
	Email     string
	EmailData []email.EmailData
}
