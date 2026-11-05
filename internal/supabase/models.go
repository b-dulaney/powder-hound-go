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
	// Optional, for pages whose open counts don't include the total ("12 of 147" is read
	// automatically). A total selector's text is the total ("147") or an "open of total"
	// count ("12 / 147"); a row selector matches every lift or run row, open or not, and the
	// total is how many match.
	LiftTotalSelector string `json:"liftTotalSelector,omitempty"`
	RunTotalSelector  string `json:"runTotalSelector,omitempty"`
	LiftRowSelector   string `json:"liftRowSelector,omitempty"`
	RunRowSelector    string `json:"runRowSelector,omitempty"`
}

type ScrapingConfig struct {
	ID            int              `json:"id"`
	Name          string           `json:"name"`
	SeparateURLs  bool             `json:"separateURLs"`
	ClickSelector string           `json:"clickSelector"`
	ConditionsURL string           `json:"conditionsURL"`
	TerrainURL    string           `json:"terrainURL"`
	Conditions    ConditionsConfig `json:"conditions"`
	Terrain       TerrainConfig    `json:"terrain"`
}

// MountainCoordinates represents a mountain's location for avalanche forecasting
type MountainCoordinates struct {
	MountainID int     `json:"mountain_id"`
	Lat        float64 `json:"lat"`
	Lon        float64 `json:"lon"`
}

type SupabaseClient interface {
	UpsertResortConditionsData(data map[string]interface{}) error
	GetUserOvernightAlerts() ([]UserOvernightAlert, error)
	GetUserForecastAlerts() ([]UserForecastAlert, error)
	// RecordJobRun reports a scheduled job's outcome (runErr nil = success) for the health
	// alerts in public.job_heartbeats. Failures to record are logged, never fatal.
	RecordJobRun(job string, runErr error)
	InsertScrapingStatus(data ScrapingStatusData) error
	GetConfigByName(name string) ScrapingConfig
	GetAllMountainObjectNames() []string
	// GetResortStatuses returns resort_conditions.status (open | closed | unknown) by mountain_id.
	GetResortStatuses() (map[int]string, error)
	// Avalanche forecast methods
	UpsertAvalancheForecast(data map[string]interface{}) error
	GetMountainsWithAvalancheForecasts() ([]MountainCoordinates, error)
}

type SupabaseService struct {
	client        *supabase.Client
	storageClient *storage_go.Client
}

type MockSupabaseService struct {
	storageClient *storage_go.Client
}
