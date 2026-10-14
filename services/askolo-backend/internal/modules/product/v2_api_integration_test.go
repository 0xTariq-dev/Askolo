package product

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"askolo/backend/internal/adapters/postgres"
	"askolo/backend/internal/config"
	"askolo/backend/internal/migrations"
)

func TestV2WorkspacePlansTasksAndReminderRules(t *testing.T) {
	fixture := openCreditPolicyIntegrationFixture(t)
	const (
		ownerID     = "v2-plan-owner"
		memberID    = "v2-plan-member"
		impostorID  = "v2-plan-impostor"
		revokedID   = "v2-plan-revoked"
		workspaceID = "v2-shared-workspace"
	)
	ownerSession := fixture.createUserAndSession(t, ownerID, "v2-owner@example.test", true)
	memberSession := fixture.createUserAndSession(t, memberID, "v2-member@example.test", true)
	impostorSession := fixture.createUserAndSession(t, impostorID, "v2-impostor@example.test", true)
	revokedSession := fixture.createUserAndSession(t, revokedID, "v2-revoked@example.test", true)

	if err := fixture.store.CreateWorkspace(
		fixture.ctx,
		workspaceID,
		"V2 shared workspace",
		ownerID,
		[]string{"workspace:manage"},
	); err != nil {
		t.Fatalf("create shared workspace: %v", err)
	}
	if err := fixture.store.SetWorkspaceMembership(
		fixture.ctx, workspaceID, memberID, "active", "member", []string{},
	); err != nil {
		t.Fatalf("add shared workspace member: %v", err)
	}
	if err := fixture.store.SetWorkspaceMembership(
		fixture.ctx, workspaceID, impostorID, "active", "owner", []string{},
	); err != nil {
		t.Fatalf("add malformed non-owner owner-role fixture: %v", err)
	}
	if err := fixture.store.SetWorkspaceMembership(
		fixture.ctx, workspaceID, revokedID, "revoked", "member", []string{},
	); err != nil {
		t.Fatalf("add revoked workspace member fixture: %v", err)
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

	taskResponse := request(
		ownerSession,
		http.MethodPost,
		"/api/v2/workspaces/"+workspaceID+"/tasks",
		`{"kind":"task","title":"Canonical task","status":"open","priority":"normal","plannedOn":"2026-10-12","plannedStartTime":"23:30","durationMinutes":60,"dueOn":"2026-10-15","dueTime":"09:15","timeZone":"Africa/Cairo","label":"Bills","details":"Pay before the end of the month."}`,
	)
	if taskResponse.Code != http.StatusCreated {
		t.Fatalf("create task status = %d, want %d: %s", taskResponse.Code, http.StatusCreated, taskResponse.Body.String())
	}
	var task struct {
		ID               string  `json:"id"`
		Kind             string  `json:"kind"`
		Priority         string  `json:"priority"`
		PlannedOn        string  `json:"plannedOn"`
		PlannedStartTime string  `json:"plannedStartTime"`
		DurationMinutes  int     `json:"durationMinutes"`
		DueOn            string  `json:"dueOn"`
		DueTime          string  `json:"dueTime"`
		TimeZone         string  `json:"timeZone"`
		Label            string  `json:"label"`
		Details          string  `json:"details"`
		AssignedToUserID *string `json:"assignedToUserId"`
	}
	if err := json.NewDecoder(taskResponse.Body).Decode(&task); err != nil {
		t.Fatalf("decode created task: %v", err)
	}
	if task.ID == "" {
		t.Fatal("created task has no ID")
	}
	tasksResponse := request(
		ownerSession,
		http.MethodGet,
		"/api/v2/workspaces/"+workspaceID+"/tasks?kind=task",
		"",
	)
	if tasksResponse.Code != http.StatusOK {
		t.Fatalf("list tasks status = %d, want %d: %s", tasksResponse.Code, http.StatusOK, tasksResponse.Body.String())
	}
	var listedTasks []struct {
		ID   string `json:"id"`
		Kind string `json:"kind"`
	}
	if err := json.NewDecoder(tasksResponse.Body).Decode(&listedTasks); err != nil {
		t.Fatalf("decode tasks: %v", err)
	}
	foundTask := false
	for _, listedTask := range listedTasks {
		if listedTask.ID == task.ID && listedTask.Kind == "task" {
			foundTask = true
		}
	}
	if !foundTask {
		t.Fatalf("Tasks route omitted the canonical Task record %q: %#v", task.ID, listedTasks)
	}
	retiredActionsResponse := request(ownerSession, http.MethodGet, "/api/v2/workspaces/"+workspaceID+"/actions", "")
	if retiredActionsResponse.Code != http.StatusNotFound {
		t.Fatalf("retired Actions route status = %d, want %d", retiredActionsResponse.Code, http.StatusNotFound)
	}
	retiredChoresResponse := request(ownerSession, http.MethodGet, "/api/chores", "")
	if retiredChoresResponse.Code != http.StatusNotFound {
		t.Fatalf("retired Chores route status = %d, want %d", retiredChoresResponse.Code, http.StatusNotFound)
	}
	taskResponse = request(
		ownerSession,
		http.MethodGet,
		"/api/v2/workspaces/"+workspaceID+"/tasks/"+task.ID,
		"",
	)
	if taskResponse.Code != http.StatusOK {
		t.Fatalf("get task status = %d, want %d: %s", taskResponse.Code, http.StatusOK, taskResponse.Body.String())
	}
	taskUpdateResponse := request(
		ownerSession,
		http.MethodPatch,
		"/api/v2/workspaces/"+workspaceID+"/tasks/"+task.ID,
		`{"title":"Canonical task"}`,
	)
	if taskUpdateResponse.Code != http.StatusOK {
		t.Fatalf("update task status = %d, want %d: %s", taskUpdateResponse.Code, http.StatusOK, taskUpdateResponse.Body.String())
	}
	if task.Kind != "task" || task.Priority != "normal" ||
		!strings.HasPrefix(task.PlannedOn, "2026-10-12") ||
		!strings.HasPrefix(task.PlannedStartTime, "23:30") || task.DurationMinutes != 60 ||
		!strings.HasPrefix(task.DueOn, "2026-10-15") || !strings.HasPrefix(task.DueTime, "09:15") ||
		task.TimeZone != "Africa/Cairo" || task.Label != "Bills" || task.Details != "Pay before the end of the month." {
		t.Fatalf("created task contract = %#v", task)
	}
	homeResponse := request(
		ownerSession,
		http.MethodGet,
		"/api/v2/workspaces/"+workspaceID+"/home",
		"",
	)
	if homeResponse.Code != http.StatusOK {
		t.Fatalf("workspace Home status = %d, want %d: %s", homeResponse.Code, http.StatusOK, homeResponse.Body.String())
	}
	nextDayAgenda := request(
		ownerSession,
		http.MethodGet,
		"/api/v2/workspaces/"+workspaceID+"/agenda?from=2026-10-13&to=2026-10-13&timeZone=Africa/Cairo",
		"",
	)
	if nextDayAgenda.Code != http.StatusOK {
		t.Fatalf("cross-midnight agenda status = %d, want %d: %s", nextDayAgenda.Code, http.StatusOK, nextDayAgenda.Body.String())
	}
	agendaDate, _ := parseV2Date("2026-10-13")
	overlappingTasks, err := fixture.store.ListV2Tasks(fixture.ctx, workspaceID, postgres.V2TaskFilter{
		From: &agendaDate, To: &agendaDate, TimeZone: "Africa/Cairo", Limit: 10,
	})
	if err != nil {
		t.Fatalf("list overlapping V2 tasks: %v", err)
	}
	foundInStore := false
	for _, item := range overlappingTasks {
		if item.ID == task.ID {
			foundInStore = true
		}
	}
	if !foundInStore {
		t.Fatalf("store date filter omitted cross-midnight task %q: %#v", task.ID, overlappingTasks)
	}
	var nextDayItems struct {
		Tasks []struct {
			TaskID string `json:"taskId"`
		} `json:"tasks"`
	}
	if err := json.NewDecoder(nextDayAgenda.Body).Decode(&nextDayItems); err != nil {
		t.Fatalf("decode cross-midnight agenda: %v", err)
	}
	foundCrossMidnight := false
	for _, item := range nextDayItems.Tasks {
		if item.TaskID == task.ID {
			foundCrossMidnight = true
		}
	}
	if !foundCrossMidnight {
		t.Fatalf("cross-midnight task %q is missing from the following agenda date: %#v; response=%s", task.ID, nextDayItems.Tasks, nextDayAgenda.Body.String())
	}
	var sourceSchema string
	if err := fixture.pool.QueryRow(fixture.ctx, "SELECT current_schema()").Scan(&sourceSchema); err != nil {
		t.Fatalf("read disposable source schema: %v", err)
	}
	v2Schema := migrations.V2SchemaName(sourceSchema)
	legacyTaskID := "v2-legacy-task"
	if _, err := fixture.pool.Exec(fixture.ctx, `
INSERT INTO "`+v2Schema+`".tasks (
  id, workspace_id, created_by_user_id, kind, title, priority, due_on
) VALUES ($1, $2, $3, 'chore', 'Legacy chore', 'medium', '2026-10-13')`,
		legacyTaskID, workspaceID, ownerID); err != nil {
		t.Fatalf("seed legacy V2 task: %v", err)
	}
	legacyTask, err := fixture.store.GetV2Task(fixture.ctx, workspaceID, legacyTaskID)
	if err != nil {
		t.Fatalf("read legacy V2 task: %v", err)
	}
	if legacyTask.Kind != "task" || legacyTask.Priority != "normal" ||
		legacyTask.PlannedOn == nil || legacyTask.PlannedOn.Format("2006-01-02") != "2026-10-13" {
		t.Fatalf("legacy V2 task normalization = %#v", legacyTask)
	}
	legacyTasks, err := fixture.store.ListV2Tasks(fixture.ctx, workspaceID, postgres.V2TaskFilter{Kind: "task", Limit: 10})
	if err != nil {
		t.Fatalf("list canonical V2 tasks: %v", err)
	}
	foundLegacyTask := false
	for _, item := range legacyTasks {
		if item.ID == legacyTaskID {
			foundLegacyTask = item.Kind == "task" && item.Priority == "normal"
		}
	}
	if !foundLegacyTask {
		t.Fatal("canonical Task filter omitted a legacy chore row")
	}
	if role, err := fixture.store.V2WorkspaceRole(fixture.ctx, workspaceID, impostorID); err != nil || role != "member" {
		t.Fatalf("non-owner owner-role mirror = %q, err=%v; want member", role, err)
	}
	revokedMemberRead := request(
		revokedSession,
		http.MethodGet,
		"/api/v2/workspaces/"+workspaceID+"/tasks",
		"",
	)
	if revokedMemberRead.Code != http.StatusNotFound {
		t.Fatalf("revoked member V2 read status = %d, want %d: %s", revokedMemberRead.Code, http.StatusNotFound, revokedMemberRead.Body.String())
	}
	unauthorizedShareResponse := request(
		impostorSession,
		http.MethodPost,
		"/api/v2/workspaces/"+workspaceID+"/plans",
		`{"title":"Unauthorized shared plan","startsOn":"2026-10-09","endsOn":"2026-10-12","timeZone":"Africa/Cairo","visibility":"shared"}`,
	)
	if unauthorizedShareResponse.Code != http.StatusForbidden {
		t.Fatalf("non-owner share status = %d, want %d: %s", unauthorizedShareResponse.Code, http.StatusForbidden, unauthorizedShareResponse.Body.String())
	}
	inactiveAssigneeResponse := request(
		ownerSession,
		http.MethodPost,
		"/api/v2/workspaces/"+workspaceID+"/tasks",
		fmt.Sprintf(`{"kind":"task","title":"Invalid assignee","assignedToUserId":%q}`, revokedID),
	)
	if inactiveAssigneeResponse.Code != http.StatusNotFound {
		t.Fatalf("assign inactive member status = %d, want %d: %s", inactiveAssigneeResponse.Code, http.StatusNotFound, inactiveAssigneeResponse.Body.String())
	}

	today := time.Now().UTC().Truncate(24 * time.Hour)
	plannedOn := today.Format("2006-01-02")
	endsOn := today.AddDate(0, 0, 3).Format("2006-01-02")
	planResponse := request(
		ownerSession,
		http.MethodPost,
		"/api/v2/workspaces/"+workspaceID+"/plans",
		fmt.Sprintf(`{"title":"Shared plan","startsOn":%q,"endsOn":%q,"timeZone":"Africa/Cairo","visibility":"shared"}`, plannedOn, endsOn),
	)
	if planResponse.Code != http.StatusCreated {
		t.Fatalf("create shared plan status = %d, want %d: %s", planResponse.Code, http.StatusCreated, planResponse.Body.String())
	}
	var sharedPlan struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(planResponse.Body).Decode(&sharedPlan); err != nil {
		t.Fatalf("decode shared plan: %v", err)
	}

	personalPlanResponse := request(
		ownerSession,
		http.MethodPost,
		"/api/v2/workspaces/"+workspaceID+"/plans",
		fmt.Sprintf(`{"title":"Personal plan","startsOn":%q,"endsOn":%q,"timeZone":"Africa/Cairo","visibility":"personal"}`, plannedOn, endsOn),
	)
	if personalPlanResponse.Code != http.StatusCreated {
		t.Fatalf("create personal plan status = %d, want %d: %s", personalPlanResponse.Code, http.StatusCreated, personalPlanResponse.Body.String())
	}
	var personalPlan struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(personalPlanResponse.Body).Decode(&personalPlan); err != nil {
		t.Fatalf("decode personal plan: %v", err)
	}

	entryResponse := request(
		ownerSession,
		http.MethodPost,
		"/api/v2/workspaces/"+workspaceID+"/plans/"+sharedPlan.ID+"/entries",
		fmt.Sprintf(`{"taskId":%q,"plannedOn":%q,"position":0}`, task.ID, plannedOn),
	)
	if entryResponse.Code != http.StatusCreated {
		t.Fatalf("create plan entry status = %d, want %d: %s", entryResponse.Code, http.StatusCreated, entryResponse.Body.String())
	}
	var entry struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(entryResponse.Body).Decode(&entry); err != nil {
		t.Fatalf("decode plan entry: %v", err)
	}

	assignResponse := request(
		ownerSession,
		http.MethodPatch,
		"/api/v2/workspaces/"+workspaceID+"/plans/"+sharedPlan.ID+"/entries/"+entry.ID,
		fmt.Sprintf(`{"assignedToUserId":%q}`, memberID),
	)
	if assignResponse.Code != http.StatusOK {
		t.Fatalf("assign task from plan status = %d, want %d: %s", assignResponse.Code, http.StatusOK, assignResponse.Body.String())
	}

	updatedTaskResponse := request(
		ownerSession,
		http.MethodGet,
		"/api/v2/workspaces/"+workspaceID+"/tasks/"+task.ID,
		"",
	)
	if updatedTaskResponse.Code != http.StatusOK {
		t.Fatalf("read assigned task status = %d, want %d: %s", updatedTaskResponse.Code, http.StatusOK, updatedTaskResponse.Body.String())
	}
	if err := json.NewDecoder(updatedTaskResponse.Body).Decode(&task); err != nil {
		t.Fatalf("decode assigned task: %v", err)
	}
	if task.AssignedToUserID == nil || *task.AssignedToUserID != memberID {
		t.Fatalf("canonical task assignee = %v, want %q", task.AssignedToUserID, memberID)
	}

	occurrenceEnd := today.AddDate(0, 0, 2).Format("2006-01-02")
	recurringTaskResponse := request(
		ownerSession,
		http.MethodPost,
		"/api/v2/workspaces/"+workspaceID+"/tasks",
		fmt.Sprintf(
			`{"kind":"habit","title":"Daily walk","status":"open","priority":"none","recurrence":{"frequency":"daily","interval":1,"startsOn":%q,"endsOn":%q,"timeZone":"Africa/Cairo"}}`,
			plannedOn,
			occurrenceEnd,
		),
	)
	if recurringTaskResponse.Code != http.StatusCreated {
		t.Fatalf("create recurring task status = %d, want %d: %s", recurringTaskResponse.Code, http.StatusCreated, recurringTaskResponse.Body.String())
	}
	var recurringTask struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(recurringTaskResponse.Body).Decode(&recurringTask); err != nil {
		t.Fatalf("decode recurring task: %v", err)
	}
	occurrencesResponse := request(
		ownerSession,
		http.MethodGet,
		"/api/v2/workspaces/"+workspaceID+"/tasks/"+recurringTask.ID+"/occurrences?from="+plannedOn+"&to="+occurrenceEnd,
		"",
	)
	if occurrencesResponse.Code != http.StatusOK {
		t.Fatalf("list task occurrences status = %d, want %d: %s", occurrencesResponse.Code, http.StatusOK, occurrencesResponse.Body.String())
	}
	var occurrences []struct {
		Status  string `json:"status"`
		History []any  `json:"history"`
	}
	if err := json.NewDecoder(occurrencesResponse.Body).Decode(&occurrences); err != nil {
		t.Fatalf("decode task occurrences: %v", err)
	}
	if len(occurrences) != 3 || occurrences[0].Status != "open" {
		t.Fatalf("generated occurrences = %#v, want three open daily occurrences", occurrences)
	}
	occurrencePath := "/api/v2/workspaces/" + workspaceID + "/tasks/" + recurringTask.ID + "/occurrences/" + plannedOn
	for attempt := 0; attempt < 2; attempt++ {
		completionResponse := request(ownerSession, http.MethodPatch, occurrencePath, `{"status":"done"}`)
		if completionResponse.Code != http.StatusOK {
			t.Fatalf("complete occurrence attempt %d status = %d, want %d: %s", attempt+1, completionResponse.Code, http.StatusOK, completionResponse.Body.String())
		}
	}
	reopenResponse := request(ownerSession, http.MethodPatch, occurrencePath, `{"status":"open"}`)
	if reopenResponse.Code != http.StatusOK {
		t.Fatalf("reopen occurrence status = %d, want %d: %s", reopenResponse.Code, http.StatusOK, reopenResponse.Body.String())
	}
	if err := json.NewDecoder(reopenResponse.Body).Decode(&occurrences[0]); err != nil {
		t.Fatalf("decode reopened occurrence: %v", err)
	}
	if occurrences[0].Status != "open" || len(occurrences[0].History) != 2 {
		t.Fatalf("reopened occurrence = %#v, want open with two distinct recorded changes", occurrences[0])
	}

	archiveActionResponse := request(
		ownerSession,
		http.MethodPost,
		"/api/v2/workspaces/"+workspaceID+"/tasks",
		`{"kind":"task","title":"Archive task check","status":"open","priority":"none","timeZone":"Africa/Cairo"}`,
	)
	if archiveActionResponse.Code != http.StatusCreated {
		t.Fatalf("create task for archive check status = %d, want %d: %s", archiveActionResponse.Code, http.StatusCreated, archiveActionResponse.Body.String())
	}
	var archiveAction struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(archiveActionResponse.Body).Decode(&archiveAction); err != nil {
		t.Fatalf("decode action for archive alias: %v", err)
	}
	archiveResponse := request(
		ownerSession,
		http.MethodDelete,
		"/api/v2/workspaces/"+workspaceID+"/tasks/"+archiveAction.ID,
		"",
	)
	if archiveResponse.Code != http.StatusNoContent {
		t.Fatalf("archive action status = %d, want %d: %s", archiveResponse.Code, http.StatusNoContent, archiveResponse.Body.String())
	}

	dateFilteredTasksResponse := request(
		ownerSession,
		http.MethodGet,
		"/api/v2/workspaces/"+workspaceID+"/tasks?from="+plannedOn+"&to="+occurrenceEnd,
		"",
	)
	if dateFilteredTasksResponse.Code != http.StatusOK {
		t.Fatalf("list tasks by date range status = %d, want %d: %s", dateFilteredTasksResponse.Code, http.StatusOK, dateFilteredTasksResponse.Body.String())
	}
	var dateFilteredTasks []struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(dateFilteredTasksResponse.Body).Decode(&dateFilteredTasks); err != nil {
		t.Fatalf("decode date-filtered tasks: %v", err)
	}
	foundRecurringTask := false
	for _, item := range dateFilteredTasks {
		if item.ID == recurringTask.ID {
			foundRecurringTask = true
		}
	}
	if !foundRecurringTask {
		t.Fatalf("date-filtered task list did not include recurring task %q: %#v", recurringTask.ID, dateFilteredTasks)
	}

	eventStart := today.Add(10 * time.Hour)
	eventEnd := eventStart.Add(time.Hour)
	if _, err := fixture.pool.Exec(
		fixture.ctx,
		`INSERT INTO "`+v2Schema+`".calendar_events (
  id, workspace_id, created_by_user_id, updated_by_user_id, title,
  starts_at, ends_at, all_day, time_zone, source_provider, source_event_id
) VALUES ($1,$2,$3,$3,$4,$5,$6,false,'UTC','google',$7)`,
		"v2-provider-event",
		workspaceID,
		ownerID,
		"Provider event",
		eventStart,
		eventEnd,
		"google-event-42",
	); err != nil {
		t.Fatalf("seed disposable provider event: %v", err)
	}
	agendaResponse := request(
		ownerSession,
		http.MethodGet,
		"/api/v2/workspaces/"+workspaceID+"/agenda?from="+plannedOn+"&to="+occurrenceEnd+"&timeZone=UTC",
		"",
	)
	if agendaResponse.Code != http.StatusOK {
		t.Fatalf("read agenda projection status = %d, want %d: %s", agendaResponse.Code, http.StatusOK, agendaResponse.Body.String())
	}
	var agenda struct {
		From          string `json:"from"`
		To            string `json:"to"`
		TimeZone      string `json:"timeZone"`
		NextPageToken string `json:"nextPageToken"`
		Tasks         []struct {
			Kind   string `json:"kind"`
			TaskID string `json:"taskId"`
		} `json:"tasks"`
		PlanEntries []json.RawMessage `json:"planEntries"`
		Events      []struct {
			ID             string `json:"id"`
			Kind           string `json:"kind"`
			SourceProvider string `json:"sourceProvider"`
			SourceEventID  string `json:"sourceEventId"`
		} `json:"calendarEvents"`
	}
	if err := json.NewDecoder(agendaResponse.Body).Decode(&agenda); err != nil {
		t.Fatalf("decode agenda projection: %v", err)
	}
	if agenda.TimeZone != "UTC" || agenda.NextPageToken != "" {
		t.Fatalf("agenda time zone/token = %q/%q, want UTC and no next page", agenda.TimeZone, agenda.NextPageToken)
	}
	if agendaResponse.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatalf("agenda Cache-Control = %q, want private, no-store", agendaResponse.Header().Get("Cache-Control"))
	}
	recurringItems := 0
	for _, item := range agenda.Tasks {
		if item.Kind == "task_occurrence" && item.TaskID == recurringTask.ID {
			recurringItems++
		}
	}
	if recurringItems != 3 {
		t.Fatalf("agenda recurring occurrences = %d, want 3: %#v", recurringItems, agenda.Tasks)
	}
	if len(agenda.PlanEntries) != 1 {
		t.Fatalf("agenda plan entries = %d, want 1", len(agenda.PlanEntries))
	}
	if len(agenda.Events) != 1 || agenda.Events[0].Kind != "calendar_event" ||
		agenda.Events[0].SourceProvider != "google" || agenda.Events[0].SourceEventID != "google-event-42" {
		t.Fatalf("agenda calendar event projection = %#v, want the distinct provider event and source ID", agenda.Events)
	}
	allSpacesResponse := request(
		ownerSession,
		http.MethodGet,
		"/api/v2/agenda?from="+plannedOn+"&to="+occurrenceEnd+"&timeZone=UTC",
		"",
	)
	if allSpacesResponse.Code != http.StatusOK {
		t.Fatalf("read All Spaces agenda status = %d, want %d: %s", allSpacesResponse.Code, http.StatusOK, allSpacesResponse.Body.String())
	}
	var allSpacesAgenda struct {
		WorkspaceID string `json:"workspaceId"`
		Tasks       []struct {
			TaskID        string `json:"taskId"`
			WorkspaceID   string `json:"workspaceId"`
			WorkspaceName string `json:"workspaceName"`
		} `json:"tasks"`
		PlanEntries []struct {
			WorkspaceID   string `json:"workspaceId"`
			WorkspaceName string `json:"workspaceName"`
		} `json:"planEntries"`
		Events []struct {
			ID            string `json:"id"`
			WorkspaceID   string `json:"workspaceId"`
			WorkspaceName string `json:"workspaceName"`
		} `json:"calendarEvents"`
	}
	if err := json.NewDecoder(allSpacesResponse.Body).Decode(&allSpacesAgenda); err != nil {
		t.Fatalf("decode All Spaces agenda: %v", err)
	}
	if allSpacesAgenda.WorkspaceID != "all" {
		t.Fatalf("All Spaces agenda workspaceId = %q, want all", allSpacesAgenda.WorkspaceID)
	}
	foundLabeledTask, foundLabeledEvent := false, false
	for _, item := range allSpacesAgenda.Tasks {
		if item.TaskID == recurringTask.ID {
			foundLabeledTask = item.WorkspaceID == workspaceID && item.WorkspaceName != ""
		}
	}
	for _, item := range allSpacesAgenda.Events {
		if item.ID == "v2-provider-event" {
			foundLabeledEvent = item.WorkspaceID == workspaceID && item.WorkspaceName != ""
		}
	}
	if !foundLabeledTask || !foundLabeledEvent {
		t.Fatalf("All Spaces agenda did not label authorized Space items: task=%t event=%t", foundLabeledTask, foundLabeledEvent)
	}
	if len(allSpacesAgenda.PlanEntries) != 1 ||
		allSpacesAgenda.PlanEntries[0].WorkspaceID != workspaceID ||
		allSpacesAgenda.PlanEntries[0].WorkspaceName == "" {
		t.Fatalf("All Spaces plan entries were not labeled with their Space: %#v", allSpacesAgenda.PlanEntries)
	}

	monthTo := today.AddDate(0, 0, v2MaxAgendaDateRange-1).Format("2006-01-02")
	monthAgendaResponse := request(
		ownerSession,
		http.MethodGet,
		"/api/v2/workspaces/"+workspaceID+"/agenda?from="+today.Format("2006-01-02")+"&to="+monthTo+"&timeZone=Africa/Cairo",
		"",
	)
	if monthAgendaResponse.Code != http.StatusOK {
		t.Fatalf("read 42-day agenda status = %d, want %d: %s", monthAgendaResponse.Code, http.StatusOK, monthAgendaResponse.Body.String())
	}
	var monthAgenda struct {
		From     string `json:"from"`
		To       string `json:"to"`
		TimeZone string `json:"timeZone"`
	}
	if err := json.NewDecoder(monthAgendaResponse.Body).Decode(&monthAgenda); err != nil {
		t.Fatalf("decode 42-day agenda: %v", err)
	}
	if monthAgenda.From != today.Format("2006-01-02") || monthAgenda.To != monthTo || monthAgenda.TimeZone != "Africa/Cairo" {
		t.Fatalf("42-day agenda boundaries = %q..%q %q, want inclusive range %q..%q in Africa/Cairo",
			monthAgenda.From, monthAgenda.To, monthAgenda.TimeZone, today.Format("2006-01-02"), monthTo)
	}
	outOfRangeAgendaResponse := request(
		ownerSession,
		http.MethodGet,
		"/api/v2/workspaces/"+workspaceID+"/agenda?from="+today.Format("2006-01-02")+"&to="+today.AddDate(0, 0, v2MaxAgendaDateRange).Format("2006-01-02")+"&timeZone=UTC",
		"",
	)
	if outOfRangeAgendaResponse.Code != http.StatusBadRequest {
		t.Fatalf("read 43-day agenda status = %d, want %d: %s", outOfRangeAgendaResponse.Code, http.StatusBadRequest, outOfRangeAgendaResponse.Body.String())
	}

	overlapStart := today.AddDate(0, 0, -1).Add(22*time.Hour + 30*time.Minute)
	overlapEnd := overlapStart.Add(2 * time.Hour)
	if _, err := fixture.pool.Exec(
		fixture.ctx,
		`INSERT INTO "`+v2Schema+`".calendar_events (
  id, workspace_id, created_by_user_id, updated_by_user_id, title,
  starts_at, ends_at, all_day, time_zone, source_provider, source_event_id
) VALUES ($1,$2,$3,$3,$4,$5,$6,false,'UTC','google',$7)`,
		"v2-overlap-event",
		workspaceID,
		ownerID,
		"Event crossing the viewer's day boundary",
		overlapStart,
		overlapEnd,
		"google-event-overlap",
	); err != nil {
		t.Fatalf("seed overlapping provider event: %v", err)
	}
	viewerDayResponse := request(
		ownerSession,
		http.MethodGet,
		"/api/v2/workspaces/"+workspaceID+"/agenda?from="+today.Format("2006-01-02")+"&to="+today.Format("2006-01-02")+"&timeZone=Africa/Cairo",
		"",
	)
	if viewerDayResponse.Code != http.StatusOK {
		t.Fatalf("read viewer-local day agenda status = %d, want %d: %s", viewerDayResponse.Code, http.StatusOK, viewerDayResponse.Body.String())
	}
	var viewerDayAgenda struct {
		Events []struct {
			ID string `json:"id"`
		} `json:"calendarEvents"`
	}
	if err := json.NewDecoder(viewerDayResponse.Body).Decode(&viewerDayAgenda); err != nil {
		t.Fatalf("decode viewer-local day agenda: %v", err)
	}
	foundOverlap := false
	for _, event := range viewerDayAgenda.Events {
		if event.ID == "v2-overlap-event" {
			foundOverlap = true
		}
	}
	if !foundOverlap {
		t.Fatalf("viewer-local agenda omitted an event that overlaps its day: %#v", viewerDayAgenda.Events)
	}

	if _, err := fixture.pool.Exec(fixture.ctx, `
INSERT INTO "`+v2Schema+`".tasks (
  id, workspace_id, created_by_user_id, kind, title, due_on
)
	SELECT 'agenda-pagination-task-' || n::text, $1, $2, 'task',
       'Agenda pagination task ' || n::text, $3::date
FROM generate_series(1, 101) AS n`,
		workspaceID, ownerID, today.AddDate(0, 0, 35)); err != nil {
		t.Fatalf("seed tasks for agenda pagination: %v", err)
	}
	seenPageTasks := make(map[string]struct{}, 101)
	pageToken := ""
	paginationComplete := false
	for page := 0; page < 10; page++ {
		pageURL := "/api/v2/workspaces/" + workspaceID + "/agenda?from=" +
			today.Format("2006-01-02") + "&to=" + monthTo + "&timeZone=UTC"
		if pageToken != "" {
			pageURL += "&pageToken=" + pageToken
		}
		pageResponse := request(ownerSession, http.MethodGet, pageURL, "")
		if pageResponse.Code != http.StatusOK {
			t.Fatalf("read agenda page %d status = %d, want %d: %s", page, pageResponse.Code, http.StatusOK, pageResponse.Body.String())
		}
		var pageAgenda struct {
			Tasks []struct {
				TaskID string `json:"taskId"`
			} `json:"tasks"`
			NextPageToken string `json:"nextPageToken"`
		}
		if err := json.NewDecoder(pageResponse.Body).Decode(&pageAgenda); err != nil {
			t.Fatalf("decode agenda page %d: %v", page, err)
		}
		for _, item := range pageAgenda.Tasks {
			if strings.HasPrefix(item.TaskID, "agenda-pagination-task-") {
				seenPageTasks[item.TaskID] = struct{}{}
			}
		}
		pageToken = pageAgenda.NextPageToken
		if pageToken == "" {
			paginationComplete = true
			break
		}
	}
	if !paginationComplete || len(seenPageTasks) != 101 {
		t.Fatalf("agenda pagination complete=%t, returned %d of 101 seeded tasks", paginationComplete, len(seenPageTasks))
	}

	memberSharedPlanResponse := request(
		memberSession,
		http.MethodGet,
		"/api/v2/workspaces/"+workspaceID+"/plans/"+sharedPlan.ID,
		"",
	)
	if memberSharedPlanResponse.Code != http.StatusOK {
		t.Fatalf("member read shared plan status = %d, want %d: %s", memberSharedPlanResponse.Code, http.StatusOK, memberSharedPlanResponse.Body.String())
	}
	memberPersonalPlanResponse := request(
		memberSession,
		http.MethodGet,
		"/api/v2/workspaces/"+workspaceID+"/plans/"+personalPlan.ID,
		"",
	)
	if memberPersonalPlanResponse.Code != http.StatusNotFound {
		t.Fatalf("member read personal plan status = %d, want %d: %s", memberPersonalPlanResponse.Code, http.StatusNotFound, memberPersonalPlanResponse.Body.String())
	}

	reminderResponse := request(
		ownerSession,
		http.MethodPost,
		"/api/v2/workspaces/"+workspaceID+"/reminder-rules",
		fmt.Sprintf(`{"kind":"task","rule":{"targetId":%q,"minutesBefore":15}}`, task.ID),
	)
	if reminderResponse.Code != http.StatusCreated {
		t.Fatalf("create inactive reminder status = %d, want %d: %s", reminderResponse.Code, http.StatusCreated, reminderResponse.Body.String())
	}
	var reminder postgres.V2ReminderRule
	if err := json.NewDecoder(reminderResponse.Body).Decode(&reminder); err != nil {
		t.Fatalf("decode reminder rule: %v", err)
	}
	if reminder.Status != "pending_automation" {
		t.Fatalf("new reminder status = %q, want pending_automation", reminder.Status)
	}

	invalidGoalResponse := request(
		ownerSession,
		http.MethodPost,
		"/api/v2/workspaces/"+workspaceID+"/goals",
		`{"title":"Invalid progress","progressPercent":65536}`,
	)
	if invalidGoalResponse.Code != http.StatusBadRequest {
		t.Fatalf("overflowing goal progress status = %d, want %d: %s", invalidGoalResponse.Code, http.StatusBadRequest, invalidGoalResponse.Body.String())
	}

	if role, err := fixture.store.V2WorkspaceRole(fixture.ctx, workspaceID, ownerID); err != nil || role != "owner" {
		t.Fatalf("mirrored workspace owner role = %q, err=%v; want owner", role, err)
	}
}

