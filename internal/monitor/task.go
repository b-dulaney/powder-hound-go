package monitor

import (
	"context"
	"fmt"

	"github.com/hibiken/asynq"
)

// HandleTask runs the daily check as an asynq task (tasks.TypeMonitorDaily). It lives here,
// not in internal/tasks, so the email service, which imports tasks, doesn't compile the
// monitor and its dependencies (Anthropic SDK).
func HandleTask(c context.Context, t *asynq.Task) error {
	m, err := New()
	if err != nil {
		return fmt.Errorf("monitor setup: %w: %w", err, asynq.SkipRetry)
	}
	err = m.Run(c)
	m.Store.RecordJobRun("scraper-monitor", err)
	return err
}
