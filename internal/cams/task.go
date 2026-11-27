package cams

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"powderhoundgo/internal/supabase"
	"powderhoundgo/internal/tasks"

	"github.com/hibiken/asynq"
	"github.com/robfig/cron/v3"
)

// The handlers live here, not in internal/tasks, so the email service, which imports tasks,
// doesn't compile cam capture and its image dependencies.

const (
	// Retention is how long frames are kept; resort snow history on the site covers 7 days.
	Retention = 7 * 24 * time.Hour

	// Job names in public.job_heartbeats.
	captureJob = "cam-capture"
	cleanupJob = "cam-cleanup"

	// Production defaults (America/Denver). :50 catches the stakes cleared on the hour (5am,
	// noon, 4pm) at their fullest.
	defaultCaptureSchedule = "50 * * * *"
	defaultCleanupSchedule = "30 3 * * *"
)

// Enqueuer is the part of *asynq.Client the queue step uses.
type Enqueuer interface {
	Enqueue(task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error)
}

// resortConfig is the part of a scraper config cams need.
type resortConfig struct {
	ID   int             `json:"id"`
	Cams json.RawMessage `json:"cams"`
}

type camConfig struct {
	Key    string `json:"key"`
	Source string `json:"source"`
	URL    string `json:"url"`
}

// QueueCaptures syncs every resort's cams from its config into resort_cams and queues a
// capture task per active cam. Closed resorts are synced but not captured. Each resort is
// handled on its own, so one bad config doesn't stop the rest; the errors are joined.
func QueueCaptures(store Store, q Enqueuer, now time.Time) error {
	slugs, err := store.ListConfigSlugs()
	if err != nil {
		return err
	}
	statuses, err := store.ResortStatuses()
	if err != nil {
		// Fail open, like the resort scrape: capturing a closed resort is harmless.
		log.Printf("[cams] couldn't read resort statuses, capturing every resort: %v", err)
	}

	var errs []error
	queued := 0
	for _, slug := range slugs {
		n, err := queueResort(store, q, slug, statuses, now)
		queued += n
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", slug, err))
		}
	}
	log.Printf("[cams] queued %d captures across %d resorts (%d errors)", queued, len(slugs), len(errs))
	return errors.Join(errs...)
}

func queueResort(store Store, q Enqueuer, slug string, statuses map[int]string, now time.Time) (int, error) {
	raw, err := store.ReadConfig(slug)
	if err != nil {
		return 0, err
	}
	var cfg resortConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return 0, fmt.Errorf("parse config: %w", err)
	}
	if cfg.ID == 0 {
		return 0, errors.New("config has no id")
	}
	camsJSON := cfg.Cams
	if len(camsJSON) == 0 || string(camsJSON) == "null" {
		camsJSON = json.RawMessage("[]")
	}
	var configured []camConfig
	if err := json.Unmarshal(camsJSON, &configured); err != nil {
		return 0, fmt.Errorf("parse cams: %w", err)
	}
	byKey := make(map[string]camConfig, len(configured))
	for _, c := range configured {
		byKey[c.Key] = c
	}

	active, err := store.SyncCams(cfg.ID, camsJSON)
	if err != nil {
		return 0, err
	}
	if statuses[cfg.ID] == "closed" {
		return 0, nil
	}

	queued := 0
	var errs []error
	for _, cam := range active {
		c, ok := byKey[cam.Key]
		if !ok {
			continue
		}
		payload, err := json.Marshal(CaptureRequest{CamID: cam.ID, Slug: slug, Key: cam.Key, Source: c.Source, URL: c.URL})
		if err != nil {
			return queued, err
		}
		// One capture per cam per hour, even if the queue step runs twice.
		id := fmt.Sprintf("cam-capture:%d:%s", cam.ID, now.UTC().Format("2006010215"))
		_, err = q.Enqueue(asynq.NewTask(tasks.TypeCamCapture, payload),
			asynq.TaskID(id), asynq.MaxRetry(2), asynq.Timeout(2*time.Minute), asynq.Retention(2*time.Hour))
		switch {
		case err == nil:
			queued++
		case errors.Is(err, asynq.ErrTaskIDConflict):
		default:
			errs = append(errs, fmt.Errorf("enqueue %s: %w", cam.Key, err))
		}
	}
	return queued, errors.Join(errs...)
}

