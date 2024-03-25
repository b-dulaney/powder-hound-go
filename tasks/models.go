package tasks

type ConditionsConfig struct {
	ConditionsSelector  string `json:"conditionsSelector"`
	BaseDepthSelector   string `json:"baseDepthSelector"`
	SnowpackSelector    string `json:"snowpackSelector"`
	SeasonTotalSelector string `json:"seasonTotalSelector"`
	Snow24Selector      string `json:"snow24Selector"`
	Snow48Selector      string `json:"snow48Selector"`
	Snow7DaySelector    string `json:"snow7DaySelector"`
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

type Config struct {
	ID            int              `json:"id"`
	Name          string           `json:"name"`
	SeparateURLs  bool             `json:"separateURLs"`
	ClickSelector string           `json:"clickSelector"`
	ConditionsURL string           `json:"conditionsURL"`
	TerrainURL    string           `json:"terrainURL"`
	Conditions    ConditionsConfig `json:"conditions"`
	Terrain       TerrainConfig    `json:"terrain"`
}

type ScrapingRequestBody struct {
	MountainName string `json:"mountainName"`
}
