package product

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sort"
	"time"

	"askolo/backend/internal/adapters/postgres"
)

type SharedEmailMessage struct {
	ID       string   `json:"id"`
	ThreadID string   `json:"threadId,omitempty"`
	Subject  string   `json:"subject,omitempty"`
	From     string   `json:"from,omitempty"`
	Date     string   `json:"date,omitempty"`
	Snippet  string   `json:"snippet,omitempty"`
	BodyText string   `json:"bodyText,omitempty"`
	LabelIDs []string `json:"labelIds,omitempty"`
}

type SharedEmailPage struct {
	Messages      []SharedEmailMessage `json:"messages"`
	NextPageToken string               `json:"nextPageToken,omitempty"`
}

type SharedEmailReader interface {
	ListSharedGmailMessages(context.Context, string, string, string) (SharedEmailPage, error)
	GetSharedGmailMessage(context.Context, string, string, string) (SharedEmailMessage, error)
}

func (h *Handler) v2ListWorkspaces(w http.ResponseWriter, r *http.Request, userID string) {
	workspaces, err := h.store.ListV2HomeWorkspaces(r.Context(), userID)
	if errors.Is(err, postgres.ErrV2HomeSpaceLimit) {
		writeError(w, http.StatusUnprocessableEntity, "SPACE_LIST_TOO_LARGE", "This account has too many active Spaces to list in one request.")
		return
	}
	if err != nil {
		h.v2InternalError(w, r, userID, "list Spaces", err)
		return
	}
	writeJSON(w, http.StatusOK, workspaces)
}

type homeFocusAction struct {
	WorkspaceID   string  `json:"workspaceId"`
	WorkspaceName string  `json:"workspaceName"`
	TaskID        string  `json:"taskId"`
	OccurrenceOn  *string `json:"occurrenceOn"`
	Title         string  `json:"title"`
	Status        string  `json:"status"`
	Priority      string  `json:"priority"`
	Selection     string  `json:"selection"`
	position      int
}

type homeSpaceEmailSummary struct {
	WorkspaceID       string     `json:"workspaceId"`
	WorkspaceName     string     `json:"workspaceName"`
	UnreadInboxCount  int64      `json:"unreadInboxCount"`
	ActionNeededCount int64      `json:"actionNeededCount"`
	LastSyncedAt      *time.Time `json:"lastSyncedAt"`
	Freshness         string     `json:"freshness"`
}

type homeEmailSummary struct {
	UnreadInboxCount  int64                   `json:"unreadInboxCount"`
	ActionNeededCount int64                   `json:"actionNeededCount"`
	LastSyncedAt      *time.Time              `json:"lastSyncedAt"`
	Freshness         string                  `json:"freshness"`
	Spaces            []homeSpaceEmailSummary `json:"spaces,omitempty"`
}

type homeNextEvent struct {
	Title    string    `json:"title"`
	StartsAt time.Time `json:"startsAt"`
}

type homeCalendarSummary struct {
	EventCount int            `json:"eventCount"`
	NextEvent  *homeNextEvent `json:"nextEvent"`
}

type homeProjection struct {
	Date            string              `json:"date"`
	TimeZone        string              `json:"timeZone"`
	GeneratedAt     time.Time           `json:"generatedAt"`
	Scope           string              `json:"scope"`
	FocusActions    []homeFocusAction   `json:"focusActions"`
	EmailSummary    homeEmailSummary    `json:"emailSummary"`
	CalendarSummary homeCalendarSummary `json:"calendarSummary"`
}

type homeWorkspaceProjection struct {
	workspace   postgres.V2HomeWorkspace
	saved       []homeFocusAction
	suggestions []homeFocusAction
	email       postgres.V2HomeEmailSummary
	events      []postgres.V2CalendarEventAgendaItem
}

type homeTaskCandidate struct {
	task         postgres.V2Task
	occurrenceOn *time.Time
	status       string
}

