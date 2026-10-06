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
	policy "askolo/backend/internal/platform/authorization"
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

func TestTranscriptionPreferencesAuthenticatedConsentPersists(t *testing.T) {
	fixture := openCreditPolicyIntegrationFixture(t)
	sessionID := fixture.createUserAndSession(
		t,
		"transcription-consent-user",
		"transcription-consent@example.test",
		true,
	)
	// Establish the same active personal-workspace capability set used by the
	// handler's lazy provisioning path, so this test isolates consent
	// persistence from authorization-fixture state.
	userID := "transcription-consent-user"
	if err := fixture.store.EnsurePersonalWorkspace(fixture.ctx, userID); err != nil {
		t.Fatalf("ensure test personal workspace: %v", err)
	}
	if err := fixture.store.SetWorkspaceMembership(
		fixture.ctx,
		postgres.DefaultWorkspaceID(userID),
		userID,
		"active",
		"owner",
		policy.PersonalWorkspaceCapabilities,
	); err != nil {
		t.Fatalf("set test workspace membership: %v", err)
	}
	decision, err := fixture.store.Authorize(fixture.ctx, policy.Input{
		ActorUserID: userID, WorkspaceID: postgres.DefaultWorkspaceID(userID), Action: policy.ActionAIExecute,
	})
	if err != nil {
		t.Fatalf("authorize test user: %v", err)
	}
	if !decision.Allowed {
		t.Fatalf("test user authorization denied: %s", decision.Reason)
	}
	sessionUserID, sessionStatus := fixture.store.SessionUserID(fixture.ctx, sessionID)
	if sessionStatus != nil || sessionUserID != userID {
		t.Fatalf("test session user lookup did not resolve active fixture user")
	}
	handler := NewHandler(config.Config{}, fixture.store, slog.Default(), "askolo_session").Routes()

	patch := httptest.NewRequest(
		http.MethodPatch,
		"/api/ai/transcription-preferences",
		strings.NewReader(`{"consent":true}`),
	)
	patch.Header.Set("Authorization", "Bearer "+sessionID)
	patch.Header.Set("Content-Type", "application/json")
	patchResponse := httptest.NewRecorder()
	handler.ServeHTTP(patchResponse, patch)

	if patchResponse.Code != http.StatusOK {
		var failure struct {
			Code string `json:"code"`
		}
		_ = json.NewDecoder(patchResponse.Body).Decode(&failure)
		t.Fatalf("consent update status = %d, want %d (code %q)", patchResponse.Code, http.StatusOK, failure.Code)
	}
	var updated struct {
		ConsentGiven   bool   `json:"consentGiven"`
		ConsentVersion string `json:"consentVersion"`
	}
	if err := json.NewDecoder(patchResponse.Body).Decode(&updated); err != nil {
		t.Fatalf("decode consent update response: %v", err)
	}
	if !updated.ConsentGiven || updated.ConsentVersion != postgres.VoiceConsentVersion {
		t.Fatalf(
			"consent update response = consentGiven:%t consentVersion:%q, want true and current version",
			updated.ConsentGiven,
			updated.ConsentVersion,
		)
	}

	get := httptest.NewRequest(http.MethodGet, "/api/ai/transcription-preferences", nil)
	get.Header.Set("Authorization", "Bearer "+sessionID)
	getResponse := httptest.NewRecorder()
	handler.ServeHTTP(getResponse, get)

	if getResponse.Code != http.StatusOK {
		t.Fatalf("consent read status = %d, want %d", getResponse.Code, http.StatusOK)
	}
	var persisted struct {
		ConsentGiven   bool   `json:"consentGiven"`
		ConsentVersion string `json:"consentVersion"`
	}
	if err := json.NewDecoder(getResponse.Body).Decode(&persisted); err != nil {
		t.Fatalf("decode persisted consent response: %v", err)
	}
	if !persisted.ConsentGiven || persisted.ConsentVersion != postgres.VoiceConsentVersion {
		t.Fatalf(
			"persisted consent = consentGiven:%t consentVersion:%q, want true and current version",
			persisted.ConsentGiven,
			persisted.ConsentVersion,
		)
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

func TestVoiceOutputPreferencesRequireAuthenticationAndConsent(t *testing.T) {
	fixture := openCreditPolicyIntegrationFixture(t)
	userID := "speech-output-consent-user"
	sessionID := fixture.createUserAndSession(
		t,
		userID,
		"speech-output-consent@example.test",
		true,
	)
	if err := fixture.store.EnsurePersonalWorkspace(fixture.ctx, userID); err != nil {
		t.Fatalf("ensure test personal workspace: %v", err)
	}
	if err := fixture.store.SetWorkspaceMembership(
		fixture.ctx,
		postgres.DefaultWorkspaceID(userID),
		userID,
		"active",
		"owner",
		policy.PersonalWorkspaceCapabilities,
	); err != nil {
		t.Fatalf("set test workspace membership: %v", err)
	}

	handler := NewHandler(config.Config{}, fixture.store, slog.Default(), "askolo_session").Routes()
	anonymous := httptest.NewRequest(http.MethodGet, "/api/ai/voice-output-preferences", nil)
	anonymousResponse := httptest.NewRecorder()
	handler.ServeHTTP(anonymousResponse, anonymous)
	if anonymousResponse.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous preferences status = %d, want %d: %s",
			anonymousResponse.Code, http.StatusUnauthorized, anonymousResponse.Body.String())
	}
	anonymousSpeech := httptest.NewRequest(
		http.MethodPost,
		"/api/ai/assistant/runs/not-owned-by-this-user/speech",
		nil,
	)
	anonymousSpeechResponse := httptest.NewRecorder()
	handler.ServeHTTP(anonymousSpeechResponse, anonymousSpeech)
	if anonymousSpeechResponse.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous speech status = %d, want %d: %s",
			anonymousSpeechResponse.Code, http.StatusUnauthorized, anonymousSpeechResponse.Body.String())
	}

	authenticatedRequest := func(method, path, body string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(method, path, strings.NewReader(body))
		request.Header.Set("Authorization", "Bearer "+sessionID)
		if body != "" {
			request.Header.Set("Content-Type", "application/json")
		}
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		return response
	}

	preferences := authenticatedRequest(http.MethodGet, "/api/ai/voice-output-preferences", "")
	if preferences.Code != http.StatusOK {
		t.Fatalf("initial preferences status = %d, want %d: %s",
			preferences.Code, http.StatusOK, preferences.Body.String())
	}
	var initial struct {
		ConsentGiven   bool    `json:"consentGiven"`
		ConsentVersion *string `json:"consentVersion"`
		AutoSpeak      bool    `json:"autoSpeakEnabled"`
	}
	if err := json.Unmarshal(preferences.Body.Bytes(), &initial); err != nil {
		t.Fatalf("decode initial preferences: %v", err)
	}
	if initial.ConsentGiven || initial.ConsentVersion != nil {
		t.Fatalf("initial preferences = %+v, want consent disabled", initial)
	}

	if _, err := fixture.pool.Exec(
		fixture.ctx,
		`INSERT INTO voice_output_preferences(user_id, consent_at, consent_version, auto_speak_enabled)
		 VALUES($1, NOW(), 'azure-tts-v1', TRUE)
		 ON CONFLICT(user_id) DO UPDATE SET
		   consent_at=EXCLUDED.consent_at,
		   consent_version=EXCLUDED.consent_version,
		   auto_speak_enabled=EXCLUDED.auto_speak_enabled`,
		userID,
	); err != nil {
		t.Fatalf("seed an outdated Azure consent: %v", err)
	}
	stalePreferences := authenticatedRequest(http.MethodGet, "/api/ai/voice-output-preferences", "")
	var stale struct {
		ConsentGiven bool `json:"consentGiven"`
		AutoSpeak    bool `json:"autoSpeakEnabled"`
	}
	if err := json.Unmarshal(stalePreferences.Body.Bytes(), &stale); err != nil {
		t.Fatalf("decode outdated preferences: %v", err)
	}
	if stalePreferences.Code != http.StatusOK || stale.ConsentGiven || stale.AutoSpeak {
		t.Fatalf("outdated preferences = status %d, %+v; want stale consent and auto-speak disabled",
			stalePreferences.Code, stale)
	}

	speechPath := "/api/ai/assistant/runs/not-owned-by-this-user/speech"
	speech := authenticatedRequest(http.MethodPost, speechPath, "")
	if speech.Code != http.StatusForbidden ||
		!strings.Contains(speech.Body.String(), `"VOICE_OUTPUT_CONSENT_REQUIRED"`) {
		t.Fatalf("speech without consent status/body = %d %s, want consent-required 403",
			speech.Code, speech.Body.String())
	}

	enabled := authenticatedRequest(
		http.MethodPatch,
		"/api/ai/voice-output-preferences",
		`{"consent":true}`,
	)
	if enabled.Code != http.StatusOK {
		t.Fatalf("enable consent status = %d, want %d: %s",
			enabled.Code, http.StatusOK, enabled.Body.String())
	}
	var granted struct {
		ConsentGiven   bool   `json:"consentGiven"`
		ConsentVersion string `json:"consentVersion"`
		AutoSpeak      bool   `json:"autoSpeakEnabled"`
	}
	if err := json.Unmarshal(enabled.Body.Bytes(), &granted); err != nil {
		t.Fatalf("decode granted preferences: %v", err)
	}
	if !granted.ConsentGiven || granted.ConsentVersion != postgres.VoiceOutputConsentVersion {
		t.Fatalf("granted preferences = %+v, want current consent version", granted)
	}
	if granted.AutoSpeak {
		t.Fatalf("re-consenting to Azure speech unexpectedly restored the old automatic-speech choice: %+v", granted)
	}

	autoSpeak := authenticatedRequest(
		http.MethodPatch,
		"/api/ai/voice-output-preferences",
		`{"autoSpeakEnabled":true}`,
	)
	var autoSpeakPreferences struct {
		ConsentGiven bool `json:"consentGiven"`
		AutoSpeak    bool `json:"autoSpeakEnabled"`
	}
	if autoSpeak.Code != http.StatusOK {
		t.Fatalf("enable automatic spoken replies status = %d: %s", autoSpeak.Code, autoSpeak.Body.String())
	}
	if err := json.Unmarshal(autoSpeak.Body.Bytes(), &autoSpeakPreferences); err != nil {
		t.Fatalf("decode automatic-speech preferences: %v", err)
	}
	if !autoSpeakPreferences.ConsentGiven || !autoSpeakPreferences.AutoSpeak {
		t.Fatalf("explicit automatic-speech choice = %+v, want consent and auto-speak enabled", autoSpeakPreferences)
	}

	ownerID := "speech-output-other-owner"
	_ = fixture.createUserAndSession(t, ownerID, "speech-output-owner@example.test", true)
	conversationID := "speech-output-other-conversation"
	foreignRunID := "speech-output-private-run"
	if _, err := fixture.pool.Exec(
		fixture.ctx,
		`INSERT INTO assistant_conversations(id,user_id,workspace_id,title)
		 VALUES($1,$2,$3,'Assistant')`,
		conversationID, ownerID, postgres.DefaultWorkspaceID(ownerID),
	); err != nil {
		t.Fatalf("insert other user's assistant conversation: %v", err)
	}
	if _, err := fixture.pool.Exec(
		fixture.ctx,
		`INSERT INTO assistant_runs(
			id,conversation_id,user_id,idempotency_key_hash,transcript,transcript_sha256,
			state,reservation_id,base_credits,reserved_credits,policy_version,assistant_message
		) VALUES($1,$2,$3,$4,'test request',$5,'completed','test-reservation',1,1,1,'Private assistant reply')`,
		foreignRunID, conversationID, ownerID, strings.Repeat("a", 64), strings.Repeat("b", 64),
	); err != nil {
		t.Fatalf("insert other user's assistant run: %v", err)
	}
	foreignRunPath := "/api/ai/assistant/runs/" + foreignRunID + "/speech"
	foreignRun := authenticatedRequest(http.MethodPost, foreignRunPath, "")
	if foreignRun.Code != http.StatusNotFound ||
		!strings.Contains(foreignRun.Body.String(), `"NOT_FOUND"`) ||
		strings.Contains(foreignRun.Body.String(), "Private assistant reply") {
		t.Fatalf("speech for another user's run status/body = %d %s, want opaque 404",
			foreignRun.Code, foreignRun.Body.String())
	}

	revoked := authenticatedRequest(
		http.MethodPatch,
		"/api/ai/voice-output-preferences",
		`{"consent":false}`,
	)
	if revoked.Code != http.StatusOK {
		t.Fatalf("revoke consent status = %d, want %d: %s",
			revoked.Code, http.StatusOK, revoked.Body.String())
	}
	var revokedPreferences struct {
		ConsentGiven   bool    `json:"consentGiven"`
		ConsentVersion *string `json:"consentVersion"`
	}
	if err := json.Unmarshal(revoked.Body.Bytes(), &revokedPreferences); err != nil {
		t.Fatalf("decode revoked preferences: %v", err)
	}
	if revokedPreferences.ConsentGiven || revokedPreferences.ConsentVersion != nil {
		t.Fatalf("revoked preferences = %+v, want consent disabled", revokedPreferences)
	}

	speechAfterRevocation := authenticatedRequest(http.MethodPost, foreignRunPath, "")
	if speechAfterRevocation.Code != http.StatusForbidden ||
		!strings.Contains(speechAfterRevocation.Body.String(), `"VOICE_OUTPUT_CONSENT_REQUIRED"`) {
		t.Fatalf("speech after revocation status/body = %d %s, want consent-required 403",
			speechAfterRevocation.Code, speechAfterRevocation.Body.String())
	}
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

