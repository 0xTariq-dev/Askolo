package product

import (
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"askolo/backend/internal/adapters/postgres"
)

var v2CalendarEventIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

type v2CalendarEventRequest struct {
	Title       string     `json:"title"`
	Description *string    `json:"description"`
	StartsAt    time.Time  `json:"startsAt"`
	EndsAt      *time.Time `json:"endsAt"`
	AllDay      bool       `json:"allDay"`
	TimeZone    string     `json:"timeZone"`
	Location    *string    `json:"location"`
	Color       *string    `json:"color"`
}

func buildV2CalendarEventWrite(request v2CalendarEventRequest) (postgres.V2CalendarEventWrite, error) {
	request.Title = strings.TrimSpace(request.Title)
	if request.Title == "" || len(request.Title) > 240 {
		return postgres.V2CalendarEventWrite{}, errors.New("title must contain 1 to 240 characters")
	}
	if request.StartsAt.IsZero() {
		return postgres.V2CalendarEventWrite{}, errors.New("startsAt is required")
	}
	if request.EndsAt != nil && request.EndsAt.Before(request.StartsAt) {
		return postgres.V2CalendarEventWrite{}, errors.New("endsAt must not be before startsAt")
	}
	if len(request.TimeZone) > 80 || !validV2TimeZone(request.TimeZone) {
		return postgres.V2CalendarEventWrite{}, errors.New("timeZone must be a valid IANA time zone")
	}
	if request.Description != nil && len(*request.Description) > 4000 {
		return postgres.V2CalendarEventWrite{}, errors.New("description must not exceed 4000 characters")
	}
	if request.Location != nil && len(*request.Location) > 500 {
		return postgres.V2CalendarEventWrite{}, errors.New("location must not exceed 500 characters")
	}
	if request.Color != nil && (len(*request.Color) > 32 || strings.TrimSpace(*request.Color) == "") {
		return postgres.V2CalendarEventWrite{}, errors.New("color is invalid")
	}
	return postgres.V2CalendarEventWrite{
		Title: request.Title, Description: request.Description, StartsAt: request.StartsAt.UTC(),
		EndsAt: request.EndsAt, AllDay: request.AllDay, TimeZone: request.TimeZone,
		Location: request.Location, Color: request.Color,
	}, nil
}

func (h *Handler) v2ListCalendarEvents(w http.ResponseWriter, r *http.Request, userID, _ string) {
	if !v2QueryValid(r, "from", "to", "timeZone", "limit", "offset") {
		writeError(w, http.StatusBadRequest, "INVALID_QUERY", "The request contains an unsupported or repeated query parameter.")
		return
	}
	query := r.URL.Query()
	from, fromOK := parseV2Date(query.Get("from"))
	to, toOK := parseV2Date(query.Get("to"))
	zone := query.Get("timeZone")
	if !fromOK || !toOK || to.Before(from) || to.Sub(from) > (v2MaxAgendaDateRange-1)*24*time.Hour || !validV2TimeZone(zone) {
		writeError(w, http.StatusBadRequest, "INVALID_RANGE", "Provide a valid range of no more than 42 inclusive dates and a valid time zone.")
		return
	}
	limit, offset := 50, 0
	var err error
	if query.Has("limit") {
		limit, err = strconv.Atoi(query.Get("limit"))
		if err != nil || limit < 1 || limit > 100 {
			writeError(w, http.StatusBadRequest, "INVALID_PAGINATION", "Use a limit from 1 to 100 and a non-negative offset.")
			return
		}
	}
	if query.Has("offset") {
		offset, err = strconv.Atoi(query.Get("offset"))
		if err != nil || offset < 0 || offset > 100000 {
			writeError(w, http.StatusBadRequest, "INVALID_PAGINATION", "Use a limit from 1 to 100 and a non-negative offset.")
			return
		}
	}
	if limit == 0 {
		writeError(w, http.StatusBadRequest, "INVALID_PAGINATION", "Use a limit from 1 to 100 and a non-negative offset.")
		return
	}
	events, err := h.store.ListV2CalendarEvents(r.Context(), r.PathValue("workspaceId"), from, to, zone, limit, offset)
	if err != nil {
		v2HandleStoreError(w, r, h, userID, "list calendar events", err)
		return
	}
	writeJSON(w, http.StatusOK, events)
}

func (h *Handler) v2CreateCalendarEvent(w http.ResponseWriter, r *http.Request, userID, _ string) {
	var request v2CalendarEventRequest
	if !decodeV2JSON(w, r, &request) {
		return
	}
	input, err := buildV2CalendarEventWrite(request)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_EVENT", err.Error())
		return
	}
	event, err := h.store.CreateV2CalendarEvent(r.Context(), r.PathValue("workspaceId"), userID, input)
	if err != nil {
		v2HandleStoreError(w, r, h, userID, "create calendar event", err)
		return
	}
	writeJSON(w, http.StatusCreated, event)
}

func (h *Handler) v2UpdateCalendarEvent(w http.ResponseWriter, r *http.Request, userID, role string) {
	eventID := r.PathValue("eventId")
	if !v2CalendarEventIDPattern.MatchString(eventID) {
		writeError(w, http.StatusBadRequest, "INVALID_EVENT_ID", "A valid event ID is required.")
		return
	}
	var request v2CalendarEventRequest
	if !decodeV2JSON(w, r, &request) {
		return
	}
	input, err := buildV2CalendarEventWrite(request)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_EVENT", err.Error())
		return
	}
	event, err := h.store.UpdateV2CalendarEvent(r.Context(), r.PathValue("workspaceId"), eventID, userID, role, input)
	if err != nil {
		v2HandleStoreError(w, r, h, userID, "update calendar event", err)
		return
	}
	writeJSON(w, http.StatusOK, event)
}

func (h *Handler) v2DeleteCalendarEvent(w http.ResponseWriter, r *http.Request, userID, role string) {
	eventID := r.PathValue("eventId")
	if !v2CalendarEventIDPattern.MatchString(eventID) {
		writeError(w, http.StatusBadRequest, "INVALID_EVENT_ID", "A valid event ID is required.")
		return
	}
	if err := h.store.DeleteV2CalendarEvent(r.Context(), r.PathValue("workspaceId"), eventID, userID, role); err != nil {
		v2HandleStoreError(w, r, h, userID, "delete calendar event", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
