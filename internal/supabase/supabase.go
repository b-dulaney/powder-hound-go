package supabase

import (
	"encoding/json"
	"log"
	"os"

	"github.com/supabase-community/supabase-go"
)

func NewSupabaseService() SupabaseClient {
	SUPABASE_URL := os.Getenv("SUPABASE_URL")
	SUPABASE_SERVICE_ROLE_KEY := os.Getenv("SUPABASE_SERVICE_ROLE_KEY")
	ENV := os.Getenv("ENV")
	if ENV == "production" {
		client, clientErr := supabase.NewClient(SUPABASE_URL, SUPABASE_SERVICE_ROLE_KEY, nil)
		if clientErr != nil {
			log.Fatalf("Error creating supabase client: %s", clientErr)
		}
		return &SupabaseService{client}
	}

	return &MockSupabaseService{}
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
