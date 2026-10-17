package postgres

import (
	"testing"
	"time"
)

func TestNextMonthlyUSDGrantCycleStaysSignupAnchored(t *testing.T) {
	anchor := time.Date(2026, 1, 3, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name string
		now  time.Time
		want time.Time
	}{
		{
			name: "normal interval",
			now:  anchor.Add(10 * 24 * time.Hour),
			want: anchor.Add(monthlyUSDGrantInterval),
		},
		{
			name: "skips missed disabled cycles",
			now:  anchor.Add(71 * 24 * time.Hour),
			want: anchor.Add(90 * 24 * time.Hour),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := nextMonthlyUSDGrantCycle(anchor, tc.now); !got.Equal(tc.want) {
				t.Fatalf("nextMonthlyUSDGrantCycle() = %s, want %s", got, tc.want)
			}
		})
	}
}
