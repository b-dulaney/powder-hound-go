package queue

import (
	"testing"

	"powderhoundgo/internal/supabase"
)

func TestIsResortClosed(t *testing.T) {
	statuses := map[int]string{1: "closed", 2: "open", 3: "unknown"}
	cases := []struct {
		id       int
		statuses map[int]string
		want     bool
	}{
		{1, statuses, true},
		{2, statuses, false},
		{3, statuses, false},
		{4, statuses, false}, // no resort_conditions row yet
		{1, nil, false},      // statuses couldn't be read: scrape everything
	}
	for _, tc := range cases {
		if got := isResortClosed(supabase.ScrapingConfig{ID: tc.id}, tc.statuses); got != tc.want {
			t.Errorf("isResortClosed(id=%d) = %v, want %v", tc.id, got, tc.want)
		}
	}
}
