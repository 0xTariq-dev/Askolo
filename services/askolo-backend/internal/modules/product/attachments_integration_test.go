package product

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"askolo/backend/internal/adapters/postgres"
	"askolo/backend/internal/adapters/r2"
	"askolo/backend/internal/config"
	policy "askolo/backend/internal/platform/authorization"
)

type attachmentIntegrationStorage struct {
	mu      sync.Mutex
	objects map[string]r2.Object
}

func newAttachmentIntegrationStorage() *attachmentIntegrationStorage {
	return &attachmentIntegrationStorage{objects: make(map[string]r2.Object)}
}

func (s *attachmentIntegrationStorage) PutObject(_ context.Context, key, _ string, body []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.objects[key] = r2.Object{Key: key, Size: int64(len(body)), LastModified: time.Now().UTC()}
	return nil
}

func (s *attachmentIntegrationStorage) ListObjects(_ context.Context, prefix string, limit int32) ([]r2.Object, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	keys := make([]string, 0)
	for key := range s.objects {
		if strings.HasPrefix(key, prefix) {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	if len(keys) > int(limit) {
		keys = keys[:limit]
	}
	objects := make([]r2.Object, 0, len(keys))
	for _, key := range keys {
		objects = append(objects, s.objects[key])
	}
	return objects, nil
}

func (s *attachmentIntegrationStorage) Delete(_ context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.objects, key)
	return nil
}

func (s *attachmentIntegrationStorage) PresignGetAttachment(_ context.Context, key string, _ time.Duration, fileName string) (r2.PresignedRequest, error) {
	return r2.PresignedRequest{
		URL:     "https://private-files.invalid/" + url.PathEscape(key) + "?filename=" + url.QueryEscape(fileName),
		Method:  http.MethodGet,
		Headers: make(http.Header),
	}, nil
}

func (s *attachmentIntegrationStorage) PresignGetInline(_ context.Context, key string, _ time.Duration) (r2.PresignedRequest, error) {
	return r2.PresignedRequest{
		URL:     "https://private-files.invalid/" + url.PathEscape(key),
		Method:  http.MethodGet,
		Headers: make(http.Header),
	}, nil
}

func (s *attachmentIntegrationStorage) hasPrefix(prefix string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for key := range s.objects {
		if strings.HasPrefix(key, prefix) {
			return true
		}
	}
	return false
}

func (s *attachmentIntegrationStorage) putTestObject(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.objects[key] = r2.Object{Key: key, Size: 1, LastModified: time.Now().UTC()}
}

func TestNoteAttachmentsAreOwnerScopedAndCleanedOnDeletionIntegration(t *testing.T) {
	fixture := openCreditPolicyIntegrationFixture(t)
	storage := newAttachmentIntegrationStorage()
	const (
		ownerID    = "attachment-note-owner"
		otherID    = "attachment-note-other"
		ownerEmail = "attachment-note-owner@example.test"
		otherEmail = "attachment-note-other@example.test"
	)
	ownerSession := fixture.createUserAndSession(t, ownerID, ownerEmail, true)
	otherSession := fixture.createUserAndSession(t, otherID, otherEmail, true)
	ensureAttachmentPersonalSpace(t, fixture, ownerID)
	ensureAttachmentPersonalSpace(t, fixture, otherID)

	api := attachmentIntegrationAPI(fixture, storage)
	noteID := createAttachmentNote(t, api, ownerSession, "Private note")
	uploaded := uploadAttachment(t, api, ownerSession, "/api/notes/"+noteID+"/attachments", "private.txt", "note attachment")
	notePrefix := fmt.Sprintf("attachments/notes/%s/%s/", ownerID, noteID)
	if !storage.hasPrefix(notePrefix) {
		t.Fatal("successful upload did not create an owner-scoped private object")
	}

	for _, attempt := range []struct {
		name   string
		method string
		path   string
		want   int
	}{
		{name: "list", method: http.MethodGet, path: "/api/notes/" + noteID + "/attachments", want: http.StatusNotFound},
		{name: "download", method: http.MethodGet, path: "/api/notes/" + noteID + "/attachments/" + uploaded.ID + "/download", want: http.StatusNotFound},
		{name: "delete", method: http.MethodDelete, path: "/api/notes/" + noteID + "/attachments/" + uploaded.ID, want: http.StatusNotFound},
	} {
		t.Run("other user cannot "+attempt.name, func(t *testing.T) {
			response := attachmentRequest(api, otherSession, attempt.method, attempt.path, nil, "")
			if response.Code != attempt.want {
				t.Fatalf("%s status = %d, want %d: %s", attempt.name, response.Code, attempt.want, response.Body.String())
			}
			if !storage.hasPrefix(notePrefix) {
				t.Fatalf("forbidden %s request changed the owner's private objects", attempt.name)
			}
		})
	}

	listed := attachmentRequest(api, ownerSession, http.MethodGet, "/api/notes/"+noteID+"/attachments", nil, "")
	if listed.Code != http.StatusOK || !strings.Contains(listed.Body.String(), uploaded.ID) {
		t.Fatalf("owner list status/body = %d %s", listed.Code, listed.Body.String())
	}
	download := attachmentRequest(api, ownerSession, http.MethodGet,
		"/api/notes/"+noteID+"/attachments/"+uploaded.ID+"/download", nil, "")
	if download.Code != http.StatusFound ||
		!strings.Contains(download.Header().Get("Location"), "private-files.invalid") ||
		download.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatalf("owner download status/headers = %d %#v", download.Code, download.Header())
	}
	deleted := attachmentRequest(api, ownerSession, http.MethodDelete,
		"/api/notes/"+noteID+"/attachments/"+uploaded.ID, nil, "")
	if deleted.Code != http.StatusNoContent || storage.hasPrefix(notePrefix) {
		t.Fatalf("owner attachment delete status = %d, remaining objects = %v", deleted.Code, storage.hasPrefix(notePrefix))
	}

	noteForDelete := createAttachmentNote(t, api, ownerSession, "Delete note cleanup")
	uploadAttachment(t, api, ownerSession, "/api/notes/"+noteForDelete+"/attachments", "cleanup.txt", "cleanup")
	noteDeletePrefix := fmt.Sprintf("attachments/notes/%s/%s/", ownerID, noteForDelete)
	noteDeleted := attachmentRequest(api, ownerSession, http.MethodDelete, "/api/notes/"+noteForDelete, nil, "")
	if noteDeleted.Code != http.StatusNoContent || storage.hasPrefix(noteDeletePrefix) {
		t.Fatalf("note deletion status = %d, attachment objects remain = %v: %s",
			noteDeleted.Code, storage.hasPrefix(noteDeletePrefix), noteDeleted.Body.String())
	}

	noteForDataDelete := createAttachmentNote(t, api, ownerSession, "Delete user data cleanup")
	uploadAttachment(t, api, ownerSession, "/api/notes/"+noteForDataDelete+"/attachments", "user-data.txt", "cleanup")
	dataDeletePrefix := fmt.Sprintf("attachments/notes/%s/", ownerID)
	storage.putTestObject("profile-images/" + ownerID + "/avatar")
	otherUserObject := fmt.Sprintf("attachments/notes/%s/999/foreign--%s--%s",
		otherID, "YXBwbGljYXRpb24vb2N0ZXQtc3RyZWFt", "Zm9yZWlnbi50eHQ")
	storage.putTestObject(otherUserObject)
	dataDeleted := attachmentRequest(api, ownerSession, http.MethodDelete, "/api/user/data", nil, "")
	if dataDeleted.Code != http.StatusNoContent {
		t.Fatalf("user data deletion status = %d: %s", dataDeleted.Code, dataDeleted.Body.String())
	}
	if storage.hasPrefix(dataDeletePrefix) || storage.hasPrefix("profile-images/"+ownerID+"/") {
		t.Fatal("user-data deletion left private Note or profile objects behind")
	}
	if !storage.hasPrefix(fmt.Sprintf("attachments/notes/%s/", otherID)) {
		t.Fatal("user-data deletion removed another user's private objects")
	}
}

