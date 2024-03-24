package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
)

func scrapingHandler(w http.ResponseWriter, req *http.Request) {
	var body ScrapingRequestBody
	err := json.NewDecoder(req.Body).Decode(&body)
	if err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}
	mountainName := body.MountainName
	configPath := fmt.Sprintf("./config/%s.json", mountainName)

	success := scrapeResortData(&configPath)
	if !success {
		log.Printf("Failed to scrape %s", mountainName)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	log.Printf("Scraping successful for %s", mountainName)

	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, "Scraping completed successfully")
}

func routeHandler(w http.ResponseWriter, req *http.Request) {
	SUPABASE_SERVICE_ROLE_KEY := os.Getenv("SUPABASE_SERVICE_ROLE_KEY")
	if req.Header.Get("Authorization") != "Bearer "+SUPABASE_SERVICE_ROLE_KEY {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	switch req.Method {
	case "POST":
		scrapingHandler(w, req)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}
