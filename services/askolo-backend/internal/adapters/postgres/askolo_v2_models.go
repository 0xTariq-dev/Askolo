package postgres

import (
	"encoding/json"
	"time"
)

// These are persistence models for the isolated askolo_v2 target schema.
// They are not API DTOs and are intentionally not wired into existing stores.

type V2Workspace struct {
	ID            string
	Name          string
	Status        string
	IsPersonal    bool
	TimeZone      string
	DefaultLocale string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type V2WorkspaceMembership struct {
	ID             string
	WorkspaceID    string
	UserID         string
	Role           string
	Status         string
	InvitedByUser  *string
	JoinedAt       *time.Time
	RevokedAt      *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type V2WorkspaceMemberPermission struct {
	WorkspaceID   string
	UserID        string
	PermissionKey string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type V2MemberPreferences struct {
	UserID          string
	PreferredLocale *string
	TimeZone        *string
	UpdatedAt       time.Time
}

type V2MemberAIPreferences struct {
	UserID                          string
	Retention                       string
	RecordedVoiceInputConsentAt     *time.Time
	RecordedVoiceInputConsentVersion *string
	AssistantProcessingConsentAt    *time.Time
	AssistantProcessingConsentVersion *string
	LiveVoiceConsentAt              *time.Time
	LiveVoiceConsentVersion         *string
	LiveVoiceName                   string
	RedactionLocation               *string
	SpokenReplyConsentAt            *time.Time
	SpokenReplyConsentVersion       *string
	AutoSpeakEnabled                bool
	UpdatedAt                       time.Time
}

type V2Habit struct {
	ID              string
	WorkspaceID     string
	CreatedByUserID string
	Name            string
	Description     *string
	Frequency       string
	DaysOfWeek      []int16
	Color           *string
	Icon            *string
	Status          string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type V2HabitCompletion struct {
	ID                string
	WorkspaceID       string
	HabitID           string
	CompletedByUserID string
	CompletedOn       time.Time
	CreatedAt         time.Time
}

type V2Goal struct {
	ID              string
	WorkspaceID     string
	CreatedByUserID string
	Title           string
	Description     *string
	TargetOn        *time.Time
	Status          string
	ProgressPercent int16
	Category        *string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type V2DailyPlan struct {
	ID              string
	WorkspaceID     string
	CreatedByUserID string
	PlanOn          time.Time
	Title           string
	Status          string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type V2DailyPlanItem struct {
	ID               string
	WorkspaceID      string
	PlanID           string
	CreatedByUserID  string
	AssignedToUserID *string
	Title            string
	Details          *string
	StartAt          *time.Time
	EndAt            *time.Time
	Priority         string
	Status           string
	Position         int
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type V2CalendarEvent struct {
	ID              string
	WorkspaceID     string
	CreatedByUserID string
	UpdatedByUserID string
	Title           string
	Description     *string
	StartsAt        time.Time
	EndsAt          *time.Time
	AllDay          bool
	TimeZone        string
	Location        *string
	Color           *string
	SourceProvider  *string
	SourceEventID   *string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type V2Chore struct {
	ID                string
	WorkspaceID       string
	CreatedByUserID   string
	AssignedToUserID  *string
	CompletedByUserID *string
	Title             string
	Description       *string
	Recurrence        string
	RecurrenceRule    json.RawMessage
	DueOn             *time.Time
	Status            string
	CompletedAt       *time.Time
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type V2Note struct {
	ID              string
	WorkspaceID     string
	CreatedByUserID string
	UpdatedByUserID string
	Title           string
	Content         string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type V2ActionItem struct {
	ID               string
	WorkspaceID      string
	CreatedByUserID  string
	AssignedToUserID *string
	Title            string
	SourceType       *string
	SourceReference  *string
	DueOn            *time.Time
	Priority         string
	Status           string
	CompletedAt      *time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type V2AssistantConversation struct {
	ID              string
	WorkspaceID     string
	CreatedByUserID string
	Title           string
	Status          string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type V2AssistantMessage struct {
	ID           string
	WorkspaceID  string
	ConversationID string
	SenderUserID *string
	Role         string
	Content      string
	CreatedAt    time.Time
}

type V2AssistantRun struct {
	ID                    string
	WorkspaceID           string
	ConversationID        string
	MemberUserID          string
	ReservationID         *string
	IdempotencyKeyHash    string
	TranscriptSHA256      string
	IntentSHA256          *string
	State                 string
	Currency              string
	BaseCredits           int64
	ReservedCredits       int64
	SettledCredits        int64
	BaseUSDMicros         *int64
	ReservedUSDMicros     *int64
	SettledUSDMicros      int64
	Intent                json.RawMessage
	Result                json.RawMessage
	RiskLevel             *string
	RequiresConfirmation  bool
	ConfirmationExpiresAt *time.Time
	ProviderStarted       bool
	ProviderModel         *string
	ProviderRequestID     *string
	InputTokens           *int64
	OutputTokens          *int64
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

type V2AssistantAuditEvent struct {
	ID                string
	WorkspaceID       string
	RunID             string
	MemberUserID      string
	EventType         string
	TranscriptSHA256  *string
	IntentSHA256      *string
	ToolName          *string
	ToolArgsSHA256    *string
	ResultResourceID  *string
	CreatedAt         time.Time
}

type V2AICreditPolicyVersion struct {
	Version                int
	OperationWeights       json.RawMessage
	RateCards              json.RawMessage
	MonthlyGrantCredits    int64
	RolloverCapCredits     int64
	RolloverExpiryDays     int
	OverrunMarginPercent   int
	MonthlyGrantUSDMicros  int64
	RolloverCapUSDMicros   int64
	CreatedByUserID        *string
	ChangeReason           string
	CreatedAt              time.Time
}

type V2AICreditAccount struct {
	WorkspaceID           string
	MemberUserID          string
	GrantedCredits        int64
	AdjustmentCredits     int64
	ReservedCredits       int64
	SpentCredits          int64
	RefundedCredits       int64
	GrantedUSDMicros      int64
	AdjustmentUSDMicros   int64
	ReservedUSDMicros     int64
	SpentUSDMicros        int64
	RefundedUSDMicros     int64
	UpdatedAt             time.Time
}

type V2AICreditGrant struct {
	ID                  string
	WorkspaceID         string
	MemberUserID        string
	ActorUserID         *string
	SourceType          string
	AmountCredits       int64
	AmountUSDMicros     int64
	Currency            string
	EntitlementKey      *string
	IdempotencyKeyHash  string
	Metadata            json.RawMessage
	CreatedAt           time.Time
}

type V2AICreditAdjustment struct {
	ID                    string
	WorkspaceID           string
	MemberUserID          string
	ActorUserID           string
	AmountCredits         int64
	AmountUSDMicros       int64
	Currency              string
	Reason                string
	IdempotencyKeyHash    string
	ReversalOfID          *string
	ReversalOfGrantID     *string
	Metadata              json.RawMessage
	CreatedAt             time.Time
}

type V2AICreditReservation struct {
	ID                    string
	WorkspaceID           string
	MemberUserID          string
	OperationType         string
	Provider              string
	Mode                  string
	Model                 *string
	Status                string
	IdempotencyKeyHash    string
	RequestFingerprint    *string
	Currency              string
	ReservedCredits       int64
	SettledCredits        int64
	RefundedCredits       int64
	ReservedUSDMicros     int64
	SettledUSDMicros      int64
	RefundedUSDMicros     int64
	MaxCredits            *int64
	MaxUSDMicros          *int64
	UnitRate              int64
	DurationSeconds       *int64
	ConnectedDurationMS   *int64
	PolicyVersion         int
	RateSnapshot          json.RawMessage
	ExpiresAt             *time.Time
	ProviderTokenHash     *string
	ProviderTokenUsedAt   *time.Time
	Metadata              json.RawMessage
	StartedAt             time.Time
	ClosedAt              *time.Time
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

type V2AICreditReservationEvent struct {
	ID                 string
	WorkspaceID        string
	MemberUserID       string
	ReservationID      string
	EventType          string
	Currency           string
	Credits            int64
	AmountUSDMicros    int64
	DurationMS         *int64
	IdempotencyKeyHash string
	Details            json.RawMessage
	CreatedAt          time.Time
}

type V2AIProviderUsageEvidence struct {
	ID                 string
	WorkspaceID        string
	MemberUserID       string
	ReservationID      string
	Provider           string
	Mode               string
	Region             *string
	Model              *string
	DurationMS         *int64
	UsageUnit          *string
	UsageUnits         *int64
	InputTokens        *int64
	OutputTokens       *int64
	ProviderRequestID  *string
	IdempotencyKeyHash string
	EvidenceSHA256     *string
	RecordedAt         time.Time
}

type V2IntegrationConnection struct {
	ID                 string
	WorkspaceID        string
	ConnectedByUserID  string
	Provider           string
	AccountLabel       *string
	Status             string
	GrantedScopes      []string
	Capabilities       []string
	CredentialRefID    *string
	RevokedAt          *time.Time
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

type V2IntegrationDefault struct {
	WorkspaceID  string
	Service      string
	ConnectionID string
	UpdatedAt    time.Time
}

type V2IntegrationSyncState struct {
	WorkspaceID  string
	ConnectionID string
	Service      string
	SyncCursor   *string
	LastSyncAt   *time.Time
	Status       string
	LastErrorCode *string
	UpdatedAt    time.Time
}

type V2AgendaItem struct {
	WorkspaceID      string
	ItemType         string
	ItemID           string
	Title            string
	StartsAt         *time.Time
	DueOn            *time.Time
	Status           string
	Priority         string
	AssignedToUserID *string
	UpdatedAt        time.Time
}

type V2WorkspaceOverview struct {
	WorkspaceID      string
	ActiveHabits     int64
	ActiveGoals      int64
	UpcomingEvents   int64
	OpenChores       int64
	OpenActionItems  int64
}