func TestTaskAttachmentsFollowWorkspaceAndTaskPermissionsIntegration(t *testing.T) {
	fixture := openCreditPolicyIntegrationFixture(t)
	storage := newAttachmentIntegrationStorage()
	const (
		ownerID     = "attachment-task-owner"
		memberID    = "attachment-task-member"
		outsiderID  = "attachment-task-outsider"
		workspaceID = "attachment-task-space"
	)
	ownerSession := fixture.createUserAndSession(t, ownerID, "attachment-task-owner@example.test", true)
	memberSession := fixture.createUserAndSession(t, memberID, "attachment-task-member@example.test", true)
	outsiderSession := fixture.createUserAndSession(t, outsiderID, "attachment-task-outsider@example.test", true)
	if err := fixture.store.CreateWorkspace(fixture.ctx, workspaceID, "Attachment Space", ownerID, []string{"workspace:manage"}); err != nil {
		t.Fatalf("create attachment Space: %v", err)
	}
	if err := fixture.store.SetWorkspaceMembership(fixture.ctx, workspaceID, memberID, "active", "member", []string{}); err != nil {
		t.Fatalf("add attachment Space member: %v", err)
	}

	api := attachmentIntegrationAPI(fixture, storage)
	taskResponse := attachmentRequest(api, ownerSession, http.MethodPost,
		"/api/v2/workspaces/"+workspaceID+"/tasks",
		strings.NewReader(`{"kind":"task","title":"Private attachment task","status":"open","priority":"normal"}`),
		"application/json")
	if taskResponse.Code != http.StatusCreated {
		t.Fatalf("create task status = %d: %s", taskResponse.Code, taskResponse.Body.String())
	}
	var task struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(taskResponse.Body).Decode(&task); err != nil || task.ID == "" {
		t.Fatalf("decode created task %#v: %v", task, err)
	}
	taskPath := "/api/v2/workspaces/" + workspaceID + "/tasks/" + task.ID + "/attachments"
	uploaded := uploadAttachment(t, api, ownerSession, taskPath, "task.txt", "task attachment")
	taskPrefix := fmt.Sprintf("attachments/tasks/%s/%s/", workspaceID, task.ID)

	for _, attempt := range []struct {
		name   string
		method string
		path   string
	}{
		{name: "list", method: http.MethodGet, path: taskPath},
		{name: "download", method: http.MethodGet, path: taskPath + "/" + uploaded.ID + "/download"},
		{name: "delete", method: http.MethodDelete, path: taskPath + "/" + uploaded.ID},
	} {
		t.Run("workspace outsider cannot "+attempt.name, func(t *testing.T) {
			response := attachmentRequest(api, outsiderSession, attempt.method, attempt.path, nil, "")
			if response.Code != http.StatusNotFound {
				t.Fatalf("%s status = %d, want %d: %s", attempt.name, response.Code, http.StatusNotFound, response.Body.String())
			}
			if !storage.hasPrefix(taskPrefix) {
				t.Fatalf("outsider %s request changed the task's private objects", attempt.name)
			}
		})
	}

	memberList := attachmentRequest(api, memberSession, http.MethodGet, taskPath, nil, "")
	if memberList.Code != http.StatusOK || !strings.Contains(memberList.Body.String(), uploaded.ID) {
		t.Fatalf("Space member list status/body = %d %s", memberList.Code, memberList.Body.String())
	}
	memberDownload := attachmentRequest(api, memberSession, http.MethodGet,
		taskPath+"/"+uploaded.ID+"/download", nil, "")
	if memberDownload.Code != http.StatusFound || !strings.Contains(memberDownload.Header().Get("Location"), "private-files.invalid") {
		t.Fatalf("Space member download status/headers = %d %#v", memberDownload.Code, memberDownload.Header())
	}
	memberDelete := attachmentRequest(api, memberSession, http.MethodDelete, taskPath+"/"+uploaded.ID, nil, "")
	if memberDelete.Code != http.StatusForbidden || !storage.hasPrefix(taskPrefix) {
		t.Fatalf("unauthorized member delete status = %d, object remains = %v: %s",
			memberDelete.Code, storage.hasPrefix(taskPrefix), memberDelete.Body.String())
	}

	ownerDelete := attachmentRequest(api, ownerSession, http.MethodDelete, taskPath+"/"+uploaded.ID, nil, "")
	if ownerDelete.Code != http.StatusNoContent || storage.hasPrefix(taskPrefix) {
		t.Fatalf("task owner delete status = %d, remaining objects = %v: %s",
			ownerDelete.Code, storage.hasPrefix(taskPrefix), ownerDelete.Body.String())
	}
}

