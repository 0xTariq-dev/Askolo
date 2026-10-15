package product

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"askolo/backend/internal/config"
)

func TestV2SpaceSettingsArePersistedAndRoleProtected(t *testing.T) {
	fixture := openCreditPolicyIntegrationFixture(t)
	const (
		ownerID     = "space-settings-owner"
		adminID     = "space-settings-admin"
		memberID    = "space-settings-member"
		workspaceID = "space-settings-shared"
	)
	ownerSession := fixture.createUserAndSession(t, ownerID, "settings-owner@example.test", true)
	adminSession := fixture.createUserAndSession(t, adminID, "settings-admin@example.test", true)
	memberSession := fixture.createUserAndSession(t, memberID, "settings-member@example.test", true)
	if err := fixture.store.CreateWorkspace(
		fixture.ctx, workspaceID, "Before", ownerID, []string{"workspace:manage"},
	); err != nil {
		t.Fatalf("create Space: %v", err)
	}
	if err := fixture.store.SetWorkspaceMembership(
		fixture.ctx, workspaceID, adminID, "active", "admin", []string{},
	); err != nil {
		t.Fatalf("add Space admin: %v", err)
	}
	if err := fixture.store.SetWorkspaceMembership(
		fixture.ctx, workspaceID, memberID, "active", "member", []string{},
	); err != nil {
		t.Fatalf("add Space member: %v", err)
	}

	handler := NewHandler(config.Config{
		AuthRateLimitHMACSecret: "test-auth-rate-limit-hmac-secret-32-bytes",
	}, fixture.store, slog.Default(), "askolo_session").Routes()
	request := func(sessionID, method, path, body string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+sessionID)
		if body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, req)
		return response
	}

	initial := request(ownerSession, http.MethodGet, "/api/v2/workspaces/"+workspaceID, "")
	if initial.Code != http.StatusOK {
		t.Fatalf("read initial Space details status = %d: %s", initial.Code, initial.Body.String())
	}
	var before struct {
		Name     string  `json:"name"`
		TimeZone string  `json:"timeZone"`
		Slug     *string `json:"slug"`
	}
	if err := json.NewDecoder(initial.Body).Decode(&before); err != nil {
		t.Fatalf("decode initial Space details: %v", err)
	}
	if before.Name != "Before" || before.TimeZone != "UTC" || before.Slug != nil {
		t.Fatalf("initial Space settings = %+v", before)
	}

	updated := request(adminSession, http.MethodPut, "/api/v2/workspaces/"+workspaceID,
		`{"name":"Family Space","description":"A shared home","imageObjectPath":"/objects/cover-1","timeZone":"Africa/Cairo","slug":"family-home"}`)
	if updated.Code != http.StatusOK {
		t.Fatalf("update Space details status = %d: %s", updated.Code, updated.Body.String())
	}
	var after struct {
		Name            string  `json:"name"`
		Description     *string `json:"description"`
		ImageObjectPath *string `json:"imageObjectPath"`
		TimeZone        string  `json:"timeZone"`
		Slug            *string `json:"slug"`
	}
	if err := json.NewDecoder(updated.Body).Decode(&after); err != nil {
		t.Fatalf("decode updated Space details: %v", err)
	}
	if after.Name != "Family Space" || after.Description == nil || *after.Description != "A shared home" ||
		after.ImageObjectPath == nil || *after.ImageObjectPath != "/objects/cover-1" ||
		after.TimeZone != "Africa/Cairo" || after.Slug == nil || *after.Slug != "family-home" {
		t.Fatalf("updated Space settings = %+v", after)
	}

	memberRead := request(memberSession, http.MethodGet, "/api/v2/workspaces/"+workspaceID, "")
	if memberRead.Code != http.StatusOK {
		t.Fatalf("member read Space details status = %d: %s", memberRead.Code, memberRead.Body.String())
	}
	memberWrite := request(memberSession, http.MethodPut, "/api/v2/workspaces/"+workspaceID,
		`{"name":"Unauthorized","timeZone":"UTC"}`)
	if memberWrite.Code != http.StatusForbidden {
		t.Fatalf("member update status = %d, want %d: %s", memberWrite.Code, http.StatusForbidden, memberWrite.Body.String())
	}
	invalidSlug := request(ownerSession, http.MethodPut, "/api/v2/workspaces/"+workspaceID,
		`{"name":"Family Space","timeZone":"UTC","slug":"Not A Slug"}`)
	if invalidSlug.Code != http.StatusBadRequest {
		t.Fatalf("invalid slug status = %d, want %d: %s", invalidSlug.Code, http.StatusBadRequest, invalidSlug.Body.String())
	}

	ownerPreferences := request(ownerSession, http.MethodGet,
		"/api/v2/workspaces/"+workspaceID+"/notification-preferences", "")
	if ownerPreferences.Code != http.StatusOK {
		t.Fatalf("read owner preferences status = %d: %s", ownerPreferences.Code, ownerPreferences.Body.String())
	}
	var ownerPreferenceRows []struct {
		EventKey     string `json:"eventKey"`
		EmailEnabled bool   `json:"emailEnabled"`
		InAppEnabled bool   `json:"inAppEnabled"`
	}
	if err := json.NewDecoder(ownerPreferences.Body).Decode(&ownerPreferenceRows); err != nil {
		t.Fatalf("decode owner preferences: %v", err)
	}
	if len(ownerPreferenceRows) != 3 {
		t.Fatalf("owner notification categories = %d, want 3: %#v", len(ownerPreferenceRows), ownerPreferenceRows)
	}
	savedPreferences := request(ownerSession, http.MethodPut,
		"/api/v2/workspaces/"+workspaceID+"/notification-preferences",
		`{"preferences":[{"eventKey":"assignments","emailEnabled":true,"inAppEnabled":true},{"eventKey":"reminders","emailEnabled":false,"inAppEnabled":true},{"eventKey":"membership","emailEnabled":true,"inAppEnabled":false}]}`)
	if savedPreferences.Code != http.StatusOK {
		t.Fatalf("save notification preferences status = %d: %s", savedPreferences.Code, savedPreferences.Body.String())
	}
	memberPreferences := request(memberSession, http.MethodGet,
		"/api/v2/workspaces/"+workspaceID+"/notification-preferences", "")
	if memberPreferences.Code != http.StatusOK {
		t.Fatalf("read member preferences status = %d: %s", memberPreferences.Code, memberPreferences.Body.String())
	}
	var memberPreferenceRows []struct {
		EventKey string `json:"eventKey"`
	}
	if err := json.NewDecoder(memberPreferences.Body).Decode(&memberPreferenceRows); err != nil {
		t.Fatalf("decode member preferences: %v", err)
	}
	if len(memberPreferenceRows) != 2 {
		t.Fatalf("member notification categories = %d, want only assignments and reminders: %#v", len(memberPreferenceRows), memberPreferenceRows)
	}
	memberMembershipPreference := request(memberSession, http.MethodPut,
		"/api/v2/workspaces/"+workspaceID+"/notification-preferences",
		`{"preferences":[{"eventKey":"assignments","emailEnabled":false,"inAppEnabled":true},{"eventKey":"reminders","emailEnabled":false,"inAppEnabled":true},{"eventKey":"membership","emailEnabled":true,"inAppEnabled":true}]}`)
	if memberMembershipPreference.Code != http.StatusForbidden {
		t.Fatalf("member membership preference status = %d, want %d: %s", memberMembershipPreference.Code, http.StatusForbidden, memberMembershipPreference.Body.String())
	}

	ownerMembers := request(ownerSession, http.MethodGet, "/api/v2/workspaces/"+workspaceID+"/members", "")
	if ownerMembers.Code != http.StatusOK {
		t.Fatalf("list owner members status = %d: %s", ownerMembers.Code, ownerMembers.Body.String())
	}
	var ownerMemberRows []struct {
		UserID string  `json:"userId"`
		Email  *string `json:"email"`
		Role   string  `json:"role"`
	}
	if err := json.NewDecoder(ownerMembers.Body).Decode(&ownerMemberRows); err != nil {
		t.Fatalf("decode owner member list: %v", err)
	}
	if len(ownerMemberRows) != 3 || ownerMemberRows[0].Email == nil {
		t.Fatalf("owner member list should include three members and contact details: %#v", ownerMemberRows)
	}
	memberList := request(memberSession, http.MethodGet, "/api/v2/workspaces/"+workspaceID+"/members", "")
	if memberList.Code != http.StatusOK {
		t.Fatalf("list member directory status = %d: %s", memberList.Code, memberList.Body.String())
	}
	var memberRows []struct {
		UserID string  `json:"userId"`
		Email  *string `json:"email"`
		Role   string  `json:"role"`
	}
	if err := json.NewDecoder(memberList.Body).Decode(&memberRows); err != nil {
		t.Fatalf("decode member directory: %v", err)
	}
	if len(memberRows) != 3 || memberRows[0].Email != nil {
		t.Fatalf("member directory should hide email addresses: %#v", memberRows)
	}
	adminRoleChange := request(adminSession, http.MethodPut,
		"/api/v2/workspaces/"+workspaceID+"/members/"+memberID+"/role", `{"role":"admin"}`)
	if adminRoleChange.Code != http.StatusForbidden {
		t.Fatalf("admin role change status = %d, want %d: %s", adminRoleChange.Code, http.StatusForbidden, adminRoleChange.Body.String())
	}
	ownerRoleChange := request(ownerSession, http.MethodPut,
		"/api/v2/workspaces/"+workspaceID+"/members/"+memberID+"/role", `{"role":"admin"}`)
	if ownerRoleChange.Code != http.StatusNoContent {
		t.Fatalf("owner role change status = %d, want %d: %s", ownerRoleChange.Code, http.StatusNoContent, ownerRoleChange.Body.String())
	}
	ownerSelfDemotion := request(ownerSession, http.MethodPut,
		"/api/v2/workspaces/"+workspaceID+"/members/"+ownerID+"/role", `{"role":"member"}`)
	if ownerSelfDemotion.Code != http.StatusForbidden {
		t.Fatalf("owner self-demotion status = %d, want %d: %s", ownerSelfDemotion.Code, http.StatusForbidden, ownerSelfDemotion.Body.String())
	}
}
