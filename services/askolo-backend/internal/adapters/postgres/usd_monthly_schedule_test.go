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

func TestProcessDueMonthlyUSDGrantsSeedsOnlyFutureCycles(t *testing.T) {
	ctx, pool, store := openTAR10(t)
	tar10User(t, ctx, pool, "monthly-scheduler-user")
	now := time.Now().UTC()
	createdAt := now.Add(-60 * 24 * time.Hour)
	if _, err := pool.Exec(ctx, `
		UPDATE users
		SET created_at=$2, email_verified_at=$2
		WHERE id=$1
	`, "monthly-scheduler-user", createdAt); err != nil {
		t.Fatalf("verify monthly scheduler user: %v", err)
	}

	result, err := store.ProcessDueMonthlyUSDGrants(ctx, now, 100)
	if err != nil {
		t.Fatalf("process monthly grants: %v", err)
	}
	if result.Processed != 0 {
		t.Fatalf("processed cycles = %d, want no historical cycle", result.Processed)
	}
	var scheduled time.Time
	if err := pool.QueryRow(ctx, `
		SELECT next_cycle_at
		FROM ai_credit_monthly_grant_schedules
		WHERE user_id=$1
	`, "monthly-scheduler-user").Scan(&scheduled); err != nil {
		t.Fatalf("read next monthly cycle: %v", err)
	}
	if !scheduled.After(now) {
		t.Fatalf("next monthly cycle = %s, want future of %s", scheduled, now)
	}
}
