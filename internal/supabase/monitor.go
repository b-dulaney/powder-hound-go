package supabase

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	storage_go "github.com/supabase-community/storage-go"
	"github.com/supabase-community/supabase-go"
)

// ResortState is the subset of resort_conditions the monitor reads.
type ResortState struct {
	MountainID   int        `json:"mountain_id"`
	DisplayName  string     `json:"display_name"`
	LiftsOpen    *int       `json:"lifts_open"`
	TotalLifts   *int       `json:"total_lifts"`
	RunsOpen     *int       `json:"runs_open"`
	UpdatedAt    *time.Time `json:"updated_at"`
	Status       string     `json:"status"`
	StatusSource string     `json:"status_source"`
}

// StatusUpdate is one call to public.set_resort_status.
type StatusUpdate struct {
	MountainID        int
	Status            string // open | closed | unknown
	Source            string // auto | approved | manual
	Evidence          string
	ExpectedOpenDate  string // YYYY-MM-DD or ""
	ExpectedCloseDate string // YYYY-MM-DD or ""
}

const EvidenceBucket = "monitor-evidence"

// MonitorStore is the database and storage access the scraper monitor needs.
type MonitorStore struct {
	client        *supabase.Client
	storageClient *storage_go.Client
	baseURL       string
}

// NewMonitorStore always talks to the real project in SUPABASE_URL (it is never mocked).
func NewMonitorStore() (*MonitorStore, error) {
	url := os.Getenv("SUPABASE_URL")
	key := os.Getenv("SUPABASE_SERVICE_ROLE_KEY")
	if url == "" || key == "" {
		return nil, fmt.Errorf("SUPABASE_URL and SUPABASE_SERVICE_ROLE_KEY must be set")
	}
	client, err := supabase.NewClient(url, key, nil)
	if err != nil {
		return nil, fmt.Errorf("create supabase client: %w", err)
	}
	return &MonitorStore{
		client:        client,
		storageClient: storage_go.NewClient(url+"/storage/v1", key, nil),
		baseURL:       strings.TrimRight(url, "/"),
	}, nil
}

func (s *MonitorStore) GetResortStates() (map[int]ResortState, error) {
	data, _, err := s.client.From("resort_conditions").
		Select("mountain_id,display_name,lifts_open,total_lifts,runs_open,updated_at,status,status_source", "", false).
		Execute()
	if err != nil {
		return nil, fmt.Errorf("read resort_conditions: %w", err)
	}
	var rows []ResortState
	if err := json.Unmarshal(data, &rows); err != nil {
		return nil, fmt.Errorf("decode resort_conditions: %w", err)
	}
	states := make(map[int]ResortState, len(rows))
	for _, row := range rows {
		states[row.MountainID] = row
	}
	return states, nil
}

// SetResortStatus calls public.set_resort_status. It returns false when nothing changed,
// including when an automatic update is skipped because the status was set by hand.
func (s *MonitorStore) SetResortStatus(u StatusUpdate) (bool, error) {
	params := map[string]any{
		"p_mountain_id": u.MountainID,
		"p_status":      u.Status,
		"p_source":      u.Source,
		"p_evidence":    u.Evidence,
	}
	if u.ExpectedOpenDate != "" {
		params["p_expected_open_date"] = u.ExpectedOpenDate
	}
	if u.ExpectedCloseDate != "" {
		params["p_expected_close_date"] = u.ExpectedCloseDate
	}
	result := s.client.Rpc("set_resort_status", "", params)
	switch strings.TrimSpace(result) {
	case "true":
		return true, nil
	case "false":
		return false, nil
	default:
		return false, fmt.Errorf("set_resort_status: unexpected response %q", truncateForError(result))
	}
}

// UploadEvidence stores an image in the public evidence bucket and returns its public URL,
// for linking from GitHub PRs and issues.
func (s *MonitorStore) UploadEvidence(path string, data []byte, contentType string) (string, error) {
	upsert := true
	_, err := s.storageClient.UploadFile(EvidenceBucket, path, bytes.NewReader(data), storage_go.FileOptions{
		ContentType: &contentType,
		Upsert:      &upsert,
	})
	if err != nil {
		return "", fmt.Errorf("upload evidence %s: %w", path, err)
	}
	return fmt.Sprintf("%s/storage/v1/object/public/%s/%s", s.baseURL, EvidenceBucket, path), nil
}

// ListConfigSlugs returns the scraper config names (file names without .json).
func (s *MonitorStore) ListConfigSlugs() ([]string, error) {
	files, err := s.storageClient.ListFiles("scraping-config", "", storage_go.FileSearchOptions{Limit: 100})
	if err != nil {
		return nil, fmt.Errorf("list scraping-config: %w", err)
	}
	var slugs []string
	for _, f := range files {
		if strings.HasSuffix(f.Name, ".json") {
			slugs = append(slugs, strings.TrimSuffix(f.Name, ".json"))
		}
	}
	return slugs, nil
}

func (s *MonitorStore) GetConfig(slug string) (ScrapingConfig, error) {
	var config ScrapingConfig
	data, err := s.storageClient.DownloadFile("scraping-config", slug+".json")
	if err != nil {
		return config, fmt.Errorf("download config %s: %w", slug, err)
	}
	if err := json.Unmarshal(data, &config); err != nil {
		return config, fmt.Errorf("decode config %s: %w", slug, err)
	}
	return config, nil
}

func truncateForError(s string) string {
	if len(s) > 300 {
		return s[:300] + "…"
	}
	return s
}
