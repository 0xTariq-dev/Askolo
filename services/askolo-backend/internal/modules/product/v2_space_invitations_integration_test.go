package product

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"askolo/backend/internal/config"
)

func TestV2SpaceInvitationsAreEmailBoundAndRoleProtected(t *testing.T) {
	fixture := openCreditPolicyIntegrationFixture(t)
	const (
		ownerID     = "space-invite-owner"
		adminID     = "space-invite-admin"
		memberID    = "space-invite-member"
		inviteeID   = "space-invitee"
		declinerID  = "space-invite-decliner"
		workspaceID = "space-invite-shared"
	)
	ownerSession := fixture.createUserAndSession(t, ownerID, "invite-owner@example.test", true)
	adminSession := fixture.createUserAndSession(t, adminID, "invite-admin@example.test", true)
	memberSession := fixture.createUserAndSession(t, memberID, "invite-member@example.test", true)
	inviteeSession := fixture.createUserAndSession(t, inviteeID, "invitee@example.test", true)
	declinerSession := fixture.createUserAndSession(t, declinerID, "decliner@example.test", true)
	if err := fixture.store.CreateWorkspace(fixture.ctx, workspaceID, "Shared Space", ownerID, []string{"workspace:manage"}); err != nil {
		t.Fatalf("create Space: %v", err)
	}
	if err := fixture.store.SetWorkspaceMembership(fixture.ctx, workspaceID, adminID, "active", "admin", []string{}); err != nil {
		t.Fatalf("add Space admin: %v", err)
	}
	if err := fixture.store.SetWorkspaceMembership(fixture.ctx, workspaceID, memberID, "active", "member", []string{}); err != nil {
		t.Fatalf("add Space member: %v", err)
	}

	var sentTo, sentBody string
	productHandler := NewHandler(config.Config{
		AuthRateLimitHMACSecret: "test-auth-rate-limit-hmac-secret-32-bytes",
		CanonicalOrigin:         "https://app.askolo.test",
	}, fixture.store, slog.Default(), "askolo_session")
	productHandler.SetSpaceInviteMailer(func(_ context.Context, to, _, body string) error {
		sentTo, sentBody = to, body
		return nil
	})
	handler := productHandler.Routes()
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

	const invitationsPath = "/api/v2/workspaces/" + workspaceID + "/invitations"
	adminInvite := request(adminSession, http.MethodPost, invitationsPath, `{"email":"not-owner@example.test","role":"admin"}`)
	if adminInvite.Code != http.StatusForbidden {
		t.Fatalf("admin invite role status = %d, want %d: %s", adminInvite.Code, http.StatusForbidden, adminInvite.Body.String())
	}
	memberList := request(memberSession, http.MethodGet, invitationsPath, "")
	if memberList.Code != http.StatusForbidden {
		t.Fatalf("member invitation list status = %d, want %d: %s", memberList.Code, http.StatusForbidden, memberList.Body.String())
	}

	created := request(ownerSession, http.MethodPost, invitationsPath, `{"email":"invitee@example.test","role":"member"}`)
	if created.Code != http.StatusCreated {
		t.Fatalf("create invitation status = %d: %s", created.Code, created.Body.String())
	}
	var invitation struct {
		ID     string `json:"id"`
		Email  string `json:"email"`
		Role   string `json:"role"`
		Status string `json:"status"`
	}
	if err := json.NewDecoder(created.Body).Decode(&invitation); err != nil {
		t.Fatalf("decode invitation: %v", err)
	}
	if invitation.ID == "" || invitation.Email != "invitee@example.test" || invitation.Role != "member" || invitation.Status != "pending" {
		t.Fatalf("created invitation = %+v", invitation)
	}
	if sentTo != invitation.Email || !strings.Contains(sentBody, "https://app.askolo.test/invite/") {
		t.Fatalf("invitation email recipient/body was not correct")
	}
	if strings.Contains(created.Body.String(), "/invite/") {
		t.Fatalf("raw invitation link must not be returned in the API response: %s", created.Body.String())
	}
	linkLine := strings.TrimSpace(sentBody[strings.LastIndex(sentBody, "https://app.askolo.test/invite/"):])
	if cut := strings.IndexAny(linkLine, "\r\n "); cut >= 0 {
		linkLine = linkLine[:cut]
	}
	token := strings.TrimPrefix(linkLine, "https://app.askolo.test/invite/")
	if token == "" {
		t.Fatal("invitation email did not contain a token")
	}
	acceptPath := "/api/v2/invitations/accept"
	wrongAccount := request(memberSession, http.MethodPost, acceptPath, `{"token":"`+token+`"}`)
	if wrongAccount.Code != http.StatusForbidden {
		t.Fatalf("wrong-email acceptance status = %d, want %d: %s", wrongAccount.Code, http.StatusForbidden, wrongAccount.Body.String())
	}
	accepted := request(inviteeSession, http.MethodPost, acceptPath, `{"token":"`+token+`"}`)
	if accepted.Code != http.StatusOK || !strings.Contains(accepted.Body.String(), workspaceID) {
		t.Fatalf("accept invitation status = %d: %s", accepted.Code, accepted.Body.String())
	}
	replay := request(inviteeSession, http.MethodPost, acceptPath, `{"token":"`+token+`"}`)
	if replay.Code != http.StatusNotFound {
		t.Fatalf("accepted invitation replay status = %d, want %d: %s", replay.Code, http.StatusNotFound, replay.Body.String())
	}

	pending := request(ownerSession, http.MethodPost, invitationsPath, `{"email":"pending@example.test","role":"member"}`)
	if pending.Code != http.StatusCreated {
		t.Fatalf("create second invitation status = %d: %s", pending.Code, pending.Body.String())
	}
	var pendingInvitation struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(pending.Body).Decode(&pendingInvitation); err != nil {
		t.Fatalf("decode second invitation: %v", err)
	}
	cancelled := request(adminSession, http.MethodDelete, invitationsPath+"/"+pendingInvitation.ID, "")
	if cancelled.Code != http.StatusNoContent {
		t.Fatalf("admin cancel invitation status = %d, want %d: %s", cancelled.Code, http.StatusNoContent, cancelled.Body.String())
	}
	remaining := request(ownerSession, http.MethodGet, invitationsPath, "")
	if remaining.Code != http.StatusOK || !strings.Contains(remaining.Body.String(), "[]") {
		t.Fatalf("pending invitation list after acceptance and cancellation = %d: %s", remaining.Code, remaining.Body.String())
	}

	declineInvite := request(ownerSession, http.MethodPost, invitationsPath, `{"email":"decliner@example.test","role":"member"}`)
	if declineInvite.Code != http.StatusCreated {
		t.Fatalf("create invitation for decline status = %d: %s", declineInvite.Code, declineInvite.Body.String())
	}
	declineLinkLine := strings.TrimSpace(sentBody[strings.LastIndex(sentBody, "https://app.askolo.test/invite/"):])
	if cut := strings.IndexAny(declineLinkLine, "\r\n "); cut >= 0 {
		declineLinkLine = declineLinkLine[:cut]
	}
	declineToken := strings.TrimPrefix(declineLinkLine, "https://app.askolo.test/invite/")
	if declineToken == "" {
		t.Fatal("invitation email for decline did not contain a token")
	}
	wrongDecliner := request(memberSession, http.MethodPost, "/api/v2/invitations/decline", `{"token":"`+declineToken+`"}`)
	if wrongDecliner.Code != http.StatusForbidden {
		t.Fatalf("wrong-email decline status = %d, want %d: %s", wrongDecliner.Code, http.StatusForbidden, wrongDecliner.Body.String())
	}
	declined := request(declinerSession, http.MethodPost, "/api/v2/invitations/decline", `{"token":"`+declineToken+`"}`)
	if declined.Code != http.StatusOK || !strings.Contains(declined.Body.String(), workspaceID) {
		t.Fatalf("decline invitation status = %d: %s", declined.Code, declined.Body.String())
	}
	declineReplay := request(declinerSession, http.MethodPost, "/api/v2/invitations/decline", `{"token":"`+declineToken+`"}`)
	if declineReplay.Code != http.StatusNotFound {
		t.Fatalf("declined invitation replay status = %d, want %d: %s", declineReplay.Code, http.StatusNotFound, declineReplay.Body.String())
	}

	workspacePath := "/api/v2/workspaces/" + workspaceID
	ownerLeave := request(ownerSession, http.MethodDelete, workspacePath+"/membership", "")
	if ownerLeave.Code != http.StatusForbidden {
		t.Fatalf("owner leave status = %d, want %d: %s", ownerLeave.Code, http.StatusForbidden, ownerLeave.Body.String())
	}
	adminDelete := request(adminSession, http.MethodDelete, workspacePath, "")
	if adminDelete.Code != http.StatusForbidden {
		t.Fatalf("admin delete status = %d, want %d: %s", adminDelete.Code, http.StatusForbidden, adminDelete.Body.String())
	}
	memberLeave := request(memberSession, http.MethodDelete, workspacePath+"/membership", "")
	if memberLeave.Code != http.StatusNoContent {
		t.Fatalf("member leave status = %d, want %d: %s", memberLeave.Code, http.StatusNoContent, memberLeave.Body.String())
	}
	memberAfterLeave := request(memberSession, http.MethodGet, workspacePath+"/members", "")
	if memberAfterLeave.Code != http.StatusNotFound {
		t.Fatalf("member access after leaving status = %d, want %d: %s", memberAfterLeave.Code, http.StatusNotFound, memberAfterLeave.Body.String())
	}

	pendingForDeletedSpace := request(ownerSession, http.MethodPost, invitationsPath, `{"email":"retained@example.test","role":"member"}`)
	if pendingForDeletedSpace.Code != http.StatusCreated {
		t.Fatalf("create invitation before Space deletion status = %d: %s", pendingForDeletedSpace.Code, pendingForDeletedSpace.Body.String())
	}
	ownerDelete := request(ownerSession, http.MethodDelete, workspacePath, "")
	if ownerDelete.Code != http.StatusNoContent {
		t.Fatalf("owner delete status = %d, want %d: %s", ownerDelete.Code, http.StatusNoContent, ownerDelete.Body.String())
	}
	ownerAfterDelete := request(ownerSession, http.MethodGet, workspacePath+"/members", "")
	if ownerAfterDelete.Code != http.StatusNotFound {
		t.Fatalf("owner access after deletion status = %d, want %d: %s", ownerAfterDelete.Code, http.StatusNotFound, ownerAfterDelete.Body.String())
	}
	invitationsAfterDelete, err := fixture.store.ListV2WorkspaceInvitations(fixture.ctx, workspaceID)
	if err != nil || len(invitationsAfterDelete) != 0 {
		t.Fatalf("pending invitations after deletion = %d, err = %v", len(invitationsAfterDelete), err)
	}
}
