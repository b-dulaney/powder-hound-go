package supabase

import (
	"os"
	"testing"
)

func TestRealDatabaseService(t *testing.T) {
	os.Setenv("ENV", "production")
	os.Setenv("SUPABASE_URL", "https://example.com")
	os.Setenv("SUPABASE_SERVICE_ROLE_KEY", "example_key")
	// Assuming NewSupabaseService() returns a SupabaseService instance when ENV is set to "prod"
	service := NewSupabaseService()

	_, ok := service.(*SupabaseService)
	if !ok {
		t.Errorf("Expected SupabaseService, got %T", service)
	}
}

func TestMockSupabaseService(t *testing.T) {
	os.Setenv("ENV", "development")
	// Assuming NewSupabaseService() returns a MockSupabaseService instance when ENV is set to "mock"
	service := NewSupabaseService()

	_, ok := service.(*MockSupabaseService)
	if !ok {
		t.Errorf("Expected MockSupabaseService, got %T", service)
	}
}
