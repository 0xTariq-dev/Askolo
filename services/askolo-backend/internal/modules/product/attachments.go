package product

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	"askolo/backend/internal/adapters/postgres"
	"askolo/backend/internal/platform/id"
)

const (
	maxAttachmentBytes = 8 << 20
	maxAttachments     = 20
)

type attachmentView struct {
	ID          string    `json:"id"`
	FileName    string    `json:"fileName"`
	ContentType string    `json:"contentType"`
	SizeBytes   int64     `json:"sizeBytes"`
	CreatedAt   time.Time `json:"createdAt"`
	DownloadURL string    `json:"downloadUrl"`
}

func (h *Handler) mountFileRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/user/profile-image", h.getProfileImage)
	mux.HandleFunc("POST /api/user/profile-image", h.uploadProfileImage)
	mux.HandleFunc("GET /api/notes/{noteId}/attachments", h.noteAttachments)
	mux.HandleFunc("POST /api/notes/{noteId}/attachments", h.noteAttachments)
	mux.HandleFunc("GET /api/notes/{noteId}/attachments/{attachmentId}/download", h.noteAttachmentDownload)
	mux.HandleFunc("DELETE /api/notes/{noteId}/attachments/{attachmentId}", h.deleteNoteAttachment)

	mux.HandleFunc("GET /api/v2/workspaces/{workspaceId}/tasks/{taskId}/attachments",
		h.v2Protect(false, h.listTaskAttachments))
	mux.HandleFunc("POST /api/v2/workspaces/{workspaceId}/tasks/{taskId}/attachments",
		h.v2Protect(true, h.uploadTaskAttachment))
	mux.HandleFunc("GET /api/v2/workspaces/{workspaceId}/tasks/{taskId}/attachments/{attachmentId}/download",
		h.v2Protect(false, h.downloadTaskAttachment))
	mux.HandleFunc("DELETE /api/v2/workspaces/{workspaceId}/tasks/{taskId}/attachments/{attachmentId}",
		h.v2Protect(true, h.deleteTaskAttachment))
}

func (h *Handler) uploadProfileImage(w http.ResponseWriter, r *http.Request) {
	userID, status := h.sessionUserID(r)
	if status != http.StatusOK {
		writeError(w, status, "UNAUTHORIZED", "Unauthorized.")
		return
	}
	if !h.attachmentStorage(w) {
		return
	}
	user, err := h.store.GetUser(r.Context(), userID)
	if err != nil {
		h.storeError(w, "profile image authorization failed", err)
		return
	}
	if user.AccountCreatedVia != "password" {
		writeError(w, http.StatusForbidden, "FORBIDDEN", "Profile photo uploads are not available for this account.")
		return
	}
	allowed, err := h.attachmentUploadLimit(r.Context(), userID)
	if err != nil {
		h.storeError(w, "profile image rate limit failed", err)
		return
	}
	if !allowed {
		w.Header().Set("Retry-After", "3600")
		writeError(w, http.StatusTooManyRequests, "RATE_LIMITED", "Too many file uploads. Try again later.")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, (5<<20)+(128<<10))
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_IMAGE", "Choose a PNG or JPEG image smaller than 5 MB.")
		return
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	file, header, err := r.FormFile("file")
	if err != nil || header.Size < 1 || header.Size > 5<<20 {
		writeError(w, http.StatusBadRequest, "INVALID_IMAGE", "Choose one PNG or JPEG image smaller than 5 MB.")
		return
	}
	defer file.Close()
	source, err := io.ReadAll(io.LimitReader(file, (5<<20)+1))
	if err != nil || len(source) == 0 || len(source) > 5<<20 {
		writeError(w, http.StatusBadRequest, "INVALID_IMAGE", "The selected image could not be read or is too large.")
		return
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(source))
	if err != nil || (format != "jpeg" && format != "png") ||
		config.Width < 1 || config.Height < 1 || config.Width > 4096 || config.Height > 4096 ||
		int64(config.Width)*int64(config.Height) > 16_777_216 {
		writeError(w, http.StatusBadRequest, "INVALID_IMAGE", "Use a valid PNG or JPEG image no larger than 4096 by 4096 pixels.")
		return
	}
	decoded, format, err := image.Decode(bytes.NewReader(source))
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_IMAGE", "The selected image could not be decoded.")
		return
	}
	var normalized bytes.Buffer
	contentType := "image/jpeg"
	if format == "png" {
		contentType = "image/png"
		err = png.Encode(&normalized, decoded)
	} else {
		err = jpeg.Encode(&normalized, decoded, &jpeg.Options{Quality: 88})
	}
	if err != nil || normalized.Len() == 0 || normalized.Len() > 8<<20 {
		writeError(w, http.StatusBadRequest, "INVALID_IMAGE", "The selected image could not be processed.")
		return
	}
	key := "profile-images/" + userID + "/avatar"
	if err := h.fileStorage.PutObject(r.Context(), key, contentType, normalized.Bytes()); err != nil {
		h.storeError(w, "profile image upload failed", err)
		return
	}
	imageURL := "/api/user/profile-image?version=" + strconv.FormatInt(time.Now().UnixNano(), 10)
	updated, err := h.store.UpdateUserProfileImage(r.Context(), userID, imageURL)
	if err != nil {
		h.storeError(w, "profile image update failed", err)
		return
	}
	writeJSON(w, http.StatusOK, toProfileUserResponse(updated))
}

