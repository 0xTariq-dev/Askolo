package product

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"askolo/backend/internal/adapters/postgres"
)

type v2GoalRequest struct {
	Title           string  `json:"title"`
	Description     *string `json:"description"`
	TargetOn        *string `json:"targetOn"`
	Status          string  `json:"status"`
	ProgressPercent int     `json:"progressPercent"`
	Category        *string `json:"category"`
}

type v2GoalPatchRequest struct {
	Title           v2PatchField[string] `json:"title"`
	Description     v2PatchField[string] `json:"description"`
	TargetOn        v2PatchField[string] `json:"targetOn"`
	Status          v2PatchField[string] `json:"status"`
	ProgressPercent v2PatchField[int]    `json:"progressPercent"`
	Category        v2PatchField[string] `json:"category"`
}

type v2PlanRequest struct {
	Title      string  `json:"title"`
	Details    *string `json:"details"`
	StartsOn   string  `json:"startsOn"`
	EndsOn     string  `json:"endsOn"`
	TimeZone   string  `json:"timeZone"`
	Visibility string  `json:"visibility"`
	Status     string  `json:"status"`
}

type v2PlanPatchRequest struct {
	Title      v2PatchField[string] `json:"title"`
	Details    v2PatchField[string] `json:"details"`
	StartsOn   v2PatchField[string] `json:"startsOn"`
	EndsOn     v2PatchField[string] `json:"endsOn"`
	TimeZone   v2PatchField[string] `json:"timeZone"`
	Visibility v2PatchField[string] `json:"visibility"`
	Status     v2PatchField[string] `json:"status"`
}

type v2PlanEntryRequest struct {
	TaskID         string     `json:"taskId"`
	OccurrenceOn   *string    `json:"occurrenceOn"`
	PlannedOn      string     `json:"plannedOn"`
	PlannedStartAt *time.Time `json:"plannedStartAt"`
	PlannedEndAt   *time.Time `json:"plannedEndAt"`
	Position       int        `json:"position"`
	AssignedToID   *string    `json:"assignedToUserId"`
}

type v2PlanEntryPatchRequest struct {
	OccurrenceOn   v2PatchField[string]    `json:"occurrenceOn"`
	PlannedOn      v2PatchField[string]    `json:"plannedOn"`
	PlannedStartAt v2PatchField[time.Time] `json:"plannedStartAt"`
	PlannedEndAt   v2PatchField[time.Time] `json:"plannedEndAt"`
	Position       v2PatchField[int]       `json:"position"`
	AssignedToID   v2PatchField[string]    `json:"assignedToUserId"`
}

type v2ReminderRequest struct {
	Kind   string          `json:"kind"`
	Rule   json.RawMessage `json:"rule"`
	Status string          `json:"status"`
}

type v2ReminderPatchRequest struct {
	Kind   v2PatchField[string]          `json:"kind"`
	Rule   v2PatchField[json.RawMessage] `json:"rule"`
	Status v2PatchField[string]          `json:"status"`
}

func (h *Handler) v2ListGoals(w http.ResponseWriter, r *http.Request, userID, _ string) {
	if !v2QueryValid(r, "limit", "offset") {
		writeError(w, http.StatusBadRequest, "INVALID_QUERY", "The request contains an unsupported or repeated query parameter.")
		return
	}
	limit, offset, ok := v2Page(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "INVALID_PAGINATION", "Use a limit from 1 to 100 and a non-negative offset.")
		return
	}
	goals, err := h.store.ListV2Goals(r.Context(), r.PathValue("workspaceId"), limit, offset)
	if err != nil {
		v2HandleStoreError(w, r, h, userID, "list goals", err)
		return
	}
	writeJSON(w, http.StatusOK, goals)
}

func (h *Handler) v2CreateGoal(w http.ResponseWriter, r *http.Request, userID, _ string) {
	var request v2GoalRequest
	if !decodeV2JSON(w, r, &request) {
		return
	}
	input, err := buildV2GoalWrite(request)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_GOAL", err.Error())
		return
	}
	goal, err := h.store.CreateV2Goal(r.Context(), r.PathValue("workspaceId"), userID, input)
	if err != nil {
		v2HandleStoreError(w, r, h, userID, "create goal", err)
		return
	}
	writeJSON(w, http.StatusCreated, goal)
}

func (h *Handler) v2GetGoal(w http.ResponseWriter, r *http.Request, userID, _ string) {
	goal, err := h.store.GetV2Goal(r.Context(), r.PathValue("workspaceId"), r.PathValue("goalId"))
	if err != nil {
		v2HandleStoreError(w, r, h, userID, "read goal", err)
		return
	}
	writeJSON(w, http.StatusOK, goal)
}

