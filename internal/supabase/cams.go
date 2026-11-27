package supabase

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	storage_go "github.com/supabase-community/storage-go"
	"github.com/supabase-community/supabase-go"
	"github.com/supabase/postgrest-go"
)

// CamBucket is the public bucket holding captured cam frames.
const CamBucket = "resort-cams"

// camCacheControl is the Cache-Control header served with frames. Paths include the frame's
// hash, so a path's content never changes. storage-go passes this through as the header
// verbatim (unlike supabase-js, which takes seconds), so it must be a full header value; a
// bare number is stored as is and served as "private, no-cache".
const camCacheControl = "public, max-age=31536000, immutable"

// Cam is a row of public.resort_cams, as returned by sync_resort_cams.
type Cam struct {
	ID         int64  `json:"id"`
	MountainID int    `json:"mountain_id"`
	Key        string `json:"key"`
	Active     bool   `json:"active"`
}

// CamImage is a row of public.resort_cam_images.
type CamImage struct {
	ID              int64      `json:"id,omitempty"`
	CamID           int64      `json:"cam_id"`
	CapturedAt      time.Time  `json:"captured_at"`
	SourceUpdatedAt *time.Time `json:"source_updated_at,omitempty"`
	Path            string     `json:"path"`
	ThumbPath       string     `json:"thumb_path"`
	Width           int        `json:"width"`
	Height          int        `json:"height"`
	SHA256          string     `json:"sha256"`
}

// CamStore is the database and storage access for cam capture. Like MonitorStore it always
// talks to the real project in SUPABASE_URL. storage-go sets per-upload headers on a shared
// transport, so use one CamStore per task, not one shared across goroutines.
type CamStore struct {
	client        *supabase.Client
	storageClient *storage_go.Client
}

// NewCamStore connects to SUPABASE_URL; production must be true to connect to production.
func NewCamStore(production bool) (*CamStore, error) {
	url := os.Getenv("SUPABASE_URL")
	key := os.Getenv("SUPABASE_SERVICE_ROLE_KEY")
	if url == "" || key == "" {
		return nil, fmt.Errorf("SUPABASE_URL and SUPABASE_SERVICE_ROLE_KEY must be set")
	}
	if err := CheckTarget(url, production); err != nil {
		return nil, err
	}
	client, err := supabase.NewClient(url, key, nil)
	if err != nil {
		return nil, fmt.Errorf("create supabase client: %w", err)
	}
	return &CamStore{
		client:        client,
		storageClient: storage_go.NewClient(strings.TrimRight(url, "/")+"/storage/v1", key, nil),
	}, nil
}

// ListConfigSlugs returns the scraper config names (file names without .json).
func (s *CamStore) ListConfigSlugs() ([]string, error) {
	files, err := s.storageClient.ListFiles("scraping-config", "", storage_go.FileSearchOptions{Limit: 100})
	if err != nil {
		return nil, fmt.Errorf("list scraping-config: %w", err)
	}
	var slugs []string
	for _, f := range files {
		if slug, ok := strings.CutSuffix(f.Name, ".json"); ok {
			slugs = append(slugs, slug)
		}
	}
	return slugs, nil
}

// ReadConfig returns a scraper config file's raw JSON.
func (s *CamStore) ReadConfig(slug string) ([]byte, error) {
	data, err := s.storageClient.DownloadFile("scraping-config", slug+".json")
	if err != nil {
		return nil, fmt.Errorf("download config %s: %w", slug, err)
	}
	return data, nil
}

// ResortStatuses maps mountain_id to resort_conditions.status.
func (s *CamStore) ResortStatuses() (map[int]string, error) {
	data, _, err := s.client.From("resort_conditions").Select("mountain_id,status", "", false).Execute()
	if err != nil {
		return nil, fmt.Errorf("read resort statuses: %w", err)
	}
	var rows []struct {
		MountainID int    `json:"mountain_id"`
		Status     string `json:"status"`
	}
	if err := json.Unmarshal(data, &rows); err != nil {
		return nil, fmt.Errorf("decode resort statuses: %w (body: %s)", err, truncateForError(string(data)))
	}
	statuses := make(map[int]string, len(rows))
	for _, r := range rows {
		statuses[r.MountainID] = r.Status
	}
	return statuses, nil
}

