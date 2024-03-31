package main

import (
	"os"
	"os/signal"
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

	cron := util.InitializeEmailCronTasks(client, supabase)

	go cron.Start()
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	<-sig

	select {}
}
