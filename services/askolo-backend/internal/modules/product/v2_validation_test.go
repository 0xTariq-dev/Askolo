package product

import (
	"askolo/backend/internal/adapters/postgres"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"
)

func TestExpandV2RecurrenceBoundaries(t *testing.T) {
	tests := []struct {
		name     string
		rule     v2RecurrenceRule
		from     string
		to       string
		expected []string
	}{
		{
			name: "daily interval and inclusive end",
			rule: v2RecurrenceRule{
				Frequency: "daily", Interval: 2, StartsOn: "2026-10-09",
				EndsOn: stringPointer("2026-10-15"), TimeZone: "Africa/Cairo",
			},
			from: "2026-10-09", to: "2026-10-16",
			expected: []string{"2026-10-09", "2026-10-11", "2026-10-13", "2026-10-15"},
		},
		{
			name: "weekly interval uses ISO weekdays",
			rule: v2RecurrenceRule{
				Frequency: "weekly", Interval: 2, Weekdays: []int{1, 5},
				StartsOn: "2026-10-09", TimeZone: "Africa/Cairo",
			},
			from: "2026-10-09", to: "2026-10-26",
			expected: []string{"2026-10-09", "2026-10-19", "2026-10-23"},
		},
		{
			name: "monthly skips missing month days",
			rule: v2RecurrenceRule{
				Frequency: "monthly", Interval: 1, StartsOn: "2026-01-31",
				TimeZone: "UTC",
			},
			from: "2026-01-31", to: "2026-04-30",
			expected: []string{"2026-01-31", "2026-03-31"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			from, _ := parseV2Date(test.from)
			to, _ := parseV2Date(test.to)
			gotDates, err := expandV2Recurrence(test.rule, from, to)
			if err != nil {
				t.Fatalf("expand recurrence: %v", err)
			}
			got := make([]string, len(gotDates))
			for i, date := range gotDates {
				got[i] = date.Format("2006-01-02")
			}
			if !reflect.DeepEqual(got, test.expected) {
				t.Fatalf("expanded dates = %v, want %v", got, test.expected)
			}
		})
	}
}

func TestV2QueryValidationRejectsUnknownRepeatedAndEmptyValues(t *testing.T) {
	for _, test := range []struct {
		name string
		url  string
		want bool
	}{
		{name: "supported", url: "/tasks?limit=10&status=open", want: true},
		{name: "unknown", url: "/tasks?limit=10&sort=title"},
		{name: "repeated", url: "/tasks?limit=10&limit=20"},
		{name: "empty", url: "/tasks?limit="},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest("GET", test.url, nil)
			if got := v2QueryValid(request, "limit", "status"); got != test.want {
				t.Fatalf("v2QueryValid() = %t, want %t", got, test.want)
			}
		})
	}
}

func TestV2WorkspaceIdentifierAllowsPersonalWorkspaceIDsOnly(t *testing.T) {
	for _, test := range []struct {
		id   string
		want bool
	}{
		{id: "space_123", want: true},
		{id: "personal:user_123", want: true},
		{id: "personal:36722678-e65c-4295-8e4b-b5f", want: true},
		{id: "personal:", want: false},
		{id: "other:user_123", want: false},
		{id: "personal:user/123", want: false},
	} {
		if got := v2WorkspaceIdentifierPattern.MatchString(test.id); got != test.want {
			t.Errorf("workspace identifier %q valid = %t, want %t", test.id, got, test.want)
		}
	}
}

func TestDecodeStrictRawJSONRejectsUnknownAndTrailingValues(t *testing.T) {
	for _, input := range []string{
		`{"targetId":"task_1","minutesBefore":15,"unexpected":true}`,
		`{"targetId":"task_1","minutesBefore":15} {}`,
	} {
		var target struct {
			TargetID      string `json:"targetId"`
			MinutesBefore int    `json:"minutesBefore"`
		}
		if err := decodeStrictRawJSON([]byte(input), &target); err == nil {
			t.Fatalf("decodeStrictRawJSON accepted %s", input)
		}
	}
}

func stringPointer(value string) *string {
	return &value
}

func TestV2DateParsingUsesCalendarDates(t *testing.T) {
	got, ok := parseV2Date("2026-10-09")
	if !ok || got.Location() != time.UTC || got.Format("2006-01-02") != "2026-10-09" {
		t.Fatalf("parseV2Date = %v, %t; want UTC calendar date 2026-10-09", got, ok)
	}
}

func TestV2TaskHabitContractValidation(t *testing.T) {
	plannedOn := "2026-10-12"
	start := "23:30"
	duration := 60
	timeZone := "Africa/Cairo"
	tests := []struct {
		name    string
		request v2TaskRequest
		wantErr bool
	}{
		{
			name: "task may be all day with no priority",
			request: v2TaskRequest{
				Kind: "task", Title: "Pay rent", PlannedOn: &plannedOn,
				TimeZone: timeZone,
			},
		},
		{
			name: "time block keeps duration with local start",
			request: v2TaskRequest{
				Kind: "task", Title: "Work block", PlannedOn: &plannedOn,
				PlannedStartTime: &start, DurationMinutes: &duration, TimeZone: timeZone,
			},
		},
		{
			name: "habit cannot have a priority",
			request: v2TaskRequest{
				Kind: "habit", Title: "Walk", Priority: "high", TimeZone: timeZone,
			},
			wantErr: true,
		},
		{
			name: "time block requires a duration",
			request: v2TaskRequest{
				Kind: "task", Title: "Work block", PlannedOn: &plannedOn,
				PlannedStartTime: &start, TimeZone: timeZone,
			},
			wantErr: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := buildV2TaskWrite(test.request)
			if (err != nil) != test.wantErr {
				t.Fatalf("buildV2TaskWrite() error = %v, wantErr %t", err, test.wantErr)
			}
		})
	}
}

func TestV2TaskTimeBlockAppearsOnEveryOverlappingAgendaDate(t *testing.T) {
	plannedOn, _ := parseV2Date("2026-10-12")
	start, duration := "23:30:00", 60
	task := postgres.V2Task{
		PlannedOn:        &plannedOn,
		PlannedStartTime: &start,
		DurationMinutes:  &duration,
		TimeZone:         "Africa/Cairo",
	}
	nextDay, _ := parseV2Date("2026-10-13")
	if !v2TaskTimeBlockOverlaps(task, nextDay, nextDay, "Africa/Cairo") {
		t.Fatal("cross-midnight time block was omitted from the following Cairo agenda day")
	}
	if v2TaskTimeBlockOverlaps(task, nextDay, nextDay, "UTC") {
		t.Fatal("time block was incorrectly included in the following UTC agenda day")
	}
}