type replaceHomeFocusInput struct {
	Items []struct {
		TaskID       string  `json:"taskId"`
		OccurrenceOn *string `json:"occurrenceOn"`
	} `json:"items"`
}

func (h *Handler) v2GetWorkspaceHome(w http.ResponseWriter, r *http.Request, userID, role string) {
	workspaceID := r.PathValue("workspaceId")
	workspaceName, err := h.store.V2WorkspaceName(r.Context(), workspaceID)
	if err != nil {
		v2HandleStoreError(w, r, h, userID, "read Home workspace", err)
		return
	}
	zone, err := h.store.V2HomeTimeZone(r.Context(), userID)
	if err != nil {
		h.v2InternalError(w, r, userID, "read Home time zone", err)
		return
	}
	home, err := h.buildHomeProjection(r.Context(), userID, []postgres.V2HomeWorkspace{{
		ID: workspaceID, Name: workspaceName, Role: role,
	}}, zone, "workspace")
	if err != nil {
		h.writeHomeError(w, r, userID, "build workspace Home", err)
		return
	}
	writeJSON(w, http.StatusOK, home)
}

func (h *Handler) v2GetAllSpacesHome(w http.ResponseWriter, r *http.Request, userID string) {
	workspaces, err := h.store.ListV2HomeWorkspaces(r.Context(), userID)
	if errors.Is(err, postgres.ErrV2HomeSpaceLimit) {
		writeError(w, http.StatusUnprocessableEntity, "HOME_SCOPE_TOO_LARGE", "This account has too many active Spaces to summarize in one request.")
		return
	}
	if err != nil {
		h.v2InternalError(w, r, userID, "list All Spaces Home workspaces", err)
		return
	}
	active := make([]postgres.V2HomeWorkspace, 0, len(workspaces))
	for _, workspace := range workspaces {
		role, roleErr := h.store.V2WorkspaceRole(r.Context(), workspace.ID, userID)
		if errors.Is(roleErr, postgres.ErrNotFound) {
			continue
		}
		if roleErr != nil {
			h.v2InternalError(w, r, userID, "authorize All Spaces Home", roleErr)
			return
		}
		workspace.Role = role
		active = append(active, workspace)
	}
	zone, err := h.store.V2HomeTimeZone(r.Context(), userID)
	if err != nil {
		h.v2InternalError(w, r, userID, "read All Spaces Home time zone", err)
		return
	}
	home, err := h.buildHomeProjection(r.Context(), userID, active, zone, "all_spaces")
	if err != nil {
		h.writeHomeError(w, r, userID, "build All Spaces Home", err)
		return
	}
	writeJSON(w, http.StatusOK, home)
}

