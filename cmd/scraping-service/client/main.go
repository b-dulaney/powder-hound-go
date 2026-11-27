package main

import (
	"os"
	"os/signal"
	"powderhoundgo/internal/cams"
	"powderhoundgo/internal/supabase"
	"powderhoundgo/internal/util"
	"syscall"

	"github.com/hibiken/asynq"
)

func main() {
	util.LoadEnvironmentVariables()
	supabase := supabase.NewSupabaseService()

	redisHost := os.Getenv("REDIS_HOST")
	if redisHost == "" {
		redisHost = "localhost"
	}

	client := asynq.NewClient(asynq.RedisClientOpt{Addr: redisHost + ":6379", Password: "", DB: 0})
	defer client.Close()

	cron := util.InitializeScrapingCronTasks(client, supabase)
	// Registered here, not in util, so the email binaries don't compile cam capture.
	cams.AddCronTasks(cron, client)

	go cron.Start()
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	<-sig

	select {}
}
