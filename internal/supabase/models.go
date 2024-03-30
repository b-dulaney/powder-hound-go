package supabase

import "github.com/supabase-community/supabase-go"

type OvernightAlert struct {
	Location string `json:"display_name"`
	Snowfall int    `json:"snow_past_24h"`
}

type ForecastAlert struct {
	Location string `json:"display_name"`
	Snowfall int    `json:"snow_next_24h"`
}

type UserOvernightAlert struct {
	Email  string           `json:"email"`
	Alerts []OvernightAlert `json:"alerts"`
}

type UserForecastAlert struct {
	Email  string          `json:"email"`
	Alerts []ForecastAlert `json:"alerts"`
}

type SupabaseClient interface {
	UpsertResortConditionsData(data map[string]interface{}) error
	GetUserOvernightAlerts() []UserOvernightAlert
	GetUserForecastAlerts() []UserForecastAlert
}

type SupabaseService struct {
	client *supabase.Client
}

type MockSupabaseService struct{}