func (h *Handler) buildHomeProjection(
	ctx context.Context,
	userID string,
	workspaces []postgres.V2HomeWorkspace,
	timeZone, scope string,
) (homeProjection, error) {
	location, err := time.LoadLocation(timeZone)
	if err != nil {
		return homeProjection{}, err
	}
	now := time.Now().In(location)
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, location)
	end := start.AddDate(0, 0, 1)
	workspaceIDs := make([]string, 0, len(workspaces))
	for _, workspace := range workspaces {
		workspaceIDs = append(workspaceIDs, workspace.ID)
	}
	emailByWorkspace, err := h.store.ListV2HomeEmailSummaries(ctx, userID, workspaceIDs)
	if err != nil {
		return homeProjection{}, err
	}

	projections := make([]homeWorkspaceProjection, 0, len(workspaces))
	for _, workspace := range workspaces {
		projection, err := h.loadHomeWorkspace(ctx, userID, workspace, start, end)
		if err != nil {
			return homeProjection{}, err
		}
		if summary, ok := emailByWorkspace[workspace.ID]; ok {
			projection.email = summary
		} else {
			projection.email.Freshness = "unavailable"
		}
		projections = append(projections, projection)
	}

	home := homeProjection{
		Date: start.Format("2006-01-02"), TimeZone: timeZone,
		GeneratedAt: now.UTC(), Scope: scope,
		FocusActions:    make([]homeFocusAction, 0, 3),
		EmailSummary:    homeEmailSummary{Freshness: "unavailable"},
		CalendarSummary: homeCalendarSummary{},
	}
	savedActions := make([]homeFocusAction, 0, 3)
	suggestedActions := make([]homeFocusAction, 0, 3)
	focusSeen := make(map[string]struct{})
	var lastSyncedAt *time.Time
	freshnessStates := make([]string, 0, len(projections))
	for _, projection := range projections {
		for _, action := range projection.saved {
			key := action.WorkspaceID + ":" + action.TaskID
			if action.OccurrenceOn != nil {
				key += ":" + *action.OccurrenceOn
			}
			if _, exists := focusSeen[key]; !exists {
				focusSeen[key] = struct{}{}
				savedActions = append(savedActions, action)
			}
		}
		for _, action := range projection.suggestions {
			key := action.WorkspaceID + ":" + action.TaskID
			if action.OccurrenceOn != nil {
				key += ":" + *action.OccurrenceOn
			}
			if _, exists := focusSeen[key]; !exists {
				focusSeen[key] = struct{}{}
				suggestedActions = append(suggestedActions, action)
			}
		}
		home.EmailSummary.UnreadInboxCount += projection.email.UnreadInboxCount
		home.EmailSummary.ActionNeededCount += projection.email.ActionNeededCount
		if projection.email.LastSyncedAt != nil &&
			(lastSyncedAt == nil || projection.email.LastSyncedAt.After(*lastSyncedAt)) {
			copied := *projection.email.LastSyncedAt
			lastSyncedAt = &copied
		}
		freshnessStates = append(freshnessStates, projection.email.Freshness)
		home.CalendarSummary.EventCount += len(projection.events)
	}
	home.EmailSummary.LastSyncedAt = lastSyncedAt
	home.EmailSummary.Freshness = combineHomeFreshness(freshnessStates)
	if scope == "all_spaces" {
		home.EmailSummary.Spaces = make([]homeSpaceEmailSummary, 0, len(projections))
		for _, projection := range projections {
			home.EmailSummary.Spaces = append(home.EmailSummary.Spaces, homeSpaceEmailSummary{
				WorkspaceID: projection.workspace.ID, WorkspaceName: projection.workspace.Name,
				UnreadInboxCount:  projection.email.UnreadInboxCount,
				ActionNeededCount: projection.email.ActionNeededCount,
				LastSyncedAt:      projection.email.LastSyncedAt,
				Freshness:         projection.email.Freshness,
			})
		}
	}
	home.FocusActions = append(home.FocusActions, savedActions...)
	home.FocusActions = append(home.FocusActions, suggestedActions...)
	if len(home.FocusActions) > 3 {
		home.FocusActions = home.FocusActions[:3]
	}

	var next *homeNextEvent
	for _, projection := range projections {
		for _, event := range projection.events {
			if !event.StartsAt.After(now) {
				continue
			}
			if next == nil || event.StartsAt.Before(next.StartsAt) {
				next = &homeNextEvent{Title: event.Title, StartsAt: event.StartsAt}
			}
		}
	}
	home.CalendarSummary.NextEvent = next
	return home, nil
}

func combineHomeFreshness(states []string) string {
	if len(states) == 0 {
		return "unavailable"
	}
	for _, state := range states {
		if state == "error" {
			return "error"
		}
	}
	for _, state := range states {
		if state == "syncing" {
			return "syncing"
		}
	}
	allCurrent, allUnavailable := true, true
	for _, state := range states {
		allCurrent = allCurrent && state == "current"
		allUnavailable = allUnavailable && state == "unavailable"
	}
	if allCurrent {
		return "current"
	}
	if allUnavailable {
		return "unavailable"
	}
	return "stale"
}

