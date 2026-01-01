package supabase

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"

	storage_go "github.com/supabase-community/storage-go"
	"github.com/supabase-community/supabase-go"
)

func NewSupabaseService() SupabaseClient {
	SUPABASE_URL := os.Getenv("SUPABASE_URL")
	SUPABASE_SERVICE_ROLE_KEY := os.Getenv("SUPABASE_SERVICE_ROLE_KEY")
	ENV := os.Getenv("ENV")

	storageUrl := fmt.Sprintf("%s/storage/v1", SUPABASE_URL)
	storageClient := storage_go.NewClient(storageUrl, SUPABASE_SERVICE_ROLE_KEY, nil)

	if ENV == "production" {
		client, clientErr := supabase.NewClient(SUPABASE_URL, SUPABASE_SERVICE_ROLE_KEY, nil)
		if clientErr != nil {
			log.Fatalf("Error creating supabase client: %s", clientErr)
		}

		return &SupabaseService{client, storageClient}
	}

	return &MockSupabaseService{storageClient}
}

func (s *SupabaseService) UpsertResortConditionsData(data map[string]interface{}) error {
	_, _, err := s.client.From("resort_conditions").Upsert(data, "mountain_id", "*", "estimated").Execute()
	if err != nil {
		log.Printf("Failed to upsert data: %s", err)
	}
	return err
}

func (s *SupabaseService) InsertScrapingStatus(data ScrapingStatusData) error {
	jsonData := map[string]interface{}{
		"display_name": data.MountainName,
		"success":      data.Success,
		"error":        data.Error,
	}
	_, _, err := s.client.From("scraping_status").Insert(jsonData, false, "id", "*", "").Execute()
	if err != nil {
		log.Printf("Failed to insert scraping status: %s", err)
	}
	return err
}

func (s *SupabaseService) GetUserOvernightAlerts() []UserOvernightAlert {
	userAlertsResponse := s.client.Rpc("group_overnight_snowfall_alert_data", "", nil)

	var userAlerts []UserOvernightAlert
	err := json.Unmarshal([]byte(userAlertsResponse), &userAlerts)
	if err != nil {
		log.Fatal(err)
	}

	return userAlerts
}

func (s *SupabaseService) GetUserForecastAlerts() []UserForecastAlert {
	userAlertsResponse := s.client.Rpc("group_24h_forecast_alert_data", "", nil)

	var userAlerts []UserForecastAlert
	err := json.Unmarshal([]byte(userAlertsResponse), &userAlerts)
	if err != nil {
		log.Fatal(err)
	}

	return userAlerts
}

func (s *SupabaseService) GetConfigByName(name string) ScrapingConfig {
	fileName := fmt.Sprintf("%s.json", name)
	log.Print(fileName)
	result, err := s.storageClient.DownloadFile("scraping-config", fileName)

	if err != nil {
		log.Fatalf("Error fetching config by name: %v", err)
	}
	var config ScrapingConfig
	jsonErr := json.Unmarshal([]byte(result), &config)
	if jsonErr != nil {
		log.Fatal(err)
	}

	return config
}

func (s *SupabaseService) GetAllMountainObjectNames() []string {
	results, err := s.storageClient.ListFiles("scraping-config", "", storage_go.FileSearchOptions{
		Limit: 50,
	})

	if err != nil {
		log.Fatalf("Error getting all mountain object names: %v", err)
	}
	var names []string
	for _, result := range results {
		trimmedName := strings.Split(result.Name, ".")[0]
		log.Print(trimmedName)
		names = append(names, trimmedName)
	}

	return names
}

func (s *SupabaseService) UpsertAvalancheForecast(data map[string]interface{}) error {
	_, _, err := s.client.From("avalanche_forecasts").Upsert(data, "mountain_id", "*", "estimated").Execute()
	if err != nil {
		log.Printf("Failed to upsert avalanche forecast: %s", err)
	}
	return err
}

func (s *SupabaseService) GetMountainsWithAvalancheForecasts() ([]MountainCoordinates, error) {
	data, _, err := s.client.From("mountains").Select("mountain_id, lat, lon", "", false).Eq("location_type", "backcountry").Execute()
	if err != nil {
		log.Printf("Failed to get backcountry mountains: %s", err)
		return nil, err
	}

	var mountains []MountainCoordinates
	if err := json.Unmarshal(data, &mountains); err != nil {
		log.Printf("Failed to unmarshal mountains: %s", err)
		return nil, err
	}

	return mountains, nil
}

/** Mock Supabase Service Implementations **/
func (s *MockSupabaseService) UpsertResortConditionsData(data map[string]interface{}) error {
	log.Printf("Mock upsert data: %v", data)
	return nil
}

func (s *MockSupabaseService) GetUserOvernightAlerts() []UserOvernightAlert {
	return []UserOvernightAlert{
		{
			Email: "test@powderhound.io",
			Alerts: []OvernightAlert{
				{
					Location: "Test Location",
					Snowfall: 12,
				},
			},
		},
	}
}

func (s *MockSupabaseService) GetUserForecastAlerts() []UserForecastAlert {
	return []UserForecastAlert{
		{
			Email: "test@powderhound.io",
			Alerts: []ForecastAlert{
				{
					Location: "Test Location",
					Snowfall: 12,
				},
			},
		},
	}
}

func (s *MockSupabaseService) InsertScrapingStatus(data ScrapingStatusData) error {
	log.Printf("Mock insert scraping status: %v", data)
	return nil
}

func (s *MockSupabaseService) GetConfigByName(name string) ScrapingConfig {
	fileName := fmt.Sprintf("%s.json", name)
	log.Printf("Fetching config for mountain: %s", name)
	log.Printf("Fetching config by formatted file name: %s", fileName)
	result, err := s.storageClient.DownloadFile("scraping-config-dev", fileName)

	if err != nil {
		log.Fatalf("Error fetching config by name: %v", err)
	}
	var config ScrapingConfig
	jsonErr := json.Unmarshal([]byte(result), &config)
	if jsonErr != nil {
		log.Fatal(err)
	}

	return config
}

func (s *MockSupabaseService) GetAllMountainObjectNames() []string {
	results, err := s.storageClient.ListFiles("scraping-config-dev", "", storage_go.FileSearchOptions{
		Limit: 50,
	})

	if err != nil {
		log.Fatalf("Error getting all mountain object names: %v", err)
	}
	var names []string
	for _, result := range results {
		log.Printf("Result: %v", result.Name)
		trimmedName := strings.Split(result.Name, ".")[0]
		log.Print(trimmedName)
		if trimmedName != "" {
			names = append(names, trimmedName)

		}
	}

	return names
}

func (s *MockSupabaseService) UpsertAvalancheForecast(data map[string]interface{}) error {
	log.Printf("Mock upsert avalanche forecast: %v", data)
	return nil
}

func (s *MockSupabaseService) GetMountainsWithAvalancheForecasts() ([]MountainCoordinates, error) {
	// Return mock data for development testing
	return []MountainCoordinates{
		{MountainID: 1, Lat: 39.6403, Lon: -105.8719}, // Loveland
		{MountainID: 2, Lat: 39.4817, Lon: -106.0384}, // Breckenridge
	}, nil
}