func ensureAttachmentPersonalSpace(t *testing.T, fixture *creditPolicyIntegrationFixture, userID string) {
	t.Helper()
	if err := fixture.store.EnsurePersonalWorkspace(fixture.ctx, userID); err != nil {
		t.Fatalf("ensure personal Space: %v", err)
	}
	if err := fixture.store.SetWorkspaceMembership(
		fixture.ctx,
		postgres.DefaultWorkspaceID(userID),
		userID,
		"active",
		"owner",
		policy.PersonalWorkspaceCapabilities,
	); err != nil {
		t.Fatalf("set personal Space capabilities: %v", err)
	}
}

func attachmentIntegrationAPI(fixture *creditPolicyIntegrationFixture, storage *attachmentIntegrationStorage) http.Handler {
	handler := NewHandler(config.Config{
		AuthRateLimitHMACSecret: "test-auth-rate-limit-hmac-secret-32-bytes",
	}, fixture.store, slog.Default(), "askolo_session")
	handler.fileStorage = storage
	return handler.Routes()
}

func attachmentRequest(api http.Handler, sessionID, method, path string, body io.Reader, contentType string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, body)
	request.Header.Set("Authorization", "Bearer "+sessionID)
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	response := httptest.NewRecorder()
	api.ServeHTTP(response, request)
	return response
}