func (h *Handler) getProfileImage(w http.ResponseWriter, r *http.Request) {
	userID, status := h.sessionUserID(r)
	if status != http.StatusOK {
		writeError(w, status, "UNAUTHORIZED", "Unauthorized.")
		return
	}
	if !h.attachmentStorage(w) {
		return
	}
	user, err := h.store.GetUser(r.Context(), userID)
	if err != nil {
		h.storeError(w, "profile image lookup failed", err)
		return
	}
	if !strings.HasPrefix(user.ProfileImageURL, "/api/user/profile-image") {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "Profile image not found.")
		return
	}
	if !h.allowAttachmentRead(w, r, userID) {
		return
	}
	request, err := h.fileStorage.PresignGetInline(r.Context(), "profile-images/"+userID+"/avatar", 5*time.Minute)
	if err != nil {
		h.storeError(w, "profile image download preparation failed", err)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	http.Redirect(w, r, request.URL, http.StatusFound)
}

func (h *Handler) attachmentStorage(w http.ResponseWriter) bool {
	w.Header().Set("Cache-Control", "private, no-store")
	if h.fileStorage != nil {
		return true
	}
	writeError(w, http.StatusServiceUnavailable, "FILE_STORAGE_UNAVAILABLE", "File storage is not available.")
	return false
}

func (h *Handler) attachmentUploadLimit(ctx context.Context, userID string) (bool, error) {
	if len([]byte(strings.TrimSpace(h.authRateLimitSecret))) < 32 {
		return false, fmt.Errorf("file upload rate limit is unavailable")
	}
	mac := hmac.New(sha256.New, []byte(h.authRateLimitSecret))
	_, _ = mac.Write([]byte("askolo:file-upload:v1\x00" + userID))
	return h.store.AllowAuthRateLimitBucket(ctx, hex.EncodeToString(mac.Sum(nil)), 40, time.Hour)
}

func (h *Handler) attachmentReadLimit(ctx context.Context, userID string) (bool, error) {
	if len([]byte(strings.TrimSpace(h.authRateLimitSecret))) < 32 {
		return false, fmt.Errorf("file read rate limit is unavailable")
	}
	mac := hmac.New(sha256.New, []byte(h.authRateLimitSecret))
	_, _ = mac.Write([]byte("askolo:file-read:v1\x00" + userID))
	return h.store.AllowAuthRateLimitBucket(ctx, hex.EncodeToString(mac.Sum(nil)), 120, time.Minute)
}

func (h *Handler) allowAttachmentRead(w http.ResponseWriter, r *http.Request, userID string) bool {
	allowed, err := h.attachmentReadLimit(r.Context(), userID)
	if err != nil {
		h.storeError(w, "file read rate limit failed", err)
		return false
	}
	if !allowed {
		w.Header().Set("Retry-After", "60")
		writeError(w, http.StatusTooManyRequests, "RATE_LIMITED", "Too many file requests. Try again shortly.")
		return false
	}
	return true
}

