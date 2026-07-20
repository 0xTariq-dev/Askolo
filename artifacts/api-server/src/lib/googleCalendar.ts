import { ReplitConnectors } from "@replit/connectors-sdk";

const connectors = new ReplitConnectors();

const GOOGLE_CALENDAR_ID = "google-calendar";

interface GoogleCalendarEvent {
  id: string;
  summary?: string;
  description?: string | null;
  start?: {
    date?: string;
    dateTime?: string;
    timeZone?: string;
  };
  end?: {
    date?: string;
    dateTime?: string;
    timeZone?: string;
  };
  location?: string;
  attendees?: Array<{ email?: string }>;
}

export async function getGoogleCalendarList() {
  const res = await connectors.proxy(GOOGLE_CALENDAR_ID, "/users/me/calendarList", { method: "GET" });
  if (!res.ok) throw new Error(`Calendar list failed: ${res.status}`);
  return (await res.json()) as { items?: Array<{ id: string; summary: string; primary?: boolean }> };
}

export async function listGoogleCalendarEvents(calendarId: string, timeMin: string, timeMax: string) {
  const encodedId = encodeURIComponent(calendarId);
  const params = new URLSearchParams({ timeMin, timeMax, singleEvents: "true", orderBy: "startTime" });
  const res = await connectors.proxy(
    GOOGLE_CALENDAR_ID,
    `/calendars/${encodedId}/events?${params.toString()}`,
    { method: "GET" },
  );
  if (!res.ok) throw new Error(`Events fetch failed: ${res.status}`);
  return (await res.json()) as { items?: GoogleCalendarEvent[] };
}

export async function createGoogleCalendarEvent(calendarId: string, event: Partial<GoogleCalendarEvent>) {
  const encodedId = encodeURIComponent(calendarId);
  const res = await connectors.proxy(GOOGLE_CALENDAR_ID, `/calendars/${encodedId}/events`, {
    method: "POST",
    body: JSON.stringify(event),
    headers: { "Content-Type": "application/json" },
  });
  if (!res.ok) throw new Error(`Event create failed: ${res.status}`);
  return (await res.json()) as GoogleCalendarEvent;
}

export async function updateGoogleCalendarEvent(
  calendarId: string,
  eventId: string,
  event: Partial<GoogleCalendarEvent>,
) {
  const encodedCalendarId = encodeURIComponent(calendarId);
  const encodedEventId = encodeURIComponent(eventId);
  const res = await connectors.proxy(
    GOOGLE_CALENDAR_ID,
    `/calendars/${encodedCalendarId}/events/${encodedEventId}`,
    {
      method: "PATCH",
      body: JSON.stringify(event),
      headers: { "Content-Type": "application/json" },
    },
  );
  if (!res.ok) throw new Error(`Event update failed: ${res.status}`);
  return (await res.json()) as GoogleCalendarEvent;
}

export async function deleteGoogleCalendarEvent(calendarId: string, eventId: string) {
  const encodedCalendarId = encodeURIComponent(calendarId);
  const encodedEventId = encodeURIComponent(eventId);
  const res = await connectors.proxy(
    GOOGLE_CALENDAR_ID,
    `/calendars/${encodedCalendarId}/events/${encodedEventId}`,
    { method: "DELETE" },
  );
  if (!res.ok) throw new Error(`Event delete failed: ${res.status}`);
  return res.ok;
}

export function formatGoogleEventToLocal(event: GoogleCalendarEvent) {
  const start = event.start?.dateTime || event.start?.date;
  const end = event.end?.dateTime || event.end?.date;
  const startDate = event.start?.date || (event.start?.dateTime ? event.start.dateTime.slice(0, 10) : "");
  const startTime = event.start?.dateTime ? event.start.dateTime.slice(11, 16) : undefined;
  const endDate = event.end?.date || (event.end?.dateTime ? event.end.dateTime.slice(0, 10) : undefined);
  const endTime = event.end?.dateTime ? event.end.dateTime.slice(11, 16) : undefined;

  return {
    googleEventId: event.id,
    title: event.summary || "Untitled event",
    description: event.description || null,
    startDate,
    startTime,
    endDate,
    endTime,
    allDay: !!event.start?.date && !event.start?.dateTime,
    location: event.location || null,
    attendees: event.attendees?.map((a) => a.email).filter(Boolean).join(", ") || null,
  };
}
