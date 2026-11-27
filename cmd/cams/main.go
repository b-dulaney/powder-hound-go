// Command cams runs the snow stake cam capture once, without Redis: it syncs every resort's
// cams from the scraper configs and captures each cam inline.
//
//	go run ./cmd/cams -env-file .env.dev               # every resort
//	go run ./cmd/cams -env-file .env.dev -only vail    # one resort
//	go run ./cmd/cams -env-file .env.dev -cleanup      # also delete frames past retention
//
// Closed resorts are skipped, as in the hourly run, unless -include-closed is set.
//
// It reads SUPABASE_URL and SUPABASE_SERVICE_ROLE_KEY from the environment or the env file,
// and refuses the production project unless ENV=production.
package main

import (
	"context"
	"flag"
	"log"
	"os"
	"time"

	"powderhoundgo/internal/cams"
	"powderhoundgo/internal/supabase"

	"github.com/joho/godotenv"
)

func main() {
	envFile := flag.String("env-file", ".env", "dotenv file to load if present")
	only := flag.String("only", "", "capture only this resort slug")
	cleanup := flag.Bool("cleanup", false, "also delete frames older than the retention period")
	includeClosed := flag.Bool("include-closed", false, "capture resorts whose status is closed too")
	flag.Parse()

	if err := godotenv.Load(*envFile); err != nil {
		log.Printf("no %s loaded (%v); using the process environment", *envFile, err)
	}
	store, err := supabase.NewCamStore(os.Getenv("ENV") == "production")
	if err != nil {
		log.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	failed, err := cams.RunOnce(ctx, store, *only, *includeClosed)
	if err != nil {
		log.Printf("queue errors: %v", err)
	}
	if *cleanup {
		n, cleanupErr := cams.Cleanup(store, time.Now(), cams.Retention)
		log.Printf("cleanup: deleted %d frames (err: %v)", n, cleanupErr)
		if cleanupErr != nil {
			failed++
		}
	}
	if failed > 0 || err != nil {
		log.Fatalf("%d capture(s) failed", failed)
	}
}