func (h *Handler) v2UpdateGoal(w http.ResponseWriter, r *http.Request, userID, role string) {
	workspaceID, goalID := r.PathValue("workspaceId"), r.PathValue("goalId")
	current, err := h.store.GetV2Goal(r.Context(), workspaceID, goalID)
	if err != nil {
		v2HandleStoreError(w, r, h, userID, "read goal before update", err)
		return
	}
	if current.CreatedByUserID != userID && role != "owner" && role != "admin" {
		writeError(w, http.StatusForbidden, "FORBIDDEN", "You are not allowed to change this goal.")
		return
	}
	var patch v2GoalPatchRequest
	if !decodeV2JSON(w, r, &patch) {
		return
	}
	input, err := mergeV2GoalPatch(current, patch)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_GOAL", err.Error())
		return
	}
	goal, err := h.store.UpdateV2Goal(r.Context(), workspaceID, goalID, input)
	if err != nil {
		v2HandleStoreError(w, r, h, userID, "update goal", err)
		return
	}
	writeJSON(w, http.StatusOK, goal)
}

func (h *Handler) v2ArchiveGoal(w http.ResponseWriter, r *http.Request, userID, role string) {
	workspaceID, goalID := r.PathValue("workspaceId"), r.PathValue("goalId")
	goal, err := h.store.GetV2Goal(r.Context(), workspaceID, goalID)
	if err != nil {
		v2HandleStoreError(w, r, h, userID, "read goal before archive", err)
		return
	}
	if goal.CreatedByUserID != userID && role != "owner" && role != "admin" {
		writeError(w, http.StatusForbidden, "FORBIDDEN", "You are not allowed to change this goal.")
		return
	}
	if err := h.store.ArchiveV2Goal(r.Context(), workspaceID, goalID); err != nil {
		v2HandleStoreError(w, r, h, userID, "archive goal", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func buildV2GoalWrite(request v2GoalRequest) (postgres.V2GoalWrite, error) {
	if request.ProgressPercent < 0 || request.ProgressPercent > 100 {
		return postgres.V2GoalWrite{}, errors.New("progressPercent must be between 0 and 100")
	}
	input := postgres.V2GoalWrite{
		Title:           strings.TrimSpace(request.Title),
		Description:     request.Description,
		Status:          request.Status,
		ProgressPercent: int16(request.ProgressPercent),
		Category:        request.Category,
	}
	if request.Status == "" {
		input.Status = "active"
	}
	if request.TargetOn != nil {
		target, ok := parseV2Date(*request.TargetOn)
		if !ok {
			return input, errors.New("targetOn must be a calendar date in YYYY-MM-DD format")
		}
		input.TargetOn = &target
	}
	return validateV2GoalWrite(input)
}

func mergeV2GoalPatch(current postgres.V2GoalRecord, patch v2GoalPatchRequest) (postgres.V2GoalWrite, error) {
	input := postgres.V2GoalWrite{
		Title:           current.Title,
		Description:     current.Description,
		TargetOn:        current.TargetOn,
		Status:          current.Status,
		ProgressPercent: current.ProgressPercent,
		Category:        current.Category,
	}
	if patch.Title.Set {
		if patch.Title.Null {
			return input, errors.New("title cannot be null")
		}
		input.Title = strings.TrimSpace(patch.Title.Value)
	}
	if patch.Description.Set {
		input.Description = nil
		if !patch.Description.Null {
			input.Description = &patch.Description.Value
		}
	}
	if patch.TargetOn.Set {
		input.TargetOn = nil
		if !patch.TargetOn.Null {
			target, ok := parseV2Date(patch.TargetOn.Value)
			if !ok {
				return input, errors.New("targetOn must be a calendar date in YYYY-MM-DD format")
			}
			input.TargetOn = &target
		}
	}
	if patch.Status.Set {
		if patch.Status.Null {
			return input, errors.New("status cannot be null")
		}
		input.Status = patch.Status.Value
	}
	if patch.ProgressPercent.Set {
		if patch.ProgressPercent.Null {
			return input, errors.New("progressPercent cannot be null")
		}
		if patch.ProgressPercent.Value < 0 || patch.ProgressPercent.Value > 100 {
			return input, errors.New("progressPercent must be between 0 and 100")
		}
		input.ProgressPercent = int16(patch.ProgressPercent.Value)
	}
	if patch.Category.Set {
		input.Category = nil
		if !patch.Category.Null {
			input.Category = &patch.Category.Value
		}
	}
	return validateV2GoalWrite(input)
}

func validateV2GoalWrite(input postgres.V2GoalWrite) (postgres.V2GoalWrite, error) {
	input.Title = strings.TrimSpace(input.Title)
	if len(input.Title) == 0 || len([]rune(input.Title)) > 200 {
		return input, errors.New("title must contain between 1 and 200 characters")
	}
	if input.Description != nil && len([]rune(*input.Description)) > 2000 {
		return input, errors.New("description cannot exceed 2000 characters")
	}
	if !oneOf(input.Status, "active", "paused", "completed", "cancelled", "archived") {
		return input, errors.New("goal status is invalid")
	}
	if input.ProgressPercent < 0 || input.ProgressPercent > 100 {
		return input, errors.New("progressPercent must be between 0 and 100")
	}
	if input.Category != nil && len([]rune(*input.Category)) > 100 {
		return input, errors.New("category cannot exceed 100 characters")
	}
	return input, nil
}

func (h *Handler) v2ListPlans(w http.ResponseWriter, r *http.Request, userID, role string) {
	if !v2QueryValid(r, "limit", "offset") {
		writeError(w, http.StatusBadRequest, "INVALID_QUERY", "The request contains an unsupported or repeated query parameter.")
		return
	}
	limit, offset, ok := v2Page(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "INVALID_PAGINATION", "Use a limit from 1 to 100 and a non-negative offset.")
		return
	}
	plans, err := h.store.ListV2Plans(r.Context(), r.PathValue("workspaceId"), userID, role, limit, offset)
	if err != nil {
		v2HandleStoreError(w, r, h, userID, "list plans", err)
		return
	}
	writeJSON(w, http.StatusOK, plans)
}

func (h *Handler) v2CreatePlan(w http.ResponseWriter, r *http.Request, userID, role string) {
	var request v2PlanRequest
	if !decodeV2JSON(w, r, &request) {
		return
	}
	input, err := buildV2PlanWrite(request)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_PLAN", err.Error())
		return
	}
	if input.Visibility == "shared" && role != "owner" && role != "admin" {
		writeError(w, http.StatusForbidden, "FORBIDDEN", "Only workspace owners and admins can share plans.")
		return
	}
	plan, err := h.store.CreateV2Plan(r.Context(), r.PathValue("workspaceId"), userID, input)
	if err != nil {
		v2HandleStoreError(w, r, h, userID, "create plan", err)
		return
	}
	writeJSON(w, http.StatusCreated, plan)
}

func (h *Handler) v2GetPlan(w http.ResponseWriter, r *http.Request, userID, role string) {
	plan, err := h.store.GetV2Plan(r.Context(), r.PathValue("workspaceId"), r.PathValue("planId"), userID, role)
	if err != nil {
		v2HandleStoreError(w, r, h, userID, "read plan", err)
		return
	}
	writeJSON(w, http.StatusOK, plan)
}

func (h *Handler) v2UpdatePlan(w http.ResponseWriter, r *http.Request, userID, role string) {
	workspaceID, planID := r.PathValue("workspaceId"), r.PathValue("planId")
	current, err := h.store.GetV2Plan(r.Context(), workspaceID, planID, userID, role)
	if err != nil {
		v2HandleStoreError(w, r, h, userID, "read plan before update", err)
		return
	}
	var patch v2PlanPatchRequest
	if !decodeV2JSON(w, r, &patch) {
		return
	}
	input, err := mergeV2PlanPatch(current, patch)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_PLAN", err.Error())
		return
	}
	if input.Visibility != current.Visibility && role != "owner" && role != "admin" {
		writeError(w, http.StatusForbidden, "FORBIDDEN", "Only workspace owners and admins can change plan visibility.")
		return
	}
	plan, err := h.store.UpdateV2Plan(r.Context(), workspaceID, planID, userID, role, input)
	if err != nil {
		v2HandleStoreError(w, r, h, userID, "update plan", err)
		return
	}
	writeJSON(w, http.StatusOK, plan)
}