func TestV2CalendarEventCRUD(t *testing.T) {
	fixture := openCreditPolicyIntegrationFixture(t)
	const userID, workspaceID = "v2-calendar-owner", "v2-calendar-workspace"
	sessionID := fixture.createUserAndSession(t, userID, "v2-calendar@example.test", true)
	if err := fixture.store.CreateWorkspace(fixture.ctx, workspaceID, "Calendar", userID, []string{"workspace:manage"}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	handler := NewHandler(config.Config{
		AuthRateLimitHMACSecret: "test-auth-rate-limit-hmac-secret-32-bytes",
	}, fixture.store, slog.Default(), "askolo_session").Routes()
	request := func(method, path, body string) *httptest.ResponseRecorder {
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

	payload := `{"title":"Dentist","description":"Annual checkup","startsAt":"2026-10-14T09:00:00Z","endsAt":"2026-10-14T10:00:00Z","allDay":false,"timeZone":"Africa/Cairo","location":"Clinic","color":"#123456"}`
	created := request(http.MethodPost, "/api/v2/workspaces/"+workspaceID+"/calendar-events", payload)
	if created.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want %d: %s", created.Code, http.StatusCreated, created.Body.String())
	}
	var event postgres.V2CalendarEventRecord
	if err := json.NewDecoder(created.Body).Decode(&event); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if event.ID == "" || event.Title != "Dentist" || event.TimeZone != "Africa/Cairo" ||
		event.Location == nil || *event.Location != "Clinic" || event.Color == nil || *event.Color != "#123456" {
		t.Fatalf("created event = %#v", event)
	}

	query := "?from=2026-10-14&to=2026-10-14&timeZone=Africa%2FCairo"
	listed := request(http.MethodGet, "/api/v2/workspaces/"+workspaceID+"/calendar-events"+query, "")
	if listed.Code != http.StatusOK {
		t.Fatalf("list status = %d: %s", listed.Code, listed.Body.String())
	}
	var events []postgres.V2CalendarEventRecord
	if err := json.NewDecoder(listed.Body).Decode(&events); err != nil || len(events) != 1 || events[0].ID != event.ID {
		t.Fatalf("listed events = %#v, decode error = %v", events, err)
	}

	updated := request(http.MethodPatch, "/api/v2/workspaces/"+workspaceID+"/calendar-events/"+event.ID,
		`{"title":"Dentist follow-up","description":null,"startsAt":"2026-10-14T11:00:00Z","endsAt":null,"allDay":false,"timeZone":"Africa/Cairo","location":null,"color":null}`)
	if updated.Code != http.StatusOK {
		t.Fatalf("update status = %d: %s", updated.Code, updated.Body.String())
	}
	if err := json.NewDecoder(updated.Body).Decode(&event); err != nil || event.Title != "Dentist follow-up" || event.Description != nil {
		t.Fatalf("updated event = %#v, decode error = %v", event, err)
	}

	deleted := request(http.MethodDelete, "/api/v2/workspaces/"+workspaceID+"/calendar-events/"+event.ID, "")
	if deleted.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, want %d: %s", deleted.Code, http.StatusNoContent, deleted.Body.String())
	}
	listed = request(http.MethodGet, "/api/v2/workspaces/"+workspaceID+"/calendar-events"+query, "")
	if listed.Code != http.StatusOK {
		t.Fatalf("list after delete status = %d: %s", listed.Code, listed.Body.String())
	}
	events = nil
	if err := json.NewDecoder(listed.Body).Decode(&events); err != nil || len(events) != 0 {
		t.Fatalf("events after delete = %#v, decode error = %v", events, err)
	}

	providerEvent := postgres.V2ProviderCalendarEvent{
		ProviderConnectionID: "google-connection-preserved",
		ProviderCalendarID:   "primary-calendar-preserved",
		ProviderEventID:      "google-event-preserved",
		Title:                "Provider event",
		StartsAt:             time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC),
		TimeZone:             "UTC",
	}
	if err := fixture.store.UpsertV2GoogleCalendarEvents(fixture.ctx, workspaceID, userID, []postgres.V2ProviderCalendarEvent{providerEvent}); err != nil {
		t.Fatalf("upsert provider event: %v", err)
	}
	listed = request(http.MethodGet, "/api/v2/workspaces/"+workspaceID+"/calendar-events"+query, "")
	if listed.Code != http.StatusOK {
		t.Fatalf("list provider event status = %d: %s", listed.Code, listed.Body.String())
	}
	events = nil
	if err := json.NewDecoder(listed.Body).Decode(&events); err != nil || len(events) != 1 {
		t.Fatalf("provider events = %#v, decode error = %v", events, err)
	}
	providerRecord := events[0]
	if providerRecord.SourceProvider == nil || *providerRecord.SourceProvider != "google" ||
		providerRecord.SourceConnectionID == nil || *providerRecord.SourceConnectionID != providerEvent.ProviderConnectionID ||
		providerRecord.SourceCalendarID == nil || *providerRecord.SourceCalendarID != providerEvent.ProviderCalendarID ||
		providerRecord.SourceEventID == nil || *providerRecord.SourceEventID != providerEvent.ProviderEventID {
		t.Fatalf("provider source identity was not preserved: %#v", providerRecord)
	}
	readOnlyUpdate := request(http.MethodPatch, "/api/v2/workspaces/"+workspaceID+"/calendar-events/"+providerRecord.ID,
		`{"title":"Do not overwrite","startsAt":"2026-10-14T13:00:00Z","allDay":false,"timeZone":"UTC"}`)
	if readOnlyUpdate.Code != http.StatusNotFound {
		t.Fatalf("provider-owned event update status = %d, want %d", readOnlyUpdate.Code, http.StatusNotFound)
	}
	providerEvent.Title = "Provider event updated"
	if err := fixture.store.UpsertV2GoogleCalendarEvents(fixture.ctx, workspaceID, userID, []postgres.V2ProviderCalendarEvent{providerEvent}); err != nil {
		t.Fatalf("upsert updated provider event: %v", err)
	}
	listed = request(http.MethodGet, "/api/v2/workspaces/"+workspaceID+"/calendar-events"+query, "")
	if listed.Code != http.StatusOK {
		t.Fatalf("list updated provider event status = %d: %s", listed.Code, listed.Body.String())
	}
	events = nil
	if err := json.NewDecoder(listed.Body).Decode(&events); err != nil || len(events) != 1 ||
		events[0].ID != providerRecord.ID || events[0].Title != providerEvent.Title ||
		events[0].SourceConnectionID == nil || *events[0].SourceConnectionID != providerEvent.ProviderConnectionID ||
		events[0].SourceCalendarID == nil || *events[0].SourceCalendarID != providerEvent.ProviderCalendarID ||
		events[0].SourceEventID == nil || *events[0].SourceEventID != providerEvent.ProviderEventID {
		t.Fatalf("provider identity/upsert result = %#v, decode error = %v", events, err)
	}
}
