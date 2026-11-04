package supabase

import (
	"testing"
)

func TestDefaultsToRealClient(t *testing.T) {
	t.Setenv("ENV", "development")
	t.Setenv("SUPABASE_MOCK", "")
	t.Setenv("SUPABASE_URL", "https://devbranchref.supabase.co")
	t.Setenv("SUPABASE_SERVICE_ROLE_KEY", "example_key")

	if _, ok := NewSupabaseService().(*SupabaseService); !ok {
		t.Error("expected the real client when SUPABASE_MOCK is unset")
	}
}

func TestMockWhenRequested(t *testing.T) {
	t.Setenv("SUPABASE_MOCK", "true")
	t.Setenv("SUPABASE_URL", "https://example.com")
	t.Setenv("SUPABASE_SERVICE_ROLE_KEY", "example_key")

	if _, ok := NewSupabaseService().(*MockSupabaseService); !ok {
		t.Error("expected the mock client when SUPABASE_MOCK=true")
	}
}

func TestCheckTarget(t *testing.T) {
	prod := "https://" + ProductionProjectRef + ".supabase.co"
	if err := CheckTarget(prod, false); err == nil {
		t.Error("a non-production run against production must be refused")
	}
	if err := CheckTarget(prod, true); err != nil {
		t.Errorf("production run against production: %v", err)
	}
	if err := CheckTarget("https://devbranchref.supabase.co", false); err != nil {
		t.Errorf("dev run against the dev branch: %v", err)
	}
	if err := CheckTarget("http://127.0.0.1:54321", false); err != nil {
		t.Errorf("dev run against the local stack: %v", err)
	}
}