func (h *Handler) v2ArchivePlan(w http.ResponseWriter, r *http.Request, userID, role string) {
	if err := h.store.ArchiveV2Plan(r.Context(), r.PathValue("workspaceId"), r.PathValue("planId"), userID, role); err != nil {
		v2HandleStoreError(w, r, h, userID, "archive plan", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func buildV2PlanWrite(request v2PlanRequest) (postgres.V2PlanWrite, error) {
	startsOn, startOK := parseV2Date(request.StartsOn)
	endsOn, endOK := parseV2Date(request.EndsOn)
	input := postgres.V2PlanWrite{
		Title:      strings.TrimSpace(request.Title),
		Details:    request.Details,
		StartsOn:   startsOn,
		EndsOn:     endsOn,
		TimeZone:   strings.TrimSpace(request.TimeZone),
		Visibility: request.Visibility,
		Status:     request.Status,
	}
	if input.Visibility == "" {
		input.Visibility = "personal"
	}
	if input.Status == "" {
		input.Status = "active"
	}
	if err := validateV2PlanWrite(input, startOK, endOK); err != nil {
		return input, err
	}
	return input, nil
}

func mergeV2PlanPatch(current postgres.V2Plan, patch v2PlanPatchRequest) (postgres.V2PlanWrite, error) {
	input := postgres.V2PlanWrite{
		Title:      current.Title,
		Details:    current.Details,
		StartsOn:   current.StartsOn,
		EndsOn:     current.EndsOn,
		TimeZone:   current.TimeZone,
		Visibility: current.Visibility,
		Status:     current.Status,
	}
	startOK, endOK := true, true
	if patch.Title.Set {
		if patch.Title.Null {
			return input, errors.New("title cannot be null")
		}
		input.Title = strings.TrimSpace(patch.Title.Value)
	}
	if patch.Details.Set {
		input.Details = nil
		if !patch.Details.Null {
			input.Details = &patch.Details.Value
		}
	}
	if patch.StartsOn.Set {
		startOK = false
		if !patch.StartsOn.Null {
			value, ok := parseV2Date(patch.StartsOn.Value)
			if !ok {
				return input, errors.New("startsOn must be a calendar date in YYYY-MM-DD format")
			}
			input.StartsOn, startOK = value, true
		} else {
			return input, errors.New("startsOn cannot be null")
		}
	}
	if patch.EndsOn.Set {
		endOK = false
		if !patch.EndsOn.Null {
			value, ok := parseV2Date(patch.EndsOn.Value)
			if !ok {
				return input, errors.New("endsOn must be a calendar date in YYYY-MM-DD format")
			}
			input.EndsOn, endOK = value, true
		} else {
			return input, errors.New("endsOn cannot be null")
		}
	}
	if patch.TimeZone.Set {
		if patch.TimeZone.Null {
			return input, errors.New("timeZone cannot be null")
		}
		input.TimeZone = strings.TrimSpace(patch.TimeZone.Value)
	}
	if patch.Visibility.Set {
		if patch.Visibility.Null {
			return input, errors.New("visibility cannot be null")
		}
		input.Visibility = patch.Visibility.Value
	}
	if patch.Status.Set {
		if patch.Status.Null {
			return input, errors.New("status cannot be null")
		}
		input.Status = patch.Status.Value
	}
	if err := validateV2PlanWrite(input, startOK, endOK); err != nil {
		return input, err
	}
	return input, nil
}

func validateV2PlanWrite(input postgres.V2PlanWrite, startOK, endOK bool) error {
	if len(input.Title) == 0 || len([]rune(input.Title)) > 200 {
		return errors.New("title must contain between 1 and 200 characters")
	}
	if input.Details != nil && len([]rune(*input.Details)) > 4000 {
		return errors.New("details cannot exceed 4000 characters")
	}
	if !startOK || !endOK || input.EndsOn.Before(input.StartsOn) {
		return errors.New("endsOn must be on or after startsOn")
	}
	if input.EndsOn.Sub(input.StartsOn) > 366*24*time.Hour {
		return errors.New("a plan cannot exceed 367 calendar days")
	}
	if input.TimeZone == "" || len(input.TimeZone) > 64 {
		return errors.New("timeZone must be a valid IANA time zone")
	}
	if _, err := time.LoadLocation(input.TimeZone); err != nil {
		return errors.New("timeZone must be a valid IANA time zone")
	}
	if !oneOf(input.Visibility, "personal", "shared") {
		return errors.New("visibility must be personal or shared")
	}
	if !oneOf(input.Status, "active", "completed", "archived") {
		return errors.New("plan status is invalid")
	}
	return nil
}

func (h *Handler) v2CreatePlanEntry(w http.ResponseWriter, r *http.Request, userID, role string) {
	workspaceID, planID := r.PathValue("workspaceId"), r.PathValue("planId")
	plan, err := h.store.GetV2Plan(r.Context(), workspaceID, planID, userID, role)
	if err != nil {
		v2HandleStoreError(w, r, h, userID, "read plan before entry create", err)
		return
	}
	var request v2PlanEntryRequest
	if !decodeV2JSON(w, r, &request) {
		return
	}
	input, err := buildV2PlanEntryWrite(request)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_PLAN_ENTRY", err.Error())
		return
	}
	if err := h.validateV2PlanEntryTask(r, workspaceID, input.TaskID, input.OccurrenceOn); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_PLAN_ENTRY", "The task occurrence does not match the selected task.")
		return
	}
	if !v2DateInRange(input.PlannedOn, plan.StartsOn, plan.EndsOn) {
		writeError(w, http.StatusBadRequest, "INVALID_RANGE", "The planned date must fall within the plan period.")
		return
	}
	entry, err := h.store.CreateV2PlanEntry(r.Context(), workspaceID, planID, userID, role, input)
	if err != nil {
		v2HandleStoreError(w, r, h, userID, "create plan entry", err)
		return
	}
	writeJSON(w, http.StatusCreated, entry)
}

func (h *Handler) v2UpdatePlanEntry(w http.ResponseWriter, r *http.Request, userID, role string) {
	workspaceID, planID, entryID := r.PathValue("workspaceId"), r.PathValue("planId"), r.PathValue("entryId")
	plan, err := h.store.GetV2Plan(r.Context(), workspaceID, planID, userID, role)
	if err != nil {
		v2HandleStoreError(w, r, h, userID, "read plan before entry update", err)
		return
	}
	current, err := h.store.GetV2PlanEntry(r.Context(), workspaceID, planID, entryID)
	if err != nil {
		v2HandleStoreError(w, r, h, userID, "read plan entry before update", err)
		return
	}
	var patch v2PlanEntryPatchRequest
	if !decodeV2JSON(w, r, &patch) {
		return
	}
	input := postgres.V2PlanEntryWrite{
		TaskID:           current.TaskID,
		OccurrenceOn:     current.OccurrenceOn,
		PlannedOn:        current.PlannedOn,
		PlannedStartAt:   current.PlannedStartAt,
		PlannedEndAt:     current.PlannedEndAt,
		Position:         current.Position,
		AssignedToUserID: current.AssignedToUserID,
	}
	assignChanged := patch.AssignedToID.Set
	if patch.OccurrenceOn.Set {
		input.OccurrenceOn = nil
		if !patch.OccurrenceOn.Null {
			value, ok := parseV2Date(patch.OccurrenceOn.Value)
			if !ok {
				writeError(w, http.StatusBadRequest, "INVALID_PLAN_ENTRY", "occurrenceOn must use YYYY-MM-DD.")
				return
			}
			input.OccurrenceOn = &value
		}
	}
	if patch.PlannedOn.Set {
		if patch.PlannedOn.Null {
			writeError(w, http.StatusBadRequest, "INVALID_PLAN_ENTRY", "plannedOn cannot be null.")
			return
		}
		value, ok := parseV2Date(patch.PlannedOn.Value)
		if !ok {
			writeError(w, http.StatusBadRequest, "INVALID_PLAN_ENTRY", "plannedOn must use YYYY-MM-DD.")
			return
		}
		input.PlannedOn = value
	}
	if patch.PlannedStartAt.Set {
		input.PlannedStartAt = nil
		if !patch.PlannedStartAt.Null {
			value := patch.PlannedStartAt.Value
			input.PlannedStartAt = &value
		}
	}
	if patch.PlannedEndAt.Set {
		input.PlannedEndAt = nil
		if !patch.PlannedEndAt.Null {
			value := patch.PlannedEndAt.Value
			input.PlannedEndAt = &value
		}
	}
	if patch.Position.Set {
		if patch.Position.Null {
			writeError(w, http.StatusBadRequest, "INVALID_PLAN_ENTRY", "position cannot be null.")
			return
		}
		input.Position = patch.Position.Value
	}
	if patch.AssignedToID.Set {
		input.AssignedToUserID = nil
		if !patch.AssignedToID.Null {
			value := strings.TrimSpace(patch.AssignedToID.Value)
			input.AssignedToUserID = &value
		}
	}
	if err := validateV2PlanEntryWrite(input); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_PLAN_ENTRY", err.Error())
		return
	}
	if !v2DateInRange(input.PlannedOn, plan.StartsOn, plan.EndsOn) {
		writeError(w, http.StatusBadRequest, "INVALID_RANGE", "The planned date must fall within the plan period.")
		return
	}
	if err := h.validateV2PlanEntryTask(r, workspaceID, input.TaskID, input.OccurrenceOn); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_PLAN_ENTRY", "The task occurrence does not match the selected task.")
		return
	}
	entry, err := h.store.UpdateV2PlanEntry(r.Context(), workspaceID, planID, entryID, userID, role, input, assignChanged)
	if err != nil {
		v2HandleStoreError(w, r, h, userID, "update plan entry", err)
		return
	}
	writeJSON(w, http.StatusOK, entry)
}