func (h *Handler) noteAttachments(w http.ResponseWriter, r *http.Request) {
	userID, status := h.sessionUserID(r)
	if status != http.StatusOK {
		writeError(w, status, "UNAUTHORIZED", "Unauthorized.")
		return
	}
	noteID, err := strconv.ParseInt(r.PathValue("noteId"), 10, 64)
	if err != nil || noteID <= 0 {
		writeError(w, http.StatusBadRequest, "INVALID_NOTE_ID", "A valid note ID is required.")
		return
	}
	if _, err := h.store.GetProduct(r.Context(), notes, userID, noteID); err != nil {
		if err == postgres.ErrNotFound {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "Note not found.")
		} else {
			h.storeError(w, "note attachment authorization failed", err)
		}
		return
	}
	if !h.attachmentStorage(w) {
		return
	}
	prefix := fmt.Sprintf("attachments/notes/%s/%d/", userID, noteID)
	switch r.Method {
	case http.MethodGet:
		if !h.allowAttachmentRead(w, r, userID) {
			return
		}
		h.listAttachments(w, r, prefix)
	case http.MethodPost:
		h.uploadAttachment(w, r, userID, prefix)
	default:
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed.")
	}
}

func (h *Handler) listTaskAttachments(w http.ResponseWriter, r *http.Request, userID, role string) {
	if !h.authorizeTaskAttachmentAccess(w, r, userID, role, false) || !h.attachmentStorage(w) {
		return
	}
	h.listAttachments(w, r, taskAttachmentPrefix(r))
}

func (h *Handler) uploadTaskAttachment(w http.ResponseWriter, r *http.Request, userID, role string) {
	if !h.authorizeTaskAttachmentAccess(w, r, userID, role, true) || !h.attachmentStorage(w) {
		return
	}
	h.uploadAttachment(w, r, userID, taskAttachmentPrefix(r))
}

func (h *Handler) authorizeTaskAttachmentAccess(w http.ResponseWriter, r *http.Request, userID, role string, write bool) bool {
	taskID, workspaceID := r.PathValue("taskId"), r.PathValue("workspaceId")
	if !validV2Identifier(taskID) {
		writeError(w, http.StatusBadRequest, "INVALID_TASK_ID", "A valid task ID is required.")
		return false
	}
	task, err := h.store.GetV2Task(r.Context(), workspaceID, taskID)
	if err != nil {
		v2HandleStoreError(w, r, h, userID, "read task for file access", err)
		return false
	}
	if !write {
		return true
	}
	canUpdate := task.CreatedByUserID == userID || role == "owner" || role == "admin"
	if !canUpdate && task.AssignedToGroupID != nil {
		canUpdate, err = h.store.IsActiveV2GroupMember(r.Context(), workspaceID, *task.AssignedToGroupID, userID)
	}
	if err == nil && !canUpdate {
		canUpdate, err = h.store.V2WorkspaceHasPermission(r.Context(), workspaceID, userID, "resource.update.any")
	}
	if err != nil {
		h.v2InternalError(w, r, userID, "check task file access", err)
		return false
	}
	if !canUpdate {
		writeError(w, http.StatusForbidden, "FORBIDDEN", "You are not allowed to change this task.")
		return false
	}
	return true
}

func taskAttachmentPrefix(r *http.Request) string {
	return fmt.Sprintf("attachments/tasks/%s/%s/", r.PathValue("workspaceId"), r.PathValue("taskId"))
}

func (h *Handler) listAttachments(w http.ResponseWriter, r *http.Request, prefix string) {
	objects, err := h.fileStorage.ListObjects(r.Context(), prefix, maxAttachments+1)
	if err != nil {
		h.storeError(w, "attachment list failed", err)
		return
	}
	if len(objects) > maxAttachments {
		objects = objects[:maxAttachments]
	}
	result := make([]attachmentView, 0, len(objects))
	for _, object := range objects {
		view, ok := attachmentFromObject(object.Key, object.Size, object.LastModified, prefix)
		if ok {
			view.DownloadURL = r.URL.Path + "/" + view.ID + "/download"
			result = append(result, view)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"attachments": result})
}

