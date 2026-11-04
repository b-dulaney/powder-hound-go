package main

import (
	"log"
	"os"
	"os/signal"
	"powderhoundgo/internal/supabase"
	"powderhoundgo/internal/util"
	"syscall"

	"github.com/hibiken/asynq"
)

func main() {
	util.LoadEnvironmentVariables()
	// Outside production the email schedule runs every minute, so never read real alerts
	// (and email real users) from the database; the mock returns a test recipient.
	if os.Getenv("ENV") != "production" {
		os.Setenv("SUPABASE_MOCK", "true")
		log.Printf("Email service: using the mock Supabase client (ENV=%q)", os.Getenv("ENV"))
	}
	supabase := supabase.NewSupabaseService()

	redisHost := os.Getenv("REDIS_HOST")
	if redisHost == "" {
		redisHost = "localhost"
	}

	client := asynq.NewClient(asynq.RedisClientOpt{Addr: redisHost + ":6379", Password: "", DB: 0})
	defer client.Close()

	cron := util.InitializeEmailCronTasks(client, supabase)

	go cron.Start()
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	<-sig

	select {}
}
