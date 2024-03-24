package main

import (
	"encoding/json"
	"io"
	"log"
	"os"
)

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
	TerrainSelector   string `json:"terrainSelector"`
	RunsOpenSelector  string `json:"runsOpenSelector"`
	LiftsOpenSelector string `json:"liftsOpenSelector"`
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

func fetchConfig(configPath *string) Config {
	if *configPath == "" {
		log.Fatal("Config path is required")
	}
	configFile, configErr := os.Open(*configPath)

	if configErr != nil {
		log.Fatal(configErr)
	}

	byteValue, _ := io.ReadAll(configFile)
	var config Config
	json.Unmarshal(byteValue, &config)

	defer configFile.Close()
	return config
}
