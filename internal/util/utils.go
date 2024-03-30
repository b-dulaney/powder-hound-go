package util

import (
	"log"

	"github.com/joho/godotenv"
)

func LoadEnvironmentVariables() {
	err := godotenv.Load("../.env")
	if err != nil {
		log.Fatalf("Error loading .env file: %s", err)
	}
}
