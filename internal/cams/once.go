package cams

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/hibiken/asynq"
)

// RunOnce syncs the cams and captures every one inline, without Redis (cmd/cams). only, when
// set, limits it to one resort slug; includeClosed captures closed resorts too. It returns how
// many captures failed.
func RunOnce(ctx context.Context, store Store, only string, includeClosed bool) (failed int, err error) {
	if only != "" {
		store = onlyStore{Store: store, slug: only}
	}
	if includeClosed {
		store = ignoreStatusStore{store}
	}
	run := &inlineQueue{ctx: ctx, capturer: &Capturer{Store: store, Fetch: sharedFetcher().Fetch, Now: time.Now}}
	err = QueueCaptures(store, run, time.Now())
	return run.failed, err
}

// inlineQueue runs each capture as it's queued.
type inlineQueue struct {
	ctx      context.Context
	capturer *Capturer
	failed   int
}

func (q *inlineQueue) Enqueue(t *asynq.Task, _ ...asynq.Option) (*asynq.TaskInfo, error) {
	var req CaptureRequest
	if err := json.Unmarshal(t.Payload(), &req); err != nil {
		return nil, err
	}
	stored, err := q.capturer.Capture(q.ctx, req)
	if recErr := q.capturer.Store.RecordCapture(req.CamID, err); recErr != nil {
		log.Printf("[cams] %v", recErr)
	}
	switch {
	case err != nil:
		q.failed++
		log.Printf("[cams] FAIL %s/%s: %v", req.Slug, req.Key, err)
	case stored:
		log.Printf("[cams] stored %s/%s", req.Slug, req.Key)
	default:
		log.Printf("[cams] unchanged %s/%s", req.Slug, req.Key)
	}
	// Report success to QueueCaptures either way: a failed capture is counted here, and
	// shouldn't read as a failure to queue.
	return &asynq.TaskInfo{}, nil
}

type onlyStore struct {
	Store
	slug string
}

func (s onlyStore) ListConfigSlugs() ([]string, error) {
	slugs, err := s.Store.ListConfigSlugs()
	if err != nil {
		return nil, err
	}
	for _, slug := range slugs {
		if slug == s.slug {
			return []string{slug}, nil
		}
	}
	return nil, fmt.Errorf("no scraper config named %q", s.slug)
}

// ignoreStatusStore reports no statuses, so closed resorts are captured too.
type ignoreStatusStore struct{ Store }

func (ignoreStatusStore) ResortStatuses() (map[int]string, error) { return nil, nil }
