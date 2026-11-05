package monitor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"powderhoundgo/internal/tasks"

	"github.com/hibiken/asynq"
)

// The task handlers live here, not in internal/tasks, so the email service, which imports
// tasks, doesn't compile the monitor and its dependencies (Anthropic SDK).

// DailyTask returns the handler for the daily check (tasks.TypeMonitorDaily). client queues
// the follow-up confirm-open checks.
func DailyTask(client *asynq.Client) asynq.HandlerFunc {
	return func(c context.Context, t *asynq.Task) error {
		m, err := New()
		if err != nil {
			return fmt.Errorf("monitor setup: %w: %w", err, asynq.SkipRetry)
		}
		m.ScheduleConfirmOpen = func(req ConfirmOpenRequest) error {
			return enqueueConfirmOpen(client, req)
		}
		err = m.Run(c)
		m.Store.RecordJobRun("scraper-monitor", err)
		return err
	}
}

func enqueueConfirmOpen(client *asynq.Client, req ConfirmOpenRequest) error {
	payload, err := json.Marshal(req)
	if err != nil {
		return err
	}
	// One follow-up per resort per day, even if the daily task is retried.
	id := fmt.Sprintf("confirm-open:%s:%s", req.Slug, req.ScrapedAt.Format("2006-01-02"))
	_, err = client.Enqueue(asynq.NewTask(tasks.TypeMonitorConfirmOpen, payload),
		asynq.TaskID(id), asynq.ProcessIn(confirmOpenDelay), asynq.MaxRetry(2),
		asynq.Timeout(10*time.Minute), asynq.Retention(24*time.Hour))
	if errors.Is(err, asynq.ErrTaskIDConflict) {
		return nil
	}
	return err
}

// HandleConfirmOpenTask runs one follow-up check (tasks.TypeMonitorConfirmOpen).
func HandleConfirmOpenTask(c context.Context, t *asynq.Task) error {
	var req ConfirmOpenRequest
	if err := json.Unmarshal(t.Payload(), &req); err != nil {
		return fmt.Errorf("decode payload: %w: %w", err, asynq.SkipRetry)
	}
	m, err := New()
	if err != nil {
		return fmt.Errorf("monitor setup: %w: %w", err, asynq.SkipRetry)
	}
	return m.ConfirmOpen(c, req)
}