func (h *Handler) deleteAttachmentPrefix(ctx context.Context, prefix string) error {
	objects, err := h.fileStorage.ListObjects(ctx, prefix, 1000)
	if err != nil {
		return err
	}
	if len(objects) == 1000 {
		return fmt.Errorf("attachment cleanup limit exceeded")
	}
	for _, object := range objects {
		if err := h.fileStorage.Delete(ctx, object.Key); err != nil {
			return err
		}
	}
	return nil
}

func (h *Handler) clearUserOwnedFiles(ctx context.Context, userID string) error {
	if h.fileStorage == nil {
		return nil
	}
	userNotes, err := h.store.ListProduct(ctx, notes, userID, nil)
	if err != nil {
		return err
	}
	for _, note := range userNotes {
		noteID, err := strconv.ParseInt(fmt.Sprint(note["id"]), 10, 64)
		if err != nil || noteID <= 0 {
			return fmt.Errorf("invalid note ID during file cleanup")
		}
		if err := h.deleteAttachmentPrefix(ctx, fmt.Sprintf("attachments/notes/%s/%d/", userID, noteID)); err != nil {
			return err
		}
	}
	return h.fileStorage.Delete(ctx, "profile-images/"+userID+"/avatar")
}

func (h *Handler) uploadAttachment(w http.ResponseWriter, r *http.Request, userID, prefix string) {
	allowed, err := h.attachmentUploadLimit(r.Context(), userID)
	if err != nil {
		h.storeError(w, "file upload rate limit failed", err)
		return
	}
	if !allowed {
		w.Header().Set("Retry-After", "3600")
		writeError(w, http.StatusTooManyRequests, "RATE_LIMITED", "Too many file uploads. Try again later.")
		return
	}
	objects, err := h.fileStorage.ListObjects(r.Context(), prefix, maxAttachments+1)
	if err != nil {
		h.storeError(w, "attachment capacity check failed", err)
		return
	}
	if len(objects) >= maxAttachments {
		writeError(w, http.StatusBadRequest, "ATTACHMENT_LIMIT", "This item already has the maximum number of attachments.")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxAttachmentBytes+(256<<10))
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_UPLOAD", "Choose a file smaller than 8 MB.")
		return
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	file, header, err := r.FormFile("file")
	if err != nil || header.Size < 1 || header.Size > maxAttachmentBytes {
		writeError(w, http.StatusBadRequest, "INVALID_UPLOAD", "Choose one file smaller than 8 MB.")
		return
	}
	defer file.Close()
	content, err := io.ReadAll(io.LimitReader(file, maxAttachmentBytes+1))
	if err != nil || len(content) == 0 || len(content) > maxAttachmentBytes {
		writeError(w, http.StatusBadRequest, "INVALID_UPLOAD", "The selected file could not be read or is too large.")
		return
	}
	fileName := safeAttachmentName(header.Filename)
	if fileName == "" {
		writeError(w, http.StatusBadRequest, "INVALID_FILENAME", "The selected filename is not valid.")
		return
	}
	contentType := "application/octet-stream"
	encodedType := base64.RawURLEncoding.EncodeToString([]byte(contentType))
	encodedName := base64.RawURLEncoding.EncodeToString([]byte(fileName))
	fileID, err := id.New()
	if err != nil {
		h.storeError(w, "attachment identifier creation failed", err)
		return
	}
	key := prefix + fileID + "--" + encodedType + "--" + encodedName
	if err := h.fileStorage.PutObject(r.Context(), key, contentType, content); err != nil {
		h.storeError(w, "attachment upload failed", err)
		return
	}
	view, _ := attachmentFromObject(key, int64(len(content)), time.Now().UTC(), prefix)
	view.DownloadURL = r.URL.Path + "/" + view.ID + "/download"
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, http.StatusCreated, view)
}

func safeAttachmentName(value string) string {
	value = path.Base(strings.ReplaceAll(strings.TrimSpace(value), "\\", "/"))
	if value == "." || value == ".." || value == "/" || len(value) > 240 {
		return ""
	}
	for _, character := range value {
		if character < 0x20 || character == 0x7f {
			return ""
		}
	}
	return value
}

