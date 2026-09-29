package product

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"askolo/backend/internal/adapters/postgres"
	"askolo/backend/internal/config"
	"askolo/backend/internal/migrations"
	"github.com/jackc/pgx/v5/pgxpool"
)

type creditPolicyIntegrationFixture struct {
	ctx   context.Context
	pool  *pgxpool.Pool
	store *postgres.Store
}

// This test uses migrations against a throw-away schema and must never use an application database.
func openCreditPolicyIntegrationFixture(t *testing.T) *creditPolicyIntegrationFixture {
	t.Helper()

	baseURL := strings.TrimSpace(os.Getenv("ASKOLO_TEST_DATABASE_URL"))
	if baseURL == "" {
		t.Skip("set ASKOLO_TEST_DATABASE_URL to run credit policy handler integration tests")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)

	adminPool, err := pgxpool.New(ctx, baseURL)
	if err != nil {
		t.Fatalf("open disposable database: %v", err)
	}
	if err := adminPool.Ping(ctx); err != nil {
		adminPool.Close()
		t.Fatalf("ping disposable database: %v", err)
	}

	schema := fmt.Sprintf("askolo_credit_policy_%d", time.Now().UnixNano())
	if _, err := adminPool.Exec(ctx, `CREATE SCHEMA "`+schema+`"`); err != nil {
		adminPool.Close()
		t.Fatalf("create disposable schema: %v", err)
	}
	t.Cleanup(func() {
		_, _ = adminPool.Exec(context.Background(), `DROP SCHEMA "`+schema+`" CASCADE`)
		adminPool.Close()
	})

	schemaURL := creditPolicySchemaURL(t, baseURL, schema)
	pool, err := pgxpool.New(ctx, schemaURL)
	if err != nil {
		t.Fatalf("open disposable schema: %v", err)
	}
	t.Cleanup(pool.Close)

	if err := migrations.Run(ctx, pool); err != nil {
		t.Fatalf("apply migrations to disposable schema: %v", err)
	}
	store, err := postgres.New(ctx, schemaURL)
	if err != nil {
		t.Fatalf("open store on disposable schema: %v", err)
	}
	t.Cleanup(store.Close)

	return &creditPolicyIntegrationFixture{ctx: ctx, pool: pool, store: store}
}