func (h *Handler) loadHomeWorkspace(
	ctx context.Context,
	userID string,
	workspace postgres.V2HomeWorkspace,
	start, end time.Time,
) (homeWorkspaceProjection, error) {
	projection := homeWorkspaceProjection{
		workspace:   workspace,
		saved:       make([]homeFocusAction, 0, 3),
		suggestions: make([]homeFocusAction, 0, 3),
		email:       postgres.V2HomeEmailSummary{Freshness: "unavailable"},
		events:      make([]postgres.V2CalendarEventAgendaItem, 0),
	}
	for offset := 0; offset < 5000; offset += 100 {
		page, err := h.store.ListV2AgendaEvents(ctx, workspace.ID, start, start, start.Location().String(), 100, offset)
		if err != nil {
			return homeWorkspaceProjection{}, err
		}
		projection.events = append(projection.events, page...)
		if len(page) < 100 {
			break
		}
		if offset == 4900 {
			return homeWorkspaceProjection{}, errors.New("Home calendar event limit exceeded")
		}
	}
	planEntries, err := h.store.ListV2AgendaPlanEntries(
		ctx, workspace.ID, userID, workspace.Role, start, start, 100, 0,
	)
	if err != nil {
		return homeWorkspaceProjection{}, err
	}
	tasks, err := h.store.ListV2Tasks(ctx, workspace.ID, postgres.V2TaskFilter{
		From: &start, To: &start, TimeZone: start.Location().String(), Limit: 100,
	})
	if err != nil {
		return homeWorkspaceProjection{}, err
	}
	taskByID := make(map[string]postgres.V2Task, len(tasks))
	taskIDs := make([]string, 0, len(tasks))
	for _, task := range tasks {
		taskByID[task.ID] = task
		taskIDs = append(taskIDs, task.ID)
	}
	states, err := h.store.ListV2AgendaOccurrenceStates(ctx, workspace.ID, taskIDs, start, start)
	if err != nil {
		return homeWorkspaceProjection{}, err
	}
	stateByKey := make(map[string]postgres.V2TaskOccurrence, len(states))
	for _, state := range states {
		stateByKey[state.TaskID+":"+state.OccurrenceOn.Format("2006-01-02")] = state
	}

	selections, err := h.store.ListV2HomeFocusSelections(ctx, workspace.ID, userID, start)
	if err != nil {
		return homeWorkspaceProjection{}, err
	}
	for _, selection := range selections {
		task, ok := taskByID[selection.TaskID]
		if !ok {
			task, err = h.store.GetV2Task(ctx, workspace.ID, selection.TaskID)
			if err != nil {
				if errors.Is(err, postgres.ErrNotFound) {
					continue
				}
				return homeWorkspaceProjection{}, err
			}
			taskByID[task.ID] = task
		}
		candidate, valid, err := savedHomeCandidate(task, selection, start, end, stateByKey, ctx, h.store, workspace.ID)
		if err != nil {
			return homeWorkspaceProjection{}, err
		}
		if valid {
			projection.saved = append(projection.saved, makeHomeFocusAction(workspace, candidate, "saved", selection.Position))
		}
	}
	sort.SliceStable(projection.saved, func(i, j int) bool {
		return projection.saved[i].position < projection.saved[j].position
	})

	seen := make(map[string]struct{}, len(projection.saved)+len(planEntries)+len(tasks))
	for _, action := range projection.saved {
		seen[homeFocusKey(action.TaskID, action.OccurrenceOn)] = struct{}{}
	}
	for _, entry := range planEntries {
		if !homeTaskStatusActive(entry.TaskStatus) {
			continue
		}
		task, ok := taskByID[entry.TaskID]
		if !ok {
			task, err = h.store.GetV2Task(ctx, workspace.ID, entry.TaskID)
			if err != nil {
				continue
			}
			taskByID[task.ID] = task
		}
		candidate := homeTaskCandidate{task: task, occurrenceOn: entry.OccurrenceOn, status: entry.TaskStatus}
		key := homeFocusKey(candidate.task.ID, formatHomeDate(candidate.occurrenceOn))
		if _, duplicate := seen[key]; duplicate {
			continue
		}
		seen[key] = struct{}{}
		projection.suggestions = append(projection.suggestions, makeHomeFocusAction(workspace, candidate, "suggested", len(projection.suggestions)))
	}
	for _, task := range tasks {
		if !homeTaskStatusActive(task.Status) {
			continue
		}
		rule, err := recurrenceFromTask(task)
		if err != nil {
			return homeWorkspaceProjection{}, err
		}
		if rule != nil {
			dates, err := expandV2Recurrence(*rule, start, start)
			if err != nil {
				return homeWorkspaceProjection{}, err
			}
			for _, date := range dates {
				state, exists := stateByKey[task.ID+":"+date.Format("2006-01-02")]
				if exists && !homeTaskStatusActive(state.Status) {
					continue
				}
				dateCopy := date
				candidate := homeTaskCandidate{task: task, occurrenceOn: &dateCopy, status: task.Status}
				if exists {
					candidate.status = state.Status
				}
				key := homeFocusKey(task.ID, formatHomeDate(candidate.occurrenceOn))
				if _, duplicate := seen[key]; duplicate {
					continue
				}
				seen[key] = struct{}{}
				projection.suggestions = append(projection.suggestions, makeHomeFocusAction(workspace, candidate, "suggested", len(projection.suggestions)))
			}
			continue
		}
		if !homeTaskDueToday(task, start, end) {
			continue
		}
		candidate := homeTaskCandidate{task: task, status: task.Status}
		key := homeFocusKey(task.ID, nil)
		if _, duplicate := seen[key]; duplicate {
			continue
		}
		seen[key] = struct{}{}
		projection.suggestions = append(projection.suggestions, makeHomeFocusAction(workspace, candidate, "suggested", len(projection.suggestions)))
	}
	return projection, nil
}