func production() bool { return os.Getenv("ENV") == "production" }

// One fetcher for the process, so fetched intermediate certificates are cached.
var sharedFetcher = sync.OnceValue(NewFetcher)

// HandleCaptureTask captures one cam (tasks.TypeCamCapture) and records the outcome in
// resort_cam_status. A store is created per task: storage-go isn't safe to share.
func HandleCaptureTask(ctx context.Context, t *asynq.Task) error {
	var req CaptureRequest
	if err := json.Unmarshal(t.Payload(), &req); err != nil {
		return fmt.Errorf("decode payload: %w: %w", err, asynq.SkipRetry)
	}
	store, err := supabase.NewCamStore(production())
	if err != nil {
		return fmt.Errorf("cam store: %w: %w", err, asynq.SkipRetry)
	}
	c := &Capturer{Store: store, Fetch: sharedFetcher().Fetch, Now: time.Now}
	stored, err := c.Capture(ctx, req)
	if recErr := store.RecordCapture(req.CamID, err); recErr != nil {
		log.Printf("[cams] %v", recErr)
	}
	if err != nil {
		err = fmt.Errorf("capture %s/%s: %w", req.Slug, req.Key, err)
		if isPermanent(err) {
			return fmt.Errorf("%w: %w", err, asynq.SkipRetry)
		}
		return err
	}
	if stored {
		log.Printf("[cams] stored a new frame for %s/%s", req.Slug, req.Key)
	} else {
		log.Printf("[cams] %s/%s unchanged since the last frame", req.Slug, req.Key)
	}
	return nil
}

// HandleCleanupTask deletes expired frames (tasks.TypeCamCleanup).
func HandleCleanupTask(ctx context.Context, t *asynq.Task) error {
	store, err := supabase.NewCamStore(production())
	if err != nil {
		return fmt.Errorf("cam store: %w: %w", err, asynq.SkipRetry)
	}
	n, err := Cleanup(store, time.Now(), Retention)
	store.RecordJobRun(cleanupJob, err)
	if err != nil {
		return fmt.Errorf("cam cleanup after %d frames: %w", n, err)
	}
	log.Printf("[cams] deleted %d frames older than %s", n, Retention)
	return nil
}

// AddCronTasks schedules the capture queue step and the daily cleanup. CAM_CAPTURE_SCHEDULE
// and CAM_CLEANUP_SCHEDULE override the times (cron specs, in the cron's America/Denver
// location); "off" disables one. Outside production they only run when set.
func AddCronTasks(c *cron.Cron, client *asynq.Client) {
	addCron(c, "CAM_CAPTURE_SCHEDULE", defaultCaptureSchedule, func() {
		store, err := supabase.NewCamStore(production())
		if err != nil {
			log.Printf("[cams] capture queue: %v", err)
			return
		}
		store.RecordJobRun(captureJob, QueueCaptures(store, client, time.Now()))
	})
	addCron(c, "CAM_CLEANUP_SCHEDULE", defaultCleanupSchedule, func() {
		id := "cam-cleanup:" + time.Now().UTC().Format("2006-01-02")
		_, err := client.Enqueue(asynq.NewTask(tasks.TypeCamCleanup, nil),
			asynq.TaskID(id), asynq.MaxRetry(2), asynq.Timeout(10*time.Minute), asynq.Retention(24*time.Hour))
		if err != nil && !errors.Is(err, asynq.ErrTaskIDConflict) {
			log.Printf("[cams] enqueue cleanup: %v", err)
		}
	})
}

func addCron(c *cron.Cron, envVar, productionDefault string, job func()) {
	spec := os.Getenv(envVar)
	if spec == "" && production() {
		spec = productionDefault
	}
	if spec == "" || spec == "off" {
		log.Printf("[cams] %s: disabled", envVar)
		return
	}
	if _, err := c.AddFunc(spec, job); err != nil {
		log.Printf("[cams] invalid %s %q: %v", envVar, spec, err)
	}
}