func TestAICreditEstimateResolvesPublicPricingKeysAndUnits(t *testing.T) {
	fixture := openCreditPolicyIntegrationFixture(t)
	userID := "credit-estimate-user"
	sessionID := fixture.createUserAndSession(t, userID, "estimate@example.test", true)
	if err := fixture.store.EnsurePersonalWorkspace(fixture.ctx, userID); err != nil {
		t.Fatalf("ensure test personal workspace: %v", err)
	}
	if err := fixture.store.SetWorkspaceMembership(
		fixture.ctx,
		postgres.DefaultWorkspaceID(userID),
		userID,
		"active",
		"owner",
		policy.PersonalWorkspaceCapabilities,
	); err != nil {
		t.Fatalf("set test workspace membership: %v", err)
	}

	handler := NewHandler(config.Config{}, fixture.store, slog.Default(), "askolo_session").Routes()
	estimate := func(key, units string) *httptest.ResponseRecorder {
		query := url.Values{}
		query.Set("pricingKey", key)
		query.Set("units", units)
		request := httptest.NewRequest(http.MethodGet, "/api/ai/credits/estimate?"+query.Encode(), nil)
		request.Header.Set("Authorization", "Bearer "+sessionID)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		return response
	}

	current, err := fixture.store.USDPolicy(fixture.ctx)
	if err != nil {
		t.Fatalf("read initial USD policy: %v", err)
	}
	if current.RateCards == nil {
		current.RateCards = make(map[string]postgres.USDRateCard)
	}
	delete(current.RateCards, "openai:assistant:"+assistantPlannerModel)
	savePolicy := func(next postgres.USDPolicy, reason string) postgres.USDPolicy {
		t.Helper()
		expectedVersion := next.Version
		next.Version++
		next.ChangeReason = reason
		updated, updateErr := fixture.store.UpdateUSDPolicy(fixture.ctx, expectedVersion, next, userID)
		if updateErr != nil {
			t.Fatalf("save test USD policy: %v", updateErr)
		}
		return updated
	}
	current = savePolicy(current, "verify missing assistant rate card handling")

	missingAssistantCard := estimate("assistant", "1")
	if missingAssistantCard.Code != http.StatusServiceUnavailable ||
		!strings.Contains(missingAssistantCard.Body.String(), `"code":"AI_POLICY_INVALID"`) {
		t.Fatalf("missing assistant card status/body = %d %q, want AI_POLICY_INVALID", missingAssistantCard.Code, missingAssistantCard.Body.String())
	}

	current.RateCards["openai:assistant:"+assistantPlannerModel] = postgres.USDRateCard{
		Provider: "openai", Mode: "assistant", Model: assistantPlannerModel, Meter: "tokens",
		InputUsdMicrosPerMillion:  3_000_000,
		OutputUsdMicrosPerMillion: 5_000_000,
	}
	current.RateCards["assemblyai:recorded:"+assemblyAIRealtimeSpeechModel] = postgres.USDRateCard{
		Provider: "assemblyai", Mode: "recorded", Model: assemblyAIRealtimeSpeechModel,
		Meter: "hour", UsdMicrosPerHour: 210_000,
	}
	current.RateCards["assemblyai:realtime:"+assemblyAIRealtimeSpeechModel] = postgres.USDRateCard{
		Provider: "assemblyai", Mode: "realtime", Model: assemblyAIRealtimeSpeechModel,
		Meter: "hour", UsdMicrosPerHour: 450_000,
	}
	current = savePolicy(current, "verify public estimate key resolution")

	for _, test := range []struct {
		name         string
		key          string
		units        string
		wantUnits    int
		wantUnit     string
		wantEstimate int64
	}{
		{name: "one assistant request uses reservation token caps", key: "assistant", units: "1", wantUnits: 1, wantUnit: "request", wantEstimate: 13_568},
		{name: "recorded voice units are seconds", key: "voice.recorded", units: "60", wantUnits: 60, wantUnit: "seconds", wantEstimate: 3_500},
		{name: "realtime voice units are seconds", key: "voice.realtime", units: "60", wantUnits: 60, wantUnit: "seconds", wantEstimate: 7_500},
	} {
		t.Run(test.name, func(t *testing.T) {
			response := estimate(test.key, test.units)
			if response.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d; body=%q", response.Code, http.StatusOK, response.Body.String())
			}
			var result struct {
				PricingKey         string `json:"pricingKey"`
				Units              int    `json:"units"`
				Unit               string `json:"unit"`
				EstimatedUsdMicros int64  `json:"estimatedUsdMicros"`
			}
			if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
				t.Fatalf("decode estimate response: %v", err)
			}
			if result.PricingKey != test.key || result.Units != test.wantUnits ||
				result.Unit != test.wantUnit || result.EstimatedUsdMicros != test.wantEstimate {
				t.Fatalf("estimate = %#v, want key %q, units %d %s, amount %d",
					result, test.key, test.wantUnits, test.wantUnit, test.wantEstimate)
			}
		})
	}

	for _, test := range []struct {
		name  string
		key   string
		units string
	}{
		{name: "unknown key", key: "unknown", units: "1"},
		{name: "zero units", key: "voice.recorded", units: "0"},
		{name: "non-numeric units", key: "voice.recorded", units: "not-a-number"},
		{name: "units over limit", key: "assistant", units: "10000001"},
		{name: "assistant estimate must cover one request", key: "assistant", units: "2"},
	} {
		t.Run(test.name, func(t *testing.T) {
			response := estimate(test.key, test.units)
			if response.Code != http.StatusBadRequest ||
				!strings.Contains(response.Body.String(), `"code":"INVALID_ESTIMATE"`) {
				t.Fatalf("status/body = %d %q, want INVALID_ESTIMATE", response.Code, response.Body.String())
			}
		})
	}
}

