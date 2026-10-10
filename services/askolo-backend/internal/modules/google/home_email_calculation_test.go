package google

import "testing"

func TestCalculateHomeEmailCountsUsesLatestMessageAndUnionsFlaggedThreads(t *testing.T) {
	counts, err := calculateHomeEmailCounts(
		"owner@example.test",
		4,
		[]homeEmailThreadMetadata{
			{
				ID: "thread-inbound-latest",
				Messages: []homeEmailMessageMetadata{
					{InternalDate: "200", From: "Owner <OWNER@example.test>"},
					{InternalDate: "300", From: "Teammate <team@example.test>"},
				},
			},
			{
				ID: "thread-outbound-latest",
				Messages: []homeEmailMessageMetadata{
					{InternalDate: "500", From: "Other <other@example.test>"},
					{InternalDate: "600", From: "owner@example.test"},
				},
			},
		},
		[]homeEmailMessageMetadata{
			{ThreadID: "thread-inbound-latest", Starred: true},
			{ThreadID: "thread-starred-only", Important: true},
			{ThreadID: "thread-starred-only", Starred: true},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if counts.UnreadInboxCount != 4 {
		t.Fatalf("unread inbox count = %d, want 4", counts.UnreadInboxCount)
	}
	if counts.ActionNeededCount != 2 {
		t.Fatalf("action-needed count = %d, want two distinct threads", counts.ActionNeededCount)
	}
}

func TestCalculateHomeEmailCountsRejectsUncertainMetadata(t *testing.T) {
	tests := []struct {
		name    string
		owner   string
		unread  int64
		threads []homeEmailThreadMetadata
		flagged []homeEmailMessageMetadata
	}{
		{
			name:    "missing latest sender",
			owner:   "owner@example.test",
			threads: []homeEmailThreadMetadata{{ID: "thread", Messages: []homeEmailMessageMetadata{{InternalDate: "1"}}}},
		},
		{
			name:    "missing flagged thread",
			owner:   "owner@example.test",
			flagged: []homeEmailMessageMetadata{{Starred: true}},
		},
		{
			name:   "negative unread count",
			owner:  "owner@example.test",
			unread: -1,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := calculateHomeEmailCounts(test.owner, test.unread, test.threads, test.flagged); err == nil {
				t.Fatal("expected incomplete metadata to fail")
			}
		})
	}
}
