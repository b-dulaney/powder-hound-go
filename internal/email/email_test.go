package email

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewResendService(t *testing.T) {
	t.Run("returns a ResendService in production", func(t *testing.T) {
		os.Setenv("ENV", "production")
		service := NewResendService()

		_, ok := service.(*ResendService)
		assert.True(t, ok, "Expected service to be of type *ResendService")
	})

	t.Run("returns a MockEmailService in development", func(t *testing.T) {
		os.Setenv("ENV", "development")
		service := NewResendService()

		_, ok := service.(*MockEmailService)
		assert.True(t, ok, "Expected service to be of type *MockEmailService")
	})
}

func TestSendEmail(t *testing.T) {
	t.Run("logs an email with the MockEmailService", func(t *testing.T) {
		os.Setenv("ENV", "development")
		service := NewResendService()

		err := service.SendEmail("Test Subject", "Test Body", "test@example.com")
		assert.Nil(t, err, "Expected SendEmail to return no error")
	})
}
