package product

import (
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