// SyncCams makes a resort's resort_cams rows match its config's cams array (passed through
// as is) and returns the resort's active cams.
func (s *CamStore) SyncCams(mountainID int, cams json.RawMessage) ([]Cam, error) {
	body := s.client.Rpc("sync_resort_cams", "", map[string]any{
		"p_mountain_id": mountainID,
		"p_cams":        cams,
	})
	var rows []Cam
	if err := json.Unmarshal([]byte(body), &rows); err != nil {
		return nil, fmt.Errorf("sync_resort_cams %d: unexpected response %q", mountainID, truncateForError(body))
	}
	return rows, nil
}

// LatestImageSHA returns the sha256 of a cam's newest stored frame, or "" if it has none.
func (s *CamStore) LatestImageSHA(camID int64) (string, error) {
	data, _, err := s.client.From("resort_cam_images").
		Select("sha256", "", false).
		Eq("cam_id", strconv.FormatInt(camID, 10)).
		Order("captured_at", &postgrest.OrderOpts{Ascending: false}).
		Limit(1, "").
		Execute()
	if err != nil {
		return "", fmt.Errorf("read latest frame for cam %d: %w", camID, err)
	}
	var rows []struct {
		SHA256 string `json:"sha256"`
	}
	if err := json.Unmarshal(data, &rows); err != nil {
		return "", fmt.Errorf("decode latest frame for cam %d: %w", camID, err)
	}
	if len(rows) == 0 {
		return "", nil
	}
	return rows[0].SHA256, nil
}

// UploadFrame stores a JPEG in the cam bucket. It never overwrites: paths are unique.
func (s *CamStore) UploadFrame(path string, data []byte) error {
	contentType, cacheControl, upsert := "image/jpeg", camCacheControl, false
	_, err := s.storageClient.UploadFile(CamBucket, path, bytes.NewReader(data), storage_go.FileOptions{
		ContentType:  &contentType,
		CacheControl: &cacheControl,
		Upsert:       &upsert,
	})
	if err != nil {
		return fmt.Errorf("upload %s: %w", path, err)
	}
	return nil
}

// RemoveFrames deletes objects from the cam bucket. Paths that don't exist are ignored.
func (s *CamStore) RemoveFrames(paths []string) error {
	if len(paths) == 0 {
		return nil
	}
	if _, err := s.storageClient.RemoveFile(CamBucket, paths); err != nil {
		return fmt.Errorf("remove %d frame objects: %w", len(paths), err)
	}
	return nil
}

// InsertImage records a stored frame.
func (s *CamStore) InsertImage(img CamImage) error {
	_, _, err := s.client.From("resort_cam_images").Insert(img, false, "", "minimal", "").Execute()
	if err != nil {
		return fmt.Errorf("insert frame %s: %w", img.Path, err)
	}
	return nil
}

// ExpiredImages returns up to limit frames captured before cutoff, oldest first.
func (s *CamStore) ExpiredImages(cutoff time.Time, limit int) ([]CamImage, error) {
	data, _, err := s.client.From("resort_cam_images").
		Select("id,cam_id,captured_at,path,thumb_path", "", false).
		Lt("captured_at", cutoff.UTC().Format(time.RFC3339)).
		Order("captured_at", &postgrest.OrderOpts{Ascending: true}).
		Limit(limit, "").
		Execute()
	if err != nil {
		return nil, fmt.Errorf("read expired frames: %w", err)
	}
	var rows []CamImage
	if err := json.Unmarshal(data, &rows); err != nil {
		return nil, fmt.Errorf("decode expired frames: %w", err)
	}
	return rows, nil
}

// DeleteImages deletes frame rows by id.
func (s *CamStore) DeleteImages(ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	values := make([]string, len(ids))
	for i, id := range ids {
		values[i] = strconv.FormatInt(id, 10)
	}
	if _, _, err := s.client.From("resort_cam_images").Delete("minimal", "").In("id", values).Execute(); err != nil {
		return fmt.Errorf("delete %d frame rows: %w", len(ids), err)
	}
	return nil
}

// RecordCapture calls public.record_cam_capture: a nil error records a success.
func (s *CamStore) RecordCapture(camID int64, captureErr error) error {
	params := map[string]any{"p_cam_id": camID}
	if captureErr != nil {
		params["p_error"] = captureErr.Error()
	}
	if body := strings.TrimSpace(s.client.Rpc("record_cam_capture", "", params)); body != "" && body != "null" {
		return fmt.Errorf("record_cam_capture %d: unexpected response %q", camID, truncateForError(body))
	}
	return nil
}

func (s *CamStore) RecordJobRun(job string, runErr error) {
	recordJobRun(s.client, job, runErr)
}
