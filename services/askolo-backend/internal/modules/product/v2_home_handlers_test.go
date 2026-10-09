package product

import (
	"testing"
	"time"

	"askolo/backend/internal/adapters/postgres"
)

func TestCombineHomeFreshness(t *testing.T) {
	tests := []struct {
		name   string
		states []string
		want   string
	}{
		{name: "no connected summaries", states: nil, want: "unavailable"},
		{name: "all current", states: []string{"current", "current"}, want: "current"},
		{name: "all unavailable", states: []string{"unavailable", "unavailable"}, want: "unavailable"},
		{name: "syncing takes priority over stale", states: []string{"stale", "syncing"}, want: "syncing"},
		{name: "error takes priority", states: []string{"syncing", "error"}, want: "error"},
		{name: "mixed freshness", states: []string{"current", "stale"}, want: "stale"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := combineHomeFreshness(test.states); got != test.want {
				t.Fatalf("combineHomeFreshness(%v) = %q; want %q", test.states, got, test.want)
			}
		})
	}
}

func TestHomeTaskDueTodayUsesLocalDateAndTimeZone(t *testing.T) {
	location, err := time.LoadLocation("Africa/Cairo")
	if err != nil {
		t.Fatal(err)
	}
	start := time.Date(2026, time.October, 10, 0, 0, 0, 0, location)
	end := start.AddDate(0, 0, 1)
	dueOn := time.Date(2026, time.October, 10, 0, 0, 0, 0, time.UTC)

	if !homeTaskDueToday(postgres.V2Task{DueOn: &dueOn}, start, end) {
		t.Fatal("date-only due task should match the Home calendar date")
	}
	beforeMidnightInCairo := time.Date(2026, time.October, 9, 22, 0, 0, 0, time.UTC)
	if !homeTaskDueToday(postgres.V2Task{PlannedStartAt: &beforeMidnightInCairo}, start, end) {
		t.Fatal("a planned start at the local day boundary should match")
	}
	atNextMidnight := end.UTC()
	if homeTaskDueToday(postgres.V2Task{PlannedStartAt: &atNextMidnight}, start, end) {
		t.Fatal("a planned start at the next local day boundary must not match")
	}
}

func TestHomeTaskStatusActive(t *testing.T) {
	for _, status := range []string{"done", "completed", "cancelled", "archived", "skipped"} {
		if homeTaskStatusActive(status) {
			t.Errorf("status %q should not be active", status)
		}
	}
	for _, status := range []string{"open", "in_progress"} {
		if !homeTaskStatusActive(status) {
			t.Errorf("status %q should be active", status)
		}
	}
}
