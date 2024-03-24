package main

import (
	"log"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
	"github.com/supabase-community/supabase-go"
)

func removeNonNumericCharacters(input string) string {
	reg, err := regexp.Compile("[^0-9]+")
	if err != nil {
		log.Fatal(err)
	}
	processedString := reg.ReplaceAllString(input, "")

	return processedString
}

func removeDenominator(input string) string {
	input = strings.Split(input, "/")[0]
	input = strings.Split(input, "of")[0]
	return removeNonNumericCharacters(input)
}

func convertStringToInt(input string) int {
	var cleanedString = removeNonNumericCharacters(input)
	result, err := strconv.Atoi(cleanedString)
	if err != nil {
		log.Fatal(err)
	}
	return result
}

func initializeSupabase() *supabase.Client {
	err := godotenv.Load("../.env")
	if err != nil {
		log.Fatal("Error loading .env file")
	}
	SUPABASE_URL := os.Getenv("SUPABASE_URL")
	SUPABASE_ANON_KEY := os.Getenv("SUPABASE_ANON_KEY")
	client, clientErr := supabase.NewClient(SUPABASE_URL, SUPABASE_ANON_KEY, nil)
	if clientErr != nil {
		log.Fatal("Error creating supabase client")
	}
	return client
}