func savedHomeCandidate(
	task postgres.V2Task,
	selection postgres.V2HomeFocusSelection,
	start, end time.Time,
	stateByKey map[string]postgres.V2TaskOccurrence,
	ctx context.Context,
	store *postgres.Store,
	workspaceID string,
) (homeTaskCandidate, bool, error) {
	if !homeTaskStatusActive(task.Status) {
		return homeTaskCandidate{}, false, nil
	}
	rule, err := recurrenceFromTask(task)
	if err != nil {
		return homeTaskCandidate{}, false, err
	}
	if selection.OccurrenceOn == nil {
		return homeTaskCandidate{task: task, status: task.Status}, rule == nil, nil
	}
	if rule == nil || selection.OccurrenceOn.Format("2006-01-02") != start.Format("2006-01-02") {
		return homeTaskCandidate{}, false, nil
	}
	dates, err := expandV2Recurrence(*rule, start, start)
	if err != nil {
		return homeTaskCandidate{}, false, err
	}
	occurrenceDate := *selection.OccurrenceOn
	isOccurrence := false
	for _, date := range dates {
		if date.Format("2006-01-02") == occurrenceDate.Format("2006-01-02") {
			isOccurrence = true
			break
		}
	}
	if !isOccurrence {
		return homeTaskCandidate{}, false, nil
	}
	state, exists := stateByKey[task.ID+":"+occurrenceDate.Format("2006-01-02")]
	if !exists {
		states, stateErr := store.ListV2AgendaOccurrenceStates(ctx, workspaceID, []string{task.ID}, occurrenceDate, occurrenceDate)
		if stateErr != nil {
			return homeTaskCandidate{}, false, stateErr
		}
		for _, item := range states {
			if item.TaskID == task.ID && item.OccurrenceOn.Format("2006-01-02") == occurrenceDate.Format("2006-01-02") {
				state, exists = item, true
				break
			}
		}
	}
	if exists && !homeTaskStatusActive(state.Status) {
		return homeTaskCandidate{}, false, nil
	}
	status := task.Status
	if exists {
		status = state.Status
	}
	dateCopy := occurrenceDate
	return homeTaskCandidate{task: task, occurrenceOn: &dateCopy, status: status}, true, nil
}

