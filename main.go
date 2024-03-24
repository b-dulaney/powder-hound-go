package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

func handleResortScraping(c echo.Context) error {
	SUPABASE_SERVICE_ROLE_KEY := os.Getenv("SUPABASE_SERVICE_ROLE_KEY")
	if c.Request().Header.Get("Authorization") != "Bearer "+SUPABASE_SERVICE_ROLE_KEY {
		return c.String(http.StatusUnauthorized, "Unauthorized")
	}

	var body ScrapingRequestBody
	err := json.NewDecoder(c.Request().Body).Decode(&body)
	if err != nil {
		return c.String(http.StatusBadRequest, "Invalid request body")
	}
	mountainName := body.MountainName
	configPath := fmt.Sprintf("./config/%s.json", mountainName)

	success := scrapeResortData(&configPath)
	if !success {
		log.Printf("Failed to scrape %s", mountainName)
		return c.String(http.StatusInternalServerError, "Failed to scrape data")
	}

	log.Printf("Scraping successful for %s", mountainName)

	return c.String(http.StatusOK, "Scraping successful")
}

func main() {
	loadEnvironmentVariables()
	e := echo.New()
	e.POST("/", handleResortScraping)

	e.Use(middleware.Logger())
	e.Use(middleware.Recover())

	e.Logger.Fatal(e.Start(":8080"))
}
