package google

import (
	"errors"
	"net/mail"
	"strconv"
	"strings"
)

type homeEmailMessageMetadata struct {
	ThreadID     string
	InternalDate string
	From         string
	Starred      bool
	Important    bool
}

type homeEmailThreadMetadata struct {
	ID       string
	Messages []homeEmailMessageMetadata
}

type homeEmailCounts struct {
	UnreadInboxCount  int64
	ActionNeededCount int64
}

// calculateHomeEmailCounts uses only Gmail message metadata. Action-needed
// items are distinct threads whose latest message is not from the account
// owner, unioned with threads containing starred or important messages.
func calculateHomeEmailCounts(
	ownerEmail string,
	unreadInboxCount int64,
	inboxThreads []homeEmailThreadMetadata,
	flaggedMessages []homeEmailMessageMetadata,
) (homeEmailCounts, error) {
	if unreadInboxCount < 0 {
		return homeEmailCounts{}, errors.New("Gmail returned a negative unread count")
	}
	owner, err := normalizeMailbox(ownerEmail)
	if err != nil {
		return homeEmailCounts{}, errors.New("connected account email is invalid")
	}

	actionThreads := make(map[string]struct{})
	for _, thread := range inboxThreads {
		if thread.ID == "" || len(thread.Messages) == 0 {
			return homeEmailCounts{}, errors.New("Gmail returned an incomplete thread")
		}
		latest := thread.Messages[0]
		latestDate, err := strconv.ParseInt(latest.InternalDate, 10, 64)
		if err != nil || latestDate < 0 {
			return homeEmailCounts{}, errors.New("Gmail returned an invalid message date")
		}
		for _, message := range thread.Messages[1:] {
			date, err := strconv.ParseInt(message.InternalDate, 10, 64)
			if err != nil || date < 0 {
				return homeEmailCounts{}, errors.New("Gmail returned an invalid message date")
			}
			if date >= latestDate {
				latest, latestDate = message, date
			}
		}
		from, err := normalizeMailbox(latest.From)
		if err != nil {
			return homeEmailCounts{}, errors.New("Gmail returned an invalid sender")
		}
		if from != owner {
			actionThreads[thread.ID] = struct{}{}
		}
	}

	for _, message := range flaggedMessages {
		if message.ThreadID == "" {
			return homeEmailCounts{}, errors.New("Gmail returned a flagged message without a thread")
		}
		if message.Starred || message.Important {
			actionThreads[message.ThreadID] = struct{}{}
		}
	}

	return homeEmailCounts{
		UnreadInboxCount:  unreadInboxCount,
		ActionNeededCount: int64(len(actionThreads)),
	}, nil
}

func normalizeMailbox(value string) (string, error) {
	address, err := mail.ParseAddress(strings.TrimSpace(value))
	if err != nil || strings.TrimSpace(address.Address) == "" {
		return "", errors.New("invalid email address")
	}
	return strings.ToLower(strings.TrimSpace(address.Address)), nil
}