func (h *Handler) v2DeletePlanEntry(w http.ResponseWriter, r *http.Request, userID, role string) {
	if err := h.store.DeleteV2PlanEntry(
		r.Context(), r.PathValue("workspaceId"), r.PathValue("planId"),
		r.PathValue("entryId"), userID, role,
	); err != nil {
		v2HandleStoreError(w, r, h, userID, "delete plan entry", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func buildV2PlanEntryWrite(request v2PlanEntryRequest) (postgres.V2PlanEntryWrite, error) {
	plannedOn, ok := parseV2Date(request.PlannedOn)
	if !ok {
		return postgres.V2PlanEntryWrite{}, errors.New("plannedOn must use YYYY-MM-DD")
	}
	input := postgres.V2PlanEntryWrite{
		TaskID:           strings.TrimSpace(request.TaskID),
		PlannedOn:        plannedOn,
		PlannedStartAt:   request.PlannedStartAt,
		PlannedEndAt:     request.PlannedEndAt,
		Position:         request.Position,
		AssignedToUserID: request.AssignedToID,
	}
	if request.OccurrenceOn != nil {
		date, ok := parseV2Date(*request.OccurrenceOn)
		if !ok {
			return input, errors.New("occurrenceOn must use YYYY-MM-DD")
		}
		input.OccurrenceOn = &date
	}
	return input, validateV2PlanEntryWrite(input)
}

func validateV2PlanEntryWrite(input postgres.V2PlanEntryWrite) error {
	if !v2IdentifierPattern.MatchString(input.TaskID) {
		return errors.New("taskId is invalid")
	}
	if input.Position < 0 || input.Position > 1000000 {
		return errors.New("position must be between 0 and 1000000")
	}
	if input.AssignedToUserID != nil && !v2IdentifierPattern.MatchString(*input.AssignedToUserID) {
		return errors.New("assignedToUserId is invalid")
	}
	if input.OccurrenceOn != nil && input.OccurrenceOn.Format("2006-01-02") != input.PlannedOn.Format("2006-01-02") {
		return errors.New("occurrenceOn must match plannedOn")
	}
	if input.PlannedStartAt != nil && input.PlannedEndAt != nil && input.PlannedEndAt.Before(*input.PlannedStartAt) {
		return errors.New("plannedEndAt must be on or after plannedStartAt")
	}
	return nil
}

func (h *Handler) validateV2PlanEntryTask(r *http.Request, workspaceID, taskID string, occurrenceOn *time.Time) error {
	task, err := h.store.GetV2Task(r.Context(), workspaceID, taskID)
	if err != nil {
		return err
	}
	rule, err := recurrenceFromTask(task)
	if err != nil {
		return err
	}
	if rule == nil && occurrenceOn != nil {
		return errors.New("one-time tasks cannot have occurrence links")
	}
	if rule != nil && (occurrenceOn == nil || !v2RecurrenceOccurs(*rule, *occurrenceOn)) {
		return errors.New("recurrence link must use a valid occurrence date")
	}
	return nil
}

func v2DateInRange(value, start, end time.Time) bool {
	value, start, end = v2DateOnly(value), v2DateOnly(start), v2DateOnly(end)
	return !value.Before(start) && !value.After(end)
}

func v2DateOnly(value time.Time) time.Time {
	return time.Date(value.Year(), value.Month(), value.Day(), 0, 0, 0, 0, time.UTC)
}

func (h *Handler) v2ListReminderRules(w http.ResponseWriter, r *http.Request, userID, role string) {
	if !v2QueryValid(r, "limit", "offset") {
		writeError(w, http.StatusBadRequest, "INVALID_QUERY", "The request contains an unsupported or repeated query parameter.")
		return
	}
	limit, offset, ok := v2Page(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "INVALID_PAGINATION", "Use a limit from 1 to 100 and a non-negative offset.")
		return
	}
	rules, err := h.store.ListV2ReminderRules(r.Context(), r.PathValue("workspaceId"), userID, role, limit, offset)
	if err != nil {
		v2HandleStoreError(w, r, h, userID, "list reminder rules", err)
		return
	}
	writeJSON(w, http.StatusOK, rules)
}

func (h *Handler) v2CreateReminderRule(w http.ResponseWriter, r *http.Request, userID, _ string) {
	var request v2ReminderRequest
	if !decodeV2JSON(w, r, &request) {
		return
	}
	if request.Status != "" && request.Status != "pending_automation" {
		writeError(w, http.StatusBadRequest, "AUTOMATION_INACTIVE", "Reminder delivery cannot be activated.")
		return
	}
	if err := h.validateV2ReminderRule(r, r.PathValue("workspaceId"), request.Kind, request.Rule); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REMINDER_RULE", err.Error())
		return
	}
	result, err := h.store.CreateV2ReminderRule(r.Context(), r.PathValue("workspaceId"), userID, request.Kind, request.Rule)
	if err != nil {
		v2HandleStoreError(w, r, h, userID, "create reminder rule", err)
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

func (h *Handler) v2UpdateReminderRule(w http.ResponseWriter, r *http.Request, userID, role string) {
	workspaceID, ruleID := r.PathValue("workspaceId"), r.PathValue("ruleId")
	current, err := h.store.GetV2ReminderRule(r.Context(), workspaceID, ruleID, userID, role)
	if err != nil {
		v2HandleStoreError(w, r, h, userID, "read reminder rule before update", err)
		return
	}
	var patch v2ReminderPatchRequest
	if !decodeV2JSON(w, r, &patch) {
		return
	}
	kind, rule, status := current.Kind, current.Rule, current.Status
	if patch.Kind.Set {
		if patch.Kind.Null {
			writeError(w, http.StatusBadRequest, "INVALID_REMINDER_RULE", "kind cannot be null.")
			return
		}
		kind = patch.Kind.Value
	}
	if patch.Rule.Set {
		if patch.Rule.Null {
			writeError(w, http.StatusBadRequest, "INVALID_REMINDER_RULE", "rule cannot be null.")
			return
		}
		rule = patch.Rule.Value
	}
	if patch.Status.Set {
		if patch.Status.Null || patch.Status.Value != "pending_automation" && patch.Status.Value != "disabled" {
			writeError(w, http.StatusBadRequest, "AUTOMATION_INACTIVE", "Reminder delivery cannot be activated.")
			return
		}
		status = patch.Status.Value
	}
	if err := h.validateV2ReminderRule(r, workspaceID, kind, rule); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REMINDER_RULE", err.Error())
		return
	}
	result, err := h.store.UpdateV2ReminderRule(r.Context(), workspaceID, ruleID, userID, role, kind, rule, status)
	if err != nil {
		v2HandleStoreError(w, r, h, userID, "update reminder rule", err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) v2DeleteReminderRule(w http.ResponseWriter, r *http.Request, userID, role string) {
	if err := h.store.DeleteV2ReminderRule(r.Context(), r.PathValue("workspaceId"), r.PathValue("ruleId"), userID, role); err != nil {
		v2HandleStoreError(w, r, h, userID, "delete reminder rule", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) validateV2ReminderRule(r *http.Request, workspaceID, kind string, rule json.RawMessage) error {
	if !oneOf(kind, "task", "goal", "daily_brief") {
		return errors.New("kind must be task, goal, or daily_brief")
	}
	if len(rule) == 0 || len(rule) > 8*1024 {
		return errors.New("rule must be a JSON object no larger than 8 KiB")
	}
	switch kind {
	case "task", "goal":
		var target struct {
			TargetID      string `json:"targetId"`
			MinutesBefore int    `json:"minutesBefore"`
		}
		if err := decodeStrictRawJSON(rule, &target); err != nil {
			return errors.New("rule must contain targetId and minutesBefore")
		}
		if !v2IdentifierPattern.MatchString(target.TargetID) || target.MinutesBefore < 0 || target.MinutesBefore > 10080 {
			return errors.New("targetId or minutesBefore is invalid")
		}
		if kind == "task" {
			if _, err := h.store.GetV2Task(r.Context(), workspaceID, target.TargetID); err != nil {
				return errors.New("targetId must identify a task in this workspace")
			}
		} else if _, err := h.store.GetV2Goal(r.Context(), workspaceID, target.TargetID); err != nil {
			return errors.New("targetId must identify a goal in this workspace")
		}
	case "daily_brief":
		var schedule struct {
			LocalTime string `json:"localTime"`
			TimeZone  string `json:"timeZone"`
		}
		if err := decodeStrictRawJSON(rule, &schedule); err != nil {
			return errors.New("rule must contain localTime and timeZone")
		}
		if _, err := time.Parse("15:04", schedule.LocalTime); err != nil || schedule.TimeZone == "" || len(schedule.TimeZone) > 64 {
			return errors.New("localTime or timeZone is invalid")
		}
		if _, err := time.LoadLocation(schedule.TimeZone); err != nil {
			return errors.New("timeZone must be a valid IANA time zone")
		}
	}
	return nil
}

func decodeStrictRawJSON(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("multiple JSON values are not allowed")
		}
		return fmt.Errorf("decode trailing JSON value: %w", err)
	}
	return nil
}

func (h *Handler) v2GetAgenda(w http.ResponseWriter, r *http.Request, userID, role string) {
	if !v2QueryValid(r, "from", "to") {
		writeError(w, http.StatusBadRequest, "INVALID_QUERY", "The request contains an unsupported or repeated query parameter.")
		return
	}
	query := r.URL.Query()
	from, fromOK := parseV2Date(query.Get("from"))
	to, toOK := parseV2Date(query.Get("to"))
	if !fromOK || !toOK || to.Before(from) || to.Sub(from) > (v2MaxDateRange-1)*24*time.Hour {
		writeError(w, http.StatusBadRequest, "INVALID_DATE_RANGE", "Provide a valid agenda range of no more than 31 days.")
		return
	}
	workspaceID := r.PathValue("workspaceId")
	zone, err := h.store.V2WorkspaceTimeZone(r.Context(), workspaceID)
	if err != nil {
		v2HandleStoreError(w, r, h, userID, "read workspace time zone", err)
		return
	}
	if _, err := time.LoadLocation(zone); err != nil {
		h.v2InternalError(w, r, userID, "validate workspace time zone", err)
		return
	}
	tasks, err := h.store.ListV2Tasks(r.Context(), workspaceID, postgres.V2TaskFilter{
		From: &from, To: &to, Limit: 100,
	})
	if err != nil {
		v2HandleStoreError(w, r, h, userID, "list agenda tasks", err)
		return
	}
	states, err := h.store.ListV2AgendaOccurrenceStates(r.Context(), workspaceID, from, to)
	if err != nil {
		v2HandleStoreError(w, r, h, userID, "list agenda occurrence states", err)
		return
	}
	stateByKey := make(map[string]postgres.V2TaskOccurrence, len(states))
	for _, state := range states {
		stateByKey[state.TaskID+":"+state.OccurrenceOn.Format("2006-01-02")] = state
	}
	agendaTasks := make([]postgres.V2TaskAgendaItem, 0)
	for _, task := range tasks {
		if task.Status == "archived" || task.Status == "cancelled" {
			continue
		}
		rule, err := recurrenceFromTask(task)
		if err != nil {
			h.v2InternalError(w, r, userID, "read task recurrence", err)
			return
		}
		if rule == nil {
			if task.DueOn != nil && v2DateInRange(*task.DueOn, from, to) {
				agendaTasks = append(agendaTasks, postgres.V2TaskAgendaItem{
					Kind: "task", TaskID: task.ID, Title: task.Title,
					Status: task.Status, Priority: task.Priority, DueOn: task.DueOn,
					PlannedStartAt: task.PlannedStartAt, AssignedToUserID: task.AssignedToUserID,
				})
			}
			continue
		}
		dates, err := expandV2Recurrence(*rule, from, to)
		if err != nil {
			h.v2InternalError(w, r, userID, "expand task recurrence", err)
			return
		}
		for _, date := range dates {
			state, exists := stateByKey[task.ID+":"+date.Format("2006-01-02")]
			if exists && (state.Status == "cancelled" || state.Status == "skipped") {
				continue
			}
			status := "open"
			var plannedStart *time.Time
			if exists {
				status = state.Status
				plannedStart = state.DueAtOverride
			}
			dateCopy := date
			agendaTasks = append(agendaTasks, postgres.V2TaskAgendaItem{
				Kind: "task_occurrence", TaskID: task.ID, OccurrenceOn: &dateCopy,
				Title: task.Title, Status: status, Priority: task.Priority,
				DueOn: &dateCopy, PlannedStartAt: plannedStart,
				AssignedToUserID: task.AssignedToUserID,
			})
			if len(agendaTasks) >= 1000 {
				break
			}
		}
		if len(agendaTasks) >= 1000 {
			break
		}
	}
	sort.SliceStable(agendaTasks, func(i, j int) bool {
		left, right := agendaTasks[i], agendaTasks[j]
		leftDate, rightDate := left.DueOn, right.DueOn
		if leftDate == nil {
			leftDate = left.OccurrenceOn
		}
		if rightDate == nil {
			rightDate = right.OccurrenceOn
		}
		if leftDate != nil && rightDate != nil && !leftDate.Equal(*rightDate) {
			return leftDate.Before(*rightDate)
		}
		return left.TaskID < right.TaskID
	})
	planEntries, err := h.store.ListV2AgendaPlanEntries(r.Context(), workspaceID, userID, role, from, to, 500)
	if err != nil {
		v2HandleStoreError(w, r, h, userID, "list agenda plan entries", err)
		return
	}
	events, err := h.store.ListV2AgendaEvents(r.Context(), workspaceID, from, to, 500)
	if err != nil {
		v2HandleStoreError(w, r, h, userID, "list agenda calendar events", err)
		return
	}
	writeJSON(w, http.StatusOK, postgres.V2Agenda{
		WorkspaceID:    workspaceID,
		From:           from.Format("2006-01-02"),
		To:             to.Format("2006-01-02"),
		Tasks:          agendaTasks,
		PlanEntries:    planEntries,
		CalendarEvents: events,
	})
}
