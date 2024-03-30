package tasks

import (
	"powderhoundgo/internal/email"
	"testing"
)

func TestNewResortWebScrapeTask(t *testing.T) {
	task, err := NewResortWebScrapeTask("Test Mountain")
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	if task.Type() != TypeResortWebScrapingJob {
		t.Errorf("Expected task type %s, got %s", TypeResortWebScrapingJob, task.Type())
	}
}

func TestNewAlertEmailTask(t *testing.T) {
	emailData := []email.EmailData{{Location: "Test Location", Snowfall: 12}}
	task, err := NewAlertEmailTask("test@example.com", emailData, TypeForecastAlertEmail)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	if task.Type() != TypeForecastAlertEmail {
		t.Errorf("Expected task type %s, got %s", TypeForecastAlertEmail, task.Type())
	}
}

func TestNewForecastAlertEmailTask(t *testing.T) {
	emailData := []email.EmailData{{Location: "Test Location", Snowfall: 12}}
	task, err := NewForecastAlertEmailTask("test@example.com", emailData)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	if task.Type() != TypeForecastAlertEmail {
		t.Errorf("Expected task type %s, got %s", TypeForecastAlertEmail, task.Type())
	}
}

func TestNewOvernightAlertEmailTask(t *testing.T) {
	emailData := []email.EmailData{{Location: "Test Location", Snowfall: 12}}
	task, err := NewOvernightAlertEmailTask("test@example.com", emailData)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	if task.Type() != TypeOvernightEmail {
		t.Errorf("Expected task type %s, got %s", TypeOvernightEmail, task.Type())
	}
}