func TestAICreditBalanceAndUsageRoutesExposeUSDLedger(t *testing.T) {
	fixture := openCreditPolicyIntegrationFixture(t)
	const userID = "usd-ledger-viewer"
	sessionID := fixture.createUserAndSession(t, userID, "usd-ledger-viewer@example.test", true)
	if err := fixture.store.EnsurePersonalWorkspace(fixture.ctx, userID); err != nil {
		t.Fatalf("ensure personal workspace: %v", err)
	}
	if err := fixture.store.SetWorkspaceMembership(
		fixture.ctx,
		postgres.DefaultWorkspaceID(userID),
		userID,
		"active",
		"owner",
		policy.PersonalWorkspaceCapabilities,
	); err != nil {
		t.Fatalf("grant test workspace capabilities: %v", err)
	}
	if _, err := fixture.pool.Exec(
		fixture.ctx,
		`INSERT INTO ai_credit_accounts (user_id, granted_usd_micros) VALUES ($1, $2)`,
		userID,
		int64(2_500_000),
	); err != nil {
		t.Fatalf("seed isolated USD test balance: %v", err)
	}

	handler := NewHandler(config.Config{}, fixture.store, slog.Default(), "askolo_session").Routes()
	request := func(path string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Authorization", "Bearer "+sessionID)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, req)
		if response.Code != http.StatusOK {
			t.Fatalf("GET %s status = %d, want %d: %s", path, response.Code, http.StatusOK, response.Body.String())
		}
		return response
	}

	t.Run("balance route", func(t *testing.T) {
		var got struct {
			Currency       string `json:"currency"`
			BalanceUSD     int64  `json:"balanceUsdMicros"`
			GrantedUSD     int64  `json:"grantedUsdMicros"`
			AdjustmentsUSD int64  `json:"adjustmentsUsdMicros"`
			ReservedUSD    int64  `json:"reservedUsdMicros"`
			SpentUSD       int64  `json:"spentUsdMicros"`
			RefundedUSD    int64  `json:"refundedUsdMicros"`
			PolicyVersion  int    `json:"policyVersion"`
			CanManage      bool   `json:"canManage"`
			Enforcement    string `json:"enforcement"`
		}
		if err := json.NewDecoder(request("/api/ai/credits").Body).Decode(&got); err != nil {
			t.Fatalf("decode balance response: %v", err)
		}
		if got.Currency != "USD" || got.BalanceUSD != 2_500_000 || got.GrantedUSD != 2_500_000 {
			t.Fatalf("balance fields = %#v, want USD balance and grant of 2500000 micro-USD", got)
		}
		if got.AdjustmentsUSD != 0 || got.ReservedUSD != 0 || got.SpentUSD != 0 || got.RefundedUSD != 0 {
			t.Fatalf("unexpected nonzero ledger totals: %#v", got)
		}
		if got.PolicyVersion < 1 || got.CanManage || got.Enforcement != "strict" {
			t.Fatalf("unexpected policy/access fields: %#v", got)
		}
	})

	t.Run("usage route", func(t *testing.T) {
		var got struct {
			Currency string `json:"currency"`
			Usage    struct {
				BalanceUSD     int64 `json:"balanceUsdMicros"`
				GrantedUSD     int64 `json:"grantedUsdMicros"`
				AdjustmentsUSD int64 `json:"adjustmentsUsdMicros"`
				ReservedUSD    int64 `json:"reservedUsdMicros"`
				SpentUSD       int64 `json:"spentUsdMicros"`
				RefundedUSD    int64 `json:"refundedUsdMicros"`
			} `json:"usage"`
			Reservations []json.RawMessage `json:"reservations"`
			Events       []json.RawMessage `json:"events"`
			Adjustments  []json.RawMessage `json:"adjustments"`
			Grants       []json.RawMessage `json:"grants"`
		}
		if err := json.NewDecoder(request("/api/ai/credits/usage").Body).Decode(&got); err != nil {
			t.Fatalf("decode usage response: %v", err)
		}
		if got.Currency != "USD" || got.Usage.BalanceUSD != 2_500_000 || got.Usage.GrantedUSD != 2_500_000 {
			t.Fatalf("usage totals = %#v, want USD balance and grant of 2500000 micro-USD", got)
		}
		if got.Usage.AdjustmentsUSD != 0 || got.Usage.ReservedUSD != 0 || got.Usage.SpentUSD != 0 || got.Usage.RefundedUSD != 0 {
			t.Fatalf("unexpected nonzero usage totals: %#v", got.Usage)
		}
		if got.Reservations == nil || got.Events == nil || got.Adjustments == nil || got.Grants == nil {
			t.Fatalf("empty history collections must be arrays, got %#v", got)
		}
		if len(got.Reservations)+len(got.Events)+len(got.Adjustments)+len(got.Grants) != 0 {
			t.Fatalf("unexpected history for a newly seeded balance: %#v", got)
		}
	})
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