func createAttachmentNote(t *testing.T, api http.Handler, sessionID, title string) string {
	t.Helper()
	response := attachmentRequest(api, sessionID, http.MethodPost, "/api/notes",
		strings.NewReader(fmt.Sprintf(`{"title":%q}`, title)), "application/json")
	if response.Code != http.StatusCreated {
		t.Fatalf("create Note status = %d: %s", response.Code, response.Body.String())
	}
	var note struct {
		ID int64 `json:"id"`
	}
	if err := json.NewDecoder(response.Body).Decode(&note); err != nil || note.ID <= 0 {
		t.Fatalf("decode created Note %#v: %v", note, err)
	}
	return fmt.Sprint(note.ID)
}

type uploadedAttachment struct {
	ID string `json:"id"`
}

func uploadAttachment(t *testing.T, api http.Handler, sessionID, path, fileName, contents string) uploadedAttachment {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", fileName)
	if err != nil {
		t.Fatalf("create upload part: %v", err)
	}
	if _, err := io.WriteString(part, contents); err != nil {
		t.Fatalf("write upload part: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close upload form: %v", err)
	}
	response := attachmentRequest(api, sessionID, http.MethodPost, path, &body, writer.FormDataContentType())
	if response.Code != http.StatusCreated {
		t.Fatalf("upload status = %d: %s", response.Code, response.Body.String())
	}
	var result uploadedAttachment
	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read uploaded attachment response: %v", err)
	}
	if err := json.Unmarshal(responseBody, &result); err != nil || result.ID == "" {
		t.Fatalf("decode uploaded attachment %#v from %s: %v", result, responseBody, err)
	}
	return result
}
