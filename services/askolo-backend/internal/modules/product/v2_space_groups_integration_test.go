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

func TestV2SpaceGroupsPermissionsAndTaskAssignment(t *testing.T) {
	fixture := openCreditPolicyIntegrationFixture(t)
	const ownerID, memberID, workspaceID = "groups-owner", "groups-member", "groups-space"
	ownerSession := fixture.createUserAndSession(t, ownerID, "groups-owner@example.test", true)
	memberSession := fixture.createUserAndSession(t, memberID, "groups-member@example.test", true)
	if err := fixture.store.CreateWorkspace(fixture.ctx, workspaceID, "Groups", ownerID, []string{"workspace:manage"}); err != nil {
		t.Fatalf("create Space: %v", err)
	}
	if err := fixture.store.SetWorkspaceMembership(fixture.ctx, workspaceID, memberID, "active", "member", []string{}); err != nil {
		t.Fatalf("add Space member: %v", err)
	}
	var assignmentEmailTo string
	productHandler := NewHandler(config.Config{
		AuthRateLimitHMACSecret: "test-auth-rate-limit-hmac-secret-32-bytes",
	}, fixture.store, slog.Default(), "askolo_session")
	productHandler.SetSpaceInviteMailer(func(_ context.Context, to, _, _ string) error {
		assignmentEmailTo = to
		return nil
	})
	handler := productHandler.Routes()
	request := func(session, method, path, body string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+session)
		if body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		return res
	}

	groupResponse := request(ownerSession, http.MethodPost, "/api/v2/workspaces/"+workspaceID+"/groups",
		`{"name":"Editors","memberUserIds":["`+memberID+`"],"permissions":["resource.update.any"]}`)
	if groupResponse.Code != http.StatusCreated {
		t.Fatalf("create group status=%d: %s", groupResponse.Code, groupResponse.Body.String())
	}
	var group struct {
		ID          string   `json:"id"`
		Permissions []string `json:"permissions"`
	}
	if err := json.NewDecoder(groupResponse.Body).Decode(&group); err != nil || group.ID == "" || len(group.Permissions) != 1 {
		t.Fatalf("created group=%+v err=%v", group, err)
	}
	preferences := request(memberSession, http.MethodPut, "/api/v2/workspaces/"+workspaceID+"/notification-preferences",
		`{"preferences":[{"eventKey":"assignments","emailEnabled":true,"inAppEnabled":true},{"eventKey":"reminders","emailEnabled":false,"inAppEnabled":true}]}`)
	if preferences.Code != http.StatusOK {
		t.Fatalf("enable assignment email preference status=%d: %s", preferences.Code, preferences.Body.String())
	}
	createdTask := request(ownerSession, http.MethodPost, "/api/v2/workspaces/"+workspaceID+"/tasks",
		`{"kind":"task","title":"Group task","assignedToGroupId":"`+group.ID+`","timeZone":"UTC"}`)
	if createdTask.Code != http.StatusCreated {
		t.Fatalf("create group task status=%d: %s", createdTask.Code, createdTask.Body.String())
	}
	// Reassigning the task to its current group does not duplicate notices; a
	// fresh assignment creates one persistent inbox item and one opted-in email.
	reassigned := request(ownerSession, http.MethodPatch, "/api/v2/workspaces/"+workspaceID+"/tasks/"+mustTaskID(t, createdTask.Body.Bytes()),
		`{"assignedToGroupId":"`+group.ID+`"}`)
	if reassigned.Code != http.StatusOK {
		t.Fatalf("update group assignment status=%d: %s", reassigned.Code, reassigned.Body.String())
	}
	var inbox []struct {
		ID    string `json:"id"`
		Title string `json:"title"`
	}
	inboxResponse := request(memberSession, http.MethodGet, "/api/v2/notifications", "")
	if inboxResponse.Code != http.StatusOK || json.NewDecoder(inboxResponse.Body).Decode(&inbox) != nil || len(inbox) != 1 {
		t.Fatalf("member assignment inbox status=%d rows=%+v: %s", inboxResponse.Code, inbox, inboxResponse.Body.String())
	}
	if inbox[0].Title != "Group task" || assignmentEmailTo != "groups-member@example.test" {
		t.Fatalf("assignment notification title=%q email=%q", inbox[0].Title, assignmentEmailTo)
	}
	readResponse := request(memberSession, http.MethodPost, "/api/v2/notifications/"+inbox[0].ID+"/read", "")
	if readResponse.Code != http.StatusNoContent {
		t.Fatalf("mark assignment notification read status=%d: %s", readResponse.Code, readResponse.Body.String())
	}
	var task struct {
		ID                string  `json:"id"`
		AssignedToGroupID *string `json:"assignedToGroupId"`
	}
	if err := json.NewDecoder(createdTask.Body).Decode(&task); err != nil || task.ID == "" || task.AssignedToGroupID == nil || *task.AssignedToGroupID != group.ID {
		t.Fatalf("created task=%+v err=%v", task, err)
	}
	unassigned := request(ownerSession, http.MethodPost, "/api/v2/workspaces/"+workspaceID+"/tasks",
		`{"kind":"task","title":"Permission test","timeZone":"UTC"}`)
	if unassigned.Code != http.StatusCreated {
		t.Fatalf("create permission test task status=%d: %s", unassigned.Code, unassigned.Body.String())
	}
	var permissionTask struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(unassigned.Body).Decode(&permissionTask); err != nil || permissionTask.ID == "" {
		t.Fatalf("decode permission task: %+v %v", permissionTask, err)
	}
	updated := request(memberSession, http.MethodPatch, "/api/v2/workspaces/"+workspaceID+"/tasks/"+permissionTask.ID,
		`{"title":"Updated by group permission"}`)
	if updated.Code != http.StatusOK {
		t.Fatalf("group permission update status=%d: %s", updated.Code, updated.Body.String())
	}

	// Removing group membership revokes both group assignment access and the
	// additive permission on the next request.
	if err := fixture.store.ReplaceV2WorkspaceGroup(fixture.ctx, workspaceID, group.ID, ownerID,
		"Editors", nil, []string{}, group.Permissions); err != nil {
		t.Fatalf("remove group member: %v", err)
	}
	revoked := request(memberSession, http.MethodPatch, "/api/v2/workspaces/"+workspaceID+"/tasks/"+permissionTask.ID,
		`{"title":"Must be rejected"}`)
	if revoked.Code != http.StatusForbidden {
		t.Fatalf("permission after group removal status=%d, want 403: %s", revoked.Code, revoked.Body.String())
	}
	removed := request(ownerSession, http.MethodDelete, "/api/v2/workspaces/"+workspaceID+"/members/"+memberID, "")
	if removed.Code != http.StatusNoContent {
		t.Fatalf("remove member status=%d: %s", removed.Code, removed.Body.String())
	}
	afterRemoval := request(memberSession, http.MethodGet, "/api/v2/workspaces/"+workspaceID+"/groups", "")
	if afterRemoval.Code == http.StatusOK {
		t.Fatal("removed member still has access to Space groups")
	}
}

func mustTaskID(t *testing.T, raw []byte) string {
	t.Helper()
	var task struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(raw, &task); err != nil || task.ID == "" {
		t.Fatalf("decode task id: %+v %v", task, err)
	}
	return task.ID
}