func attachmentFromObject(key string, size int64, modified time.Time, prefix string) (attachmentView, bool) {
	if !strings.HasPrefix(key, prefix) {
		return attachmentView{}, false
	}
	encoded := strings.TrimPrefix(key, prefix)
	encodedContentType := base64.RawURLEncoding.EncodeToString([]byte("application/octet-stream"))
	typeSeparator := encodedContentType + "--"
	for delimiter := strings.Index(encoded, "--"); delimiter >= 0; {
		attachmentID := encoded[:delimiter]
		encodedFile := encoded[delimiter+2:]
		if attachmentID != "" && validV2Identifier(attachmentID) && strings.HasPrefix(encodedFile, typeSeparator) {
			fileNameBytes, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(encodedFile, typeSeparator))
			if err == nil {
				fileName := string(fileNameBytes)
				if fileName != "" && safeAttachmentName(fileName) == fileName {
					return attachmentView{
						ID: attachmentID, FileName: fileName, ContentType: "application/octet-stream",
						SizeBytes: size, CreatedAt: modified.UTC(),
					}, true
				}
			}
		}
		next := strings.Index(encoded[delimiter+1:], "--")
		if next < 0 {
			break
		}
		delimiter += next + 1
	}
	return attachmentView{}, false
}

func (h *Handler) noteAttachmentDownload(w http.ResponseWriter, r *http.Request) {
	h.noteAttachmentAction(w, r, false)
}

func (h *Handler) deleteNoteAttachment(w http.ResponseWriter, r *http.Request) {
	h.noteAttachmentAction(w, r, true)
}

func (h *Handler) noteAttachmentAction(w http.ResponseWriter, r *http.Request, remove bool) {
	userID, status := h.sessionUserID(r)
	if status != http.StatusOK {
		writeError(w, status, "UNAUTHORIZED", "Unauthorized.")
		return
	}
	noteID, err := strconv.ParseInt(r.PathValue("noteId"), 10, 64)
	if err != nil || noteID <= 0 {
		writeError(w, http.StatusBadRequest, "INVALID_NOTE_ID", "A valid note ID is required.")
		return
	}
	if _, err := h.store.GetProduct(r.Context(), notes, userID, noteID); err != nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "Note not found.")
		return
	}
	if !h.attachmentStorage(w) {
		return
	}
	if !h.allowAttachmentRead(w, r, userID) {
		return
	}
	prefix := fmt.Sprintf("attachments/notes/%s/%d/", userID, noteID)
	h.attachmentAction(w, r, prefix, remove)
}

func (h *Handler) downloadTaskAttachment(w http.ResponseWriter, r *http.Request, userID, role string) {
	if !h.authorizeTaskAttachmentAccess(w, r, userID, role, false) || !h.attachmentStorage(w) {
		return
	}
	h.attachmentAction(w, r, taskAttachmentPrefix(r), false)
}

func (h *Handler) deleteTaskAttachment(w http.ResponseWriter, r *http.Request, userID, role string) {
	if !h.authorizeTaskAttachmentAccess(w, r, userID, role, true) || !h.attachmentStorage(w) {
		return
	}
	h.attachmentAction(w, r, taskAttachmentPrefix(r), true)
}

func (h *Handler) attachmentAction(w http.ResponseWriter, r *http.Request, prefix string, remove bool) {
	fileID := r.PathValue("attachmentId")
	if fileID == "" || strings.ContainsAny(fileID, "/\\") {
		writeError(w, http.StatusBadRequest, "INVALID_ATTACHMENT_ID", "A valid attachment ID is required.")
		return
	}
	objects, err := h.fileStorage.ListObjects(r.Context(), prefix, maxAttachments+1)
	if err != nil {
		h.storeError(w, "attachment lookup failed", err)
		return
	}
	for _, object := range objects {
		view, ok := attachmentFromObject(object.Key, object.Size, object.LastModified, prefix)
		if !ok || view.ID != fileID {
			continue
		}
		if remove {
			if err := h.fileStorage.Delete(r.Context(), object.Key); err != nil {
				h.storeError(w, "attachment deletion failed", err)
				return
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}
		request, err := h.fileStorage.PresignGetAttachment(r.Context(), object.Key, 5*time.Minute, view.FileName)
		if err != nil {
			h.storeError(w, "attachment download preparation failed", err)
			return
		}
		w.Header().Set("Cache-Control", "private, no-store")
		http.Redirect(w, r, request.URL, http.StatusFound)
		return
	}
	writeError(w, http.StatusNotFound, "NOT_FOUND", "Attachment not found.")
}
