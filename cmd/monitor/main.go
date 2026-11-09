// Command monitor runs the scraper health and resort status check once, without Redis.
//
//	go run ./cmd/monitor                # check every resort
//	go run ./cmd/monitor -only a-basin  # check one resort
//
// It reads SUPABASE_URL, SUPABASE_SERVICE_ROLE_KEY, ANTHROPIC_API_KEY, GITHUB_APP_ID +
// GITHUB_APP_PRIVATE_KEY (or GITHUB_TOKEN), GITHUB_REPO, GITHUB_BASE_BRANCH and MONITOR_ENV from the environment or a .env file.
// Point it at the dev branch while testing so status changes and PRs stay off production.
package main

import (
	"context"
	"flag"
	"log"
	"time"

	"powderhoundgo/internal/monitor"

	"github.com/joho/godotenv"
)

func main() {
	envFile := flag.String("env-file", ".env", "dotenv file to load if present")
	only := flag.String("only", "", "check only this resort slug")
	flag.Parse()

	if err := godotenv.Load(*envFile); err != nil {
		log.Printf("no %s loaded (%v); using the process environment", *envFile, err)
	}

	m, err := monitor.New()
	if err != nil {
		log.Fatal(err)
	}
	if *only != "" {
		m.Only = *only
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Minute)
	defer cancel()
	if err := m.Run(ctx); err != nil {
		log.Fatal(err)
	}
}