func homeTaskStatusActive(status string) bool {
	switch status {
	case "completed", "done", "cancelled", "archived", "skipped":
		return false
	default:
		return true
	}
}

func homeTaskDueToday(task postgres.V2Task, start, end time.Time) bool {
	if task.DueOn != nil && task.DueOn.Format("2006-01-02") == start.Format("2006-01-02") {
		return true
	}
	if task.PlannedStartAt != nil {
		local := task.PlannedStartAt.In(start.Location())
		if !local.Before(start) && local.Before(end) {
			return true
		}
	}
	return false
}

func makeHomeFocusAction(
	workspace postgres.V2HomeWorkspace,
	candidate homeTaskCandidate,
	selection string,
	position int,
) homeFocusAction {
	var occurrenceOn *string
	if candidate.occurrenceOn != nil {
		formatted := candidate.occurrenceOn.Format("2006-01-02")
		occurrenceOn = &formatted
	}
	return homeFocusAction{
		WorkspaceID: workspace.ID, WorkspaceName: workspace.Name,
		TaskID: candidate.task.ID, OccurrenceOn: occurrenceOn,
		Title: candidate.task.Title, Status: candidate.status,
		Priority: candidate.task.Priority, Selection: selection, position: position,
	}
}

func formatHomeDate(value *time.Time) *string {
	if value == nil {
		return nil
	}
	formatted := value.Format("2006-01-02")
	return &formatted
}

func homeFocusKey(taskID string, occurrenceOn *string) string {
	if occurrenceOn == nil {
		return taskID
	}
	return taskID + ":" + *occurrenceOn
}

func (h *Handler) writeHomeError(w http.ResponseWriter, r *http.Request, userID, operation string, err error) {
	if errors.Is(err, postgres.ErrV2InvalidHomeFocus) {
		writeError(w, http.StatusBadRequest, "INVALID_FOCUS", "The selected focus actions are unavailable.")
		return
	}
	if errors.Is(err, postgres.ErrV2HomeSpaceLimit) {
		writeError(w, http.StatusUnprocessableEntity, "HOME_SCOPE_TOO_LARGE", "This account has too many active Spaces to summarize in one request.")
		return
	}
	h.v2InternalError(w, r, userID, operation, err)
}

