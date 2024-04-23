package supabase

import (
	storage_go "github.com/supabase-community/storage-go"
	"github.com/supabase-community/supabase-go"
)

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

type ScrapingStatusData struct {
	MountainName string
	Success      bool
	Error        string
}

type ConditionsConfig struct {
	ConditionsSelector  string `json:"conditionsSelector"`
	BaseDepthSelector   string `json:"baseDepthSelector"`
	SnowpackSelector    string `json:"snowpackSelector"`
	SeasonTotalSelector string `json:"seasonTotalSelector"`
	Snow24Selector      string `json:"snow24Selector"`
	Snow48Selector      string `json:"snow48Selector"`
	Snow7DaySelector    string `json:"snow7DaySelector"`
	WaitForSelector     string `json:"waitForSelector"`
}

type TerrainConfig struct {
	SumRunsFromMultipleSources bool   `json:"sumRunsFromMultipleSources"`
	CountLifts                 bool   `json:"countLifts"`
	CountRuns                  bool   `json:"countRuns"`
	TerrainSelector            string `json:"terrainSelector"`
	RunsOpenSelector           string `json:"runsOpenSelector"`
	LiftsOpenSelector          string `json:"liftsOpenSelector"`
	LiftStatusSelector         string `json:"liftStatusSelector"`
	RunStatusSelector          string `json:"runStatusSelector"`
	RunClickInteraction        bool   `json:"runClickInteraction"`
	RunClickSelector           string `json:"runClickSelector"`
}

type ScrapingConfig struct {
	ID            int              `json:"id"`
	Name          string           `json:"name"`
	ClosingDate   string           `json:"closingDate"`
	SeparateURLs  bool             `json:"separateURLs"`
	ClickSelector string           `json:"clickSelector"`
	ConditionsURL string           `json:"conditionsURL"`
	TerrainURL    string           `json:"terrainURL"`
	Conditions    ConditionsConfig `json:"conditions"`
	Terrain       TerrainConfig    `json:"terrain"`
}

type SupabaseClient interface {
	UpsertResortConditionsData(data map[string]interface{}) error
	GetUserOvernightAlerts() []UserOvernightAlert
	GetUserForecastAlerts() []UserForecastAlert
	InsertScrapingStatus(data ScrapingStatusData) error
	GetConfigByName(name string) ScrapingConfig
	GetAllMountainObjectNames() []string
}

type SupabaseService struct {
	client        *supabase.Client
	storageClient *storage_go.Client
}

type MockSupabaseService struct {
	storageClient *storage_go.Client
}
