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

func (s *MockSupabaseService) UpsertResortConditionsData(data map[string]interface{}) error {
	log.Printf("Mock upsert data: %v", data)
	return nil
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

// Mock implementation of GetUserOvernightAlerts for testing and local development
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

// Mock implementation of GetUserForecastAlerts for testing and local development
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