func TestTranscriptionPreferenceRoutesRejectAnonymousRequests(t *testing.T) {
	fixture := openCreditPolicyIntegrationFixture(t)
	handler := NewHandler(config.Config{}, fixture.store, slog.Default(), "askolo_session").Routes()

	for _, test := range []struct {
		name   string
		method string
		body   string
	}{
		{name: "read", method: http.MethodGet},
		{name: "update", method: http.MethodPatch, body: `{"consent":true}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(
				test.method,
				"/api/ai/transcription-preferences",
				strings.NewReader(test.body),
			)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)

			if response.Code != http.StatusUnauthorized {
				t.Fatalf("anonymous %s status = %d, want %d: %s",
					test.name, response.Code, http.StatusUnauthorized, response.Body.String())
			}
			if !strings.Contains(response.Body.String(), `"UNAUTHORIZED"`) {
				t.Fatalf("anonymous %s response lacks UNAUTHORIZED code: %s",
					test.name, response.Body.String())
			}
		})
	}
}

func creditPolicySchemaURL(t *testing.T, databaseURL, schema string) string {
	t.Helper()
	parsed, err := url.Parse(databaseURL)
	if err != nil {
		t.Fatalf("parse disposable database URL: %v", err)
	}
	query := parsed.Query()
	query.Set("options", "-c search_path="+schema)
	parsed.RawQuery = strings.ReplaceAll(query.Encode(), "+", "%20")
	return parsed.String()
}

func (f *creditPolicyIntegrationFixture) createUserAndSession(
	t *testing.T,
	userID, email string,
	verified bool,
) string {
	t.Helper()
	var verifiedAt any
	if verified {
		verifiedAt = time.Now().UTC()
	}
	if _, err := f.pool.Exec(
		f.ctx,
		`INSERT INTO users (id, email, status, email_verified_at)
		 VALUES ($1, $2, 'active', $3)`,
		userID, email, verifiedAt,
	); err != nil {
		t.Fatalf("insert test user: %v", err)
	}
	sessionID, err := f.store.CreateSession(f.ctx, userID, "integration-test", time.Hour)
	if err != nil {
		t.Fatalf("create test session: %v", err)
	}
	return sessionID
}

func (f *creditPolicyIntegrationFixture) addCreditAccount(t *testing.T, userID string, credits int) {
	t.Helper()
	if _, err := f.pool.Exec(
		f.ctx,
		`INSERT INTO ai_credit_accounts (user_id, granted_credits) VALUES ($1, $2)`,
		userID, credits,
	); err != nil {
		t.Fatalf("insert test credit account: %v", err)
	}
}

func TestCreditAdminRoutesRejectNonAdminsWithoutMutations(t *testing.T) {
	fixture := openCreditPolicyIntegrationFixture(t)
	nonAdminSession := fixture.createUserAndSession(t, "credit-non-admin", "member@example.test", true)
	unverifiedAdminSession := fixture.createUserAndSession(t, "credit-unverified-admin", "operator@example.test", false)
	fixture.createUserAndSession(t, "credit-target", "target@example.test", true)
	fixture.addCreditAccount(t, "credit-target", 40)

	handler := NewHandler(config.Config{
		AdminEmails: map[string]struct{}{"operator@example.test": {}},
	}, fixture.store, slog.Default(), "askolo_session").Routes()

	policyBefore, err := fixture.store.AICreditPolicy(fixture.ctx)
	if err != nil {
		t.Fatalf("read initial policy: %v", err)
	}
	balanceBefore, err := fixture.store.AICreditBalance(fixture.ctx, "credit-target")
	if err != nil {
		t.Fatalf("read initial target balance: %v", err)
	}

	policyBody, err := json.Marshal(map[string]any{
		"expectedVersion":      policyBefore.Version,
		"changeReason":         "unauthorized integration test",
		"operationWeights":     policyBefore.OperationWeights,
		"monthlyGrantCredits":  policyBefore.MonthlyGrantCredits,
		"rolloverCapCredits":   policyBefore.RolloverCapCredits,
		"rolloverExpiryDays":   policyBefore.RolloverExpiryDays,
		"overrunMarginPercent": policyBefore.OverrunMarginPercent,
	})
	if err != nil {
		t.Fatalf("encode policy update: %v", err)
	}

	routes := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{name: "read policy", method: http.MethodGet, path: "/api/admin/ai-credit-policy"},
		{name: "update policy", method: http.MethodPatch, path: "/api/admin/ai-credit-policy", body: string(policyBody)},
		{
			name: "adjust balance", method: http.MethodPost, path: "/api/admin/ai-credit-adjustments",
			body: `{"userId":"credit-target","amountCredits":5,"reason":"grant","idempotencyKey":"unauthorized-adjustment"}`,
		},
		{
			name: "reverse adjustment", method: http.MethodPost, path: "/api/admin/ai-credit-adjustments/1/reverse",
			body: `{"userId":"credit-target","reason":"reverse","idempotencyKey":"unauthorized-reversal"}`,
		},
		{
			name: "reverse grant", method: http.MethodPost, path: "/api/admin/ai-credit-grants/1/reverse",
			body: `{"userId":"credit-target","reason":"reverse","idempotencyKey":"unauthorized-grant-reversal"}`,
		},
		{
			name: "refund reservation", method: http.MethodPost, path: "/api/admin/ai-credit-reservations/voice-test/refund",
			body: `{"userId":"credit-target","amountCredits":1,"reason":"refund","idempotencyKey":"unauthorized-refund"}`,
		},
		{name: "inspect another user's usage", method: http.MethodGet, path: "/api/admin/ai-credit-usage/credit-target"},
	}

	sessions := []struct {
		name      string
		sessionID string
	}{
		{name: "verified non-admin", sessionID: nonAdminSession},
		{name: "unverified allowlisted user", sessionID: unverifiedAdminSession},
	}
	for _, session := range sessions {
		t.Run(session.name, func(t *testing.T) {
			for _, route := range routes {
				t.Run(route.name, func(t *testing.T) {
					request := httptest.NewRequest(route.method, route.path, strings.NewReader(route.body))
					request.Header.Set("Authorization", "Bearer "+session.sessionID)
					response := httptest.NewRecorder()
					handler.ServeHTTP(response, request)

					if response.Code != http.StatusForbidden {
						t.Fatalf("status = %d, want %d; body=%q", response.Code, http.StatusForbidden, response.Body.String())
					}
					if !strings.Contains(response.Body.String(), `"code":"FORBIDDEN"`) {
						t.Fatalf("response does not identify authorization denial: %q", response.Body.String())
					}
				})
			}
		})
	}

	policyAfter, err := fixture.store.AICreditPolicy(fixture.ctx)
	if err != nil {
		t.Fatalf("read policy after denied requests: %v", err)
	}
	if policyAfter.Version != policyBefore.Version {
		t.Fatalf("policy version after denied requests = %d, want unchanged version %d", policyAfter.Version, policyBefore.Version)
	}
	balanceAfter, err := fixture.store.AICreditBalance(fixture.ctx, "credit-target")
	if err != nil {
		t.Fatalf("read target balance after denied requests: %v", err)
	}
	if balanceAfter != balanceBefore {
		t.Fatalf("target balance after denied requests = %d, want unchanged balance %d", balanceAfter, balanceBefore)
	}
	for table, query := range map[string]string{
		"adjustments":  `SELECT count(*) FROM ai_credit_adjustments WHERE user_id='credit-target'`,
		"grants":       `SELECT count(*) FROM ai_credit_grants WHERE user_id='credit-target'`,
		"reservations": `SELECT count(*) FROM ai_credit_reservations WHERE user_id='credit-target'`,
	} {
		var count int
		if err := fixture.pool.QueryRow(fixture.ctx, query).Scan(&count); err != nil {
			t.Fatalf("count target %s: %v", table, err)
		}
		if count != 0 {
			t.Fatalf("denied requests created %d target %s, want none", count, table)
		}
	}
}

func TestCreditAdminAllowlistedVerifiedUserCanUpdatePolicy(t *testing.T) {
	fixture := openCreditPolicyIntegrationFixture(t)
	adminSession := fixture.createUserAndSession(t, "credit-admin", "operator@example.test", true)
	handler := NewHandler(config.Config{
		AdminEmails: map[string]struct{}{"operator@example.test": {}},
	}, fixture.store, slog.Default(), "askolo_session").Routes()

	current, err := fixture.store.USDPolicy(fixture.ctx)
	if err != nil {
		t.Fatalf("read initial policy: %v", err)
	}
	// The integration fixture may predate the USD tariff columns, or contain
	// legacy/non-USD cards. Exercise the USD endpoint with an explicit,
	// independently valid AssemblyAI tariff snapshot.
	current.RateCards = map[string]postgres.USDRateCard{
		"assemblyai:recorded:universal-3-5-pro": {Provider: "assemblyai", Mode: "recorded", Model: "universal-3-5-pro", Meter: "hour", UsdMicrosPerHour: 210000},
		"assemblyai:realtime:universal-3-5-pro": {Provider: "assemblyai", Mode: "realtime", Model: "universal-3-5-pro", Meter: "hour", UsdMicrosPerHour: 450000},
	}
	body, err := json.Marshal(map[string]any{
		"expectedVersion":       current.Version,
		"changeReason":          "authorized integration test",
		"rateCards":             current.RateCards,
		"monthlyGrantUsdMicros": int64(0),
		"rolloverCapUsdMicros":  int64(0),
		"rolloverExpiryDays":    30,
		"overrunMarginPercent":  10,
	})
	if err != nil {
		t.Fatalf("encode policy update: %v", err)
	}

	request := httptest.NewRequest(http.MethodPatch, "/api/admin/ai-credit-policy", strings.NewReader(string(body)))
	request.Header.Set("Authorization", "Bearer "+adminSession)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%q", response.Code, http.StatusOK, response.Body.String())
	}
	updated, err := fixture.store.USDPolicy(fixture.ctx)
	if err != nil {
		t.Fatalf("read updated policy: %v", err)
	}
	if updated.Version != current.Version+1 {
		t.Fatalf("updated policy version = %d, want %d", updated.Version, current.Version+1)
	}
}

func TestVoiceCreditReservationRejectsStalePolicyBeforeReserving(t *testing.T) {
	fixture := openCreditPolicyIntegrationFixture(t)
	userID := "stale-policy-user"
	fixture.createUserAndSession(t, userID, "stale@example.test", true)
	fixture.addCreditAccount(t, userID, 20)

	current, err := fixture.store.AICreditPolicy(fixture.ctx)
	if err != nil {
		t.Fatalf("read initial policy: %v", err)
	}
	updated := current
	updated.Version = current.Version + 1
	updated.ChangeReason = "stale estimate integration test"
	if _, err := fixture.store.UpdateAICreditPolicy(fixture.ctx, current.Version, updated, userID); err != nil {
		t.Fatalf("advance policy version: %v", err)
	}

	handler := NewHandler(config.Config{}, fixture.store, slog.Default(), "askolo_session")
	request := httptest.NewRequest(http.MethodPost, "/api/ai/realtime", nil)
	request.Header.Set("Idempotency-Key", "stale-policy-request")
	request.Header.Set("X-AI-Credit-Policy-Version", fmt.Sprint(current.Version))
	response := httptest.NewRecorder()

	reservation, ok := handler.reserveVoiceProviderCredit(response, request, userID, "realtime")
	if ok {
		t.Fatal("stale policy version was accepted")
	}
	if reservation.ID != "" {
		t.Fatalf("stale request returned reservation %q", reservation.ID)
	}
	if response.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d; body=%q", response.Code, http.StatusConflict, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"code":"AI_POLICY_CHANGED"`) {
		t.Fatalf("response does not identify stale policy: %q", response.Body.String())
	}

	usage, err := fixture.store.AICreditUsage(fixture.ctx, userID)
	if err != nil {
		t.Fatalf("read usage after stale request: %v", err)
	}
	if usage.Balance != 20 || usage.Reserved != 0 {
		t.Fatalf("usage after stale request = %#v, want balance 20 and no reservation", usage)
	}
	var reservationCount int
	if err := fixture.pool.QueryRow(
		fixture.ctx,
		`SELECT count(*) FROM ai_credit_reservations WHERE user_id=$1`,
		userID,
	).Scan(&reservationCount); err != nil {
		t.Fatalf("count stale request reservations: %v", err)
	}
	if reservationCount != 0 {
		t.Fatalf("stale request created %d reservations, want none", reservationCount)
	}
}