func (h *Handler) v2ReplaceHomeFocus(w http.ResponseWriter, r *http.Request, userID, role string) {
	var input replaceHomeFocusInput
	if err := decodeHomeBody(w, r, &input, 16*1024); err != nil || input.Items == nil || len(input.Items) > 3 {
		writeError(w, http.StatusBadRequest, "INVALID_FOCUS", "Choose up to three valid focus actions.")
		return
	}
	timeZone, err := h.store.V2HomeTimeZone(r.Context(), userID)
	if err != nil {
		h.v2InternalError(w, r, userID, "read focus time zone", err)
		return
	}
	location, err := time.LoadLocation(timeZone)
	if err != nil {
		h.v2InternalError(w, r, userID, "load focus time zone", err)
		return
	}
	now := time.Now().In(location)
	planDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, location)
	workspaceID := r.PathValue("workspaceId")

	references := make([]postgres.V2HomeFocusReference, 0, len(input.Items))
	taskIDs := make([]string, 0, len(input.Items))
	seen := make(map[string]struct{}, len(input.Items))
	for _, item := range input.Items {
		if !v2IdentifierPattern.MatchString(item.TaskID) {
			writeError(w, http.StatusBadRequest, "INVALID_FOCUS", "Choose up to three valid focus actions.")
			return
		}
		reference := postgres.V2HomeFocusReference{TaskID: item.TaskID, Position: len(references)}
		key := item.TaskID
		if item.OccurrenceOn != nil {
			date, parseErr := time.ParseInLocation("2006-01-02", *item.OccurrenceOn, location)
			if parseErr != nil || date.Format("2006-01-02") != planDay.Format("2006-01-02") {
				writeError(w, http.StatusBadRequest, "INVALID_FOCUS", "Recurring focus actions must be occurrences for today.")
				return
			}
			reference.OccurrenceOn = &date
			key += ":" + date.Format("2006-01-02")
		}
		if _, duplicate := seen[key]; duplicate {
			writeError(w, http.StatusBadRequest, "INVALID_FOCUS", "A focus action cannot be selected more than once.")
			return
		}
		seen[key] = struct{}{}

		task, taskErr := h.store.GetV2Task(r.Context(), workspaceID, item.TaskID)
		if taskErr != nil {
			v2HandleStoreError(w, r, h, userID, "validate Home focus task", taskErr)
			return
		}
		if !homeTaskStatusActive(task.Status) {
			writeError(w, http.StatusBadRequest, "INVALID_FOCUS", "Completed or inactive tasks cannot be selected.")
			return
		}
		rule, recurrenceErr := recurrenceFromTask(task)
		if recurrenceErr != nil {
			h.v2InternalError(w, r, userID, "validate Home focus recurrence", recurrenceErr)
			return
		}
		if rule == nil && item.OccurrenceOn != nil {
			writeError(w, http.StatusBadRequest, "INVALID_FOCUS", "This task does not have a recurring occurrence.")
			return
		}
		if rule != nil {
			if item.OccurrenceOn == nil {
				writeError(w, http.StatusBadRequest, "INVALID_FOCUS", "Select today's occurrence for a recurring task.")
				return
			}
			dates, expandErr := expandV2Recurrence(*rule, planDay, planDay)
			if expandErr != nil {
				h.v2InternalError(w, r, userID, "validate Home focus occurrence", expandErr)
				return
			}
			matches := false
			for _, date := range dates {
				matches = matches || date.Format("2006-01-02") == *item.OccurrenceOn
			}
			if !matches {
				writeError(w, http.StatusBadRequest, "INVALID_FOCUS", "The selected recurring occurrence is not scheduled for today.")
				return
			}
			taskIDs = append(taskIDs, task.ID)
		}
		references = append(references, reference)
	}
	if len(taskIDs) > 0 {
		states, stateErr := h.store.ListV2AgendaOccurrenceStates(r.Context(), workspaceID, taskIDs, planDay, planDay)
		if stateErr != nil {
			h.v2InternalError(w, r, userID, "read Home focus occurrence states", stateErr)
			return
		}
		for _, state := range states {
			if !homeTaskStatusActive(state.Status) {
				key := state.TaskID + ":" + state.OccurrenceOn.Format("2006-01-02")
				if _, selected := seen[key]; selected {
					writeError(w, http.StatusBadRequest, "INVALID_FOCUS", "Completed or inactive occurrences cannot be selected.")
					return
				}
			}
		}
	}
	if err := h.store.ReplaceV2HomeFocusSelections(r.Context(), workspaceID, userID, planDay, references); err != nil {
		h.writeHomeError(w, r, userID, "replace Home focus selections", err)
		return
	}
	h.v2GetWorkspaceHome(w, r, userID, role)
}

func decodeHomeBody(w http.ResponseWriter, r *http.Request, target any, limit int64) error {
	if r.Body == nil {
		return io.EOF
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, limit))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return errors.New("request must contain exactly one JSON value")
	}
	return nil
}
