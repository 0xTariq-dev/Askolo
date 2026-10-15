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
	handler := NewHandler(config.Config{
		AuthRateLimitHMACSecret: "test-auth-rate-limit-hmac-secret-32-bytes",
	}, fixture.store, slog.Default(), "askolo_session").Routes()
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
	createdTask := request(ownerSession, http.MethodPost, "/api/v2/workspaces/"+workspaceID+"/tasks",
		`{"kind":"task","title":"Group task","assignedToGroupId":"`+group.ID+`","timeZone":"UTC"}`)
	if createdTask.Code != http.StatusCreated {
		t.Fatalf("create group task status=%d: %s", createdTask.Code, createdTask.Body.String())
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
}
