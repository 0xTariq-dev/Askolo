import { calendarApiRequest } from "./googleOAuth";

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

export async function getGoogleCalendarList(userId: string) {
  const res = await calendarApiRequest(userId, "/users/me/calendarList", { method: "GET" });
  return (await res.json()) as { items?: Array<{ id: string; summary: string; primary?: boolean }> };
}

export async function verifyGoogleCalendarConnection(userId: string): Promise<boolean> {
  try {
    const res = await calendarApiRequest(userId, "/users/me/calendarList", { method: "GET" });
    return res.ok;
  } catch {
    return false;
  }
}

export async function listGoogleCalendarEvents(userId: string, calendarId: string, timeMin: string, timeMax: string) {
  const encodedId = encodeURIComponent(calendarId);
  const params = new URLSearchParams({ timeMin, timeMax, singleEvents: "true", orderBy: "startTime" });
  const res = await calendarApiRequest(
    userId,
    `/calendars/${encodedId}/events?${params.toString()}`,
    { method: "GET" },
  );
  return (await res.json()) as { items?: GoogleCalendarEvent[] };
}

export async function createGoogleCalendarEvent(
  userId: string,
  calendarId: string,
  event: Partial<GoogleCalendarEvent>,
) {
  const encodedId = encodeURIComponent(calendarId);
  const res = await calendarApiRequest(userId, `/calendars/${encodedId}/events`, {
    method: "POST",
    body: JSON.stringify(event),
    headers: { "Content-Type": "application/json" },
  });
  return (await res.json()) as GoogleCalendarEvent;
}

export async function updateGoogleCalendarEvent(
  userId: string,
  calendarId: string,
  eventId: string,
  event: Partial<GoogleCalendarEvent>,
) {
  const encodedCalendarId = encodeURIComponent(calendarId);
  const encodedEventId = encodeURIComponent(eventId);
  const res = await calendarApiRequest(
    userId,
    `/calendars/${encodedCalendarId}/events/${encodedEventId}`,
    {
      method: "PATCH",
      body: JSON.stringify(event),
      headers: { "Content-Type": "application/json" },
    },
  );
  return (await res.json()) as GoogleCalendarEvent;
}

export async function deleteGoogleCalendarEvent(userId: string, calendarId: string, eventId: string) {
  const encodedCalendarId = encodeURIComponent(calendarId);
  const encodedEventId = encodeURIComponent(eventId);
  const res = await calendarApiRequest(
    userId,
    `/calendars/${encodedCalendarId}/events/${encodedEventId}`,
    { method: "DELETE" },
  );
  return res.ok;
}

export function formatGoogleEventToLocal(event: GoogleCalendarEvent) {
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

export interface LocalEventInput {
  title?: string;
  description?: string | null;
  startDate?: string;
  startTime?: string | null;
  endDate?: string | null;
  endTime?: string | null;
  allDay?: boolean;
  location?: string | null;
  attendees?: string | null;
}

export function toGoogleCalendarEvent(input: LocalEventInput): Partial<GoogleCalendarEvent> {
  const event: Partial<GoogleCalendarEvent> = {};

  if (input.title !== undefined) event.summary = input.title;
  if (input.description !== undefined) event.description = input.description;
  if (input.location !== undefined) event.location = input.location || undefined;
  if (input.attendees !== undefined) {
    event.attendees = input.attendees
      ? input.attendees.split(",").map((e) => e.trim()).filter(Boolean).map((email) => ({ email }))
      : undefined;
  }

  if (input.allDay) {
    event.start = { date: input.startDate || new Date().toISOString().split("T")[0] };
    event.end = { date: input.endDate || input.startDate || new Date().toISOString().split("T")[0] };
  } else if (input.startDate) {
    const startDateTime = input.startTime
      ? `${input.startDate}T${input.startTime}:00`
      : `${input.startDate}T00:00:00`;
    const endDateTime = input.endDate && input.endTime
      ? `${input.endDate}T${input.endTime}:00`
      : input.startTime
        ? `${input.startDate}T${input.startTime}:00`
        : `${input.startDate}T01:00:00`;
    event.start = { dateTime: startDateTime };
    event.end = { dateTime: endDateTime };
  }

  return event;
}
