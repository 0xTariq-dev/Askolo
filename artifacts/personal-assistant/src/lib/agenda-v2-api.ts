export type V2AgendaTask = {
  kind: string;
  taskId: string;
  workspaceId?: string;
  workspaceName?: string;
  occurrenceOn: string | null;
  title: string;
  status: string;
  priority: string;
  dueOn: string | null;
  plannedStartAt: string | null;
  plannedOn: string | null;
  plannedStartTime: string | null;
  durationMinutes: number | null;
  timeZone: string;
  label: string | null;
  assignedToUserId: string | null;
  planId: string | null;
};

export type V2AgendaPlanEntry = {
  id: string;
  workspaceId: string;
  workspaceName?: string;
  planId: string;
  taskId: string;
  occurrenceOn: string | null;
  plannedOn: string;
  plannedStartAt: string | null;
  plannedEndAt: string | null;
  position: number;
  createdByUserId: string;
  assignedToUserId: string | null;
  taskTitle: string;
  taskStatus: string;
};

export type V2AgendaEvent = {
  kind: string;
  id: string;
  workspaceId?: string;
  workspaceName?: string;
  title: string;
  description: string | null;
  startsAt: string;
  endsAt: string | null;
  allDay: boolean;
  timeZone: string;
  location: string | null;
  color: string | null;
  sourceProvider: string | null;
  sourceConnectionId: string | null;
  sourceCalendarId: string | null;
  sourceEventId: string | null;
};

export type V2AgendaPage = {
  workspaceId: string;
  from: string;
  to: string;
  timeZone: string;
  tasks: V2AgendaTask[];
  planEntries: V2AgendaPlanEntry[];
  calendarEvents: V2AgendaEvent[];
  nextPageToken?: string;
};

export type V2Workspace = {
  id: string;
  name: string;
  isPersonal: boolean;
  role: string;
};

export type V2Goal = {
  id: string;
  workspaceId: string;
  createdByUserId: string;
  title: string;
  description: string | null;
  targetOn: string | null;
  status: 'active' | 'paused' | 'completed';
  progressPercent: number;
  category: string | null;
  workspaceName?: string;
};

const basePath = import.meta.env.BASE_URL.replace(/\/$/, '');

export async function getV2Workspaces(signal?: AbortSignal): Promise<V2Workspace[]> {
  const response = await fetch(`${basePath}/api/v2/workspaces`, {
    credentials: 'include',
    headers: { Accept: 'application/json' },
    signal,
  });
  if (!response.ok) {
    throw new Error(`Spaces could not be loaded (${response.status}).`);
  }
  return response.json() as Promise<V2Workspace[]>;
}

export async function getV2Goals(
  workspaceId: string,
  signal?: AbortSignal,
): Promise<V2Goal[]> {
  const goals: V2Goal[] = [];
  const limit = 100;
  for (let offset = 0; ; offset += limit) {
    const query = new URLSearchParams({ limit: String(limit), offset: String(offset) });
    const response = await fetch(
      `${basePath}/api/v2/workspaces/${encodeURIComponent(workspaceId)}/goals?${query}`,
      { credentials: 'include', headers: { Accept: 'application/json' }, signal },
    );
    if (!response.ok) throw new Error(`Goals could not be loaded (${response.status}).`);
    const page = await response.json() as V2Goal[];
    goals.push(...page);
    if (page.length < limit) return goals;
  }
}

export async function getV2GoalsForScope(
  workspaceId: string | null,
  signal?: AbortSignal,
): Promise<V2Goal[]> {
  if (workspaceId) return getV2Goals(workspaceId, signal);
  const workspaces = await getV2Workspaces(signal);
  const goals = await Promise.all(workspaces.map(async (workspace) =>
    (await getV2Goals(workspace.id, signal)).map((goal) => ({
      ...goal,
      workspaceName: workspace.name,
    })),
  ));
  return goals.flat();
}

export type V2GoalInput = {
  title: string;
  description?: string | null;
  targetOn?: string | null;
  status?: V2Goal['status'];
  progressPercent?: number;
  category?: string | null;
};

export async function createV2Goal(workspaceId: string, input: V2GoalInput): Promise<V2Goal> {
  const result = await v2Request<V2Goal>(
    `/api/v2/workspaces/${encodeURIComponent(workspaceId)}/goals`,
    'POST',
    { status: 'active', progressPercent: 0, ...input },
  );
  if (!result?.id) throw new Error('Goal was created without an identifier.');
  return result;
}

export function updateV2Goal(
  workspaceId: string,
  goalId: string,
  patch: Partial<V2GoalInput>,
): Promise<V2Goal | undefined> {
  return v2Request(
    `/api/v2/workspaces/${encodeURIComponent(workspaceId)}/goals/${encodeURIComponent(goalId)}`,
    'PATCH',
    patch,
  );
}

export function archiveV2Goal(workspaceId: string, goalId: string): Promise<void> {
  return v2Request(
    `/api/v2/workspaces/${encodeURIComponent(workspaceId)}/goals/${encodeURIComponent(goalId)}`,
    'DELETE',
  ) as Promise<void>;
}

export async function getV2AgendaPage(
  workspaceId: string | null,
  range: { from: string; to: string; timeZone: string },
  pageToken?: string,
  signal?: AbortSignal,
): Promise<V2AgendaPage> {
  const query = new URLSearchParams(range);
  if (pageToken) query.set('pageToken', pageToken);
  const endpoint = workspaceId
    ? `/api/v2/workspaces/${encodeURIComponent(workspaceId)}/agenda`
    : '/api/v2/agenda';
  const response = await fetch(
    `${basePath}${endpoint}?${query}`,
    { credentials: 'include', headers: { Accept: 'application/json' }, signal },
  );
  if (!response.ok) {
    throw new Error(`Agenda could not be loaded (${response.status}).`);
  }
  return response.json() as Promise<V2AgendaPage>;
}

export async function getV2AgendaRange(
  workspaceId: string | null,
  range: { from: string; to: string; timeZone: string },
  signal?: AbortSignal,
): Promise<V2AgendaPage> {
  const combined: V2AgendaPage = {
    workspaceId: workspaceId ?? 'all',
    ...range,
    tasks: [],
    planEntries: [],
    calendarEvents: [],
  };
  let pageToken: string | undefined;
  let pageCount = 0;

  do {
    if (signal?.aborted) throw new DOMException('Request aborted.', 'AbortError');
    const page = await getV2AgendaPage(workspaceId, range, pageToken, signal);
    combined.tasks.push(...(Array.isArray(page.tasks) ? page.tasks : []));
    combined.planEntries.push(...(Array.isArray(page.planEntries) ? page.planEntries : []));
    combined.calendarEvents.push(...(Array.isArray(page.calendarEvents) ? page.calendarEvents : []));
    pageToken = page.nextPageToken || undefined;
    pageCount += 1;
    if (pageCount > 100) {
      throw new Error('Agenda range exceeds the supported number of pages.');
    }
  } while (pageToken);

  return combined;
}

export type V2TaskRecurrence = {
  frequency: 'daily' | 'weekly' | 'monthly' | 'yearly';
  interval: number;
  weekdays?: number[];
  startsOn: string;
  endsOn: string | null;
  count?: number;
  timeOfDay?: string;
  timeZone: string;
};

export type V2TaskCreateInput = {
  kind: 'task' | 'habit';
  title: string;
  details?: string | null;
  status?: 'open' | 'in_progress' | 'done' | 'cancelled';
  priority?: 'none' | 'low' | 'normal' | 'high';
  dueOn?: string | null;
  dueTime?: string | null;
  plannedOn?: string | null;
  plannedStartTime?: string | null;
  durationMinutes?: number | null;
  timeZone: string;
  label?: string | null;
  goalId?: string | null;
  recurrence?: V2TaskRecurrence | null;
};

export type V2TaskRecord = V2TaskCreateInput & {
  id: string;
  workspaceId: string;
  createdByUserId: string;
  assignedToUserId: string | null;
  parentTaskId: string | null;
  completedAt: string | null;
  archivedAt: string | null;
  createdAt: string;
  updatedAt: string;
};

export type V2TaskReminderRule = {
  id: string;
  workspaceId: string;
  kind: 'task';
  rule: {
    targetId: string;
    basis: 'start' | 'due' | 'daily_brief';
    minutesBefore?: number;
    localTime?: string;
    timeZone?: string;
  };
  status: 'pending_automation' | 'disabled';
};

async function v2Request<T>(
  path: string,
  method: 'POST' | 'PATCH' | 'DELETE',
  body?: unknown,
  signal?: AbortSignal,
): Promise<T | undefined> {
  const response = await fetch(`${basePath}${path}`, {
    method,
    credentials: 'include',
    headers: body === undefined ? { Accept: 'application/json' } : {
      Accept: 'application/json',
      'Content-Type': 'application/json',
    },
    body: body === undefined ? undefined : JSON.stringify(body),
    signal,
  });
  if (!response.ok) throw new Error(`Request could not be completed (${response.status}).`);
  if (response.status === 204) return undefined;
  return response.json() as Promise<T>;
}

export type V2CalendarEventInput = {
  title: string;
  description: string | null;
  startsAt: string;
  endsAt: string | null;
  allDay: boolean;
  timeZone: string;
  location: string | null;
  color: string | null;
};

export async function createV2CalendarEvent(
  workspaceId: string,
  input: V2CalendarEventInput,
): Promise<{ id: string }> {
  const result = await v2Request<{ id: string }>(
    `/api/v2/workspaces/${encodeURIComponent(workspaceId)}/calendar-events`,
    'POST',
    input,
  );
  if (!result?.id) throw new Error('Calendar event was created without an identifier.');
  return result;
}

export function updateV2CalendarEvent(
  workspaceId: string,
  eventId: string,
  input: V2CalendarEventInput,
): Promise<unknown> {
  return v2Request(
    `/api/v2/workspaces/${encodeURIComponent(workspaceId)}/calendar-events/${encodeURIComponent(eventId)}`,
    'PATCH',
    input,
  );
}

export function deleteV2CalendarEvent(workspaceId: string, eventId: string): Promise<void> {
  return v2Request(
    `/api/v2/workspaces/${encodeURIComponent(workspaceId)}/calendar-events/${encodeURIComponent(eventId)}`,
    'DELETE',
  ) as Promise<void>;
}

export async function createV2Task(
  workspaceId: string,
  input: V2TaskCreateInput,
  signal?: AbortSignal,
): Promise<{ id: string }> {
  const result = await v2Request<{ id: string }>(
    `/api/v2/workspaces/${encodeURIComponent(workspaceId)}/tasks`,
    'POST',
    input,
    signal,
  );
  if (!result?.id) throw new Error('Task was created without an identifier.');
  return result;
}

export async function getV2Task(
  workspaceId: string,
  taskId: string,
  signal?: AbortSignal,
): Promise<V2TaskRecord> {
  const response = await fetch(
    `${basePath}/api/v2/workspaces/${encodeURIComponent(workspaceId)}/tasks/${encodeURIComponent(taskId)}`,
    { credentials: 'include', headers: { Accept: 'application/json' }, signal },
  );
  if (!response.ok) throw new Error(`Task could not be loaded (${response.status}).`);
  return response.json() as Promise<V2TaskRecord>;
}

export function updateV2Task(
  workspaceId: string,
  taskId: string,
  patch: Partial<V2TaskCreateInput>,
  signal?: AbortSignal,
): Promise<unknown> {
  return v2Request(
    `/api/v2/workspaces/${encodeURIComponent(workspaceId)}/tasks/${encodeURIComponent(taskId)}`,
    'PATCH',
    patch,
    signal,
  );
}

export async function getV2TaskReminderRules(
  workspaceId: string,
  signal?: AbortSignal,
): Promise<V2TaskReminderRule[]> {
  const rules: V2TaskReminderRule[] = [];
  const limit = 100;
  for (let offset = 0; ; offset += limit) {
    const query = new URLSearchParams({ limit: String(limit), offset: String(offset) });
    const response = await fetch(
      `${basePath}/api/v2/workspaces/${encodeURIComponent(workspaceId)}/reminder-rules?${query}`,
      { credentials: 'include', headers: { Accept: 'application/json' }, signal },
    );
    if (!response.ok) throw new Error(`Reminders could not be loaded (${response.status}).`);
    const page = await response.json() as V2TaskReminderRule[];
    rules.push(...page);
    if (page.length < limit) return rules;
  }
}

export function createV2TaskReminderRule(
  workspaceId: string,
  rule: V2TaskReminderRule['rule'],
): Promise<V2TaskReminderRule | undefined> {
  return v2Request(
    `/api/v2/workspaces/${encodeURIComponent(workspaceId)}/reminder-rules`,
    'POST',
    { kind: 'task', rule, status: 'pending_automation' },
  );
}

export function updateV2TaskReminderRule(
  workspaceId: string,
  ruleId: string,
  rule: V2TaskReminderRule['rule'],
): Promise<V2TaskReminderRule | undefined> {
  return v2Request(
    `/api/v2/workspaces/${encodeURIComponent(workspaceId)}/reminder-rules/${encodeURIComponent(ruleId)}`,
    'PATCH',
    { rule, status: 'pending_automation' },
  );
}

export function deleteV2TaskReminderRule(workspaceId: string, ruleId: string): Promise<void> {
  return v2Request(
    `/api/v2/workspaces/${encodeURIComponent(workspaceId)}/reminder-rules/${encodeURIComponent(ruleId)}`,
    'DELETE',
  ) as Promise<void>;
}

export function updateV2TaskOccurrence(
  workspaceId: string,
  taskId: string,
  date: string,
  patch: { status: 'open' | 'in_progress' | 'done' | 'cancelled' },
  signal?: AbortSignal,
): Promise<unknown> {
  return v2Request(
    `/api/v2/workspaces/${encodeURIComponent(workspaceId)}/tasks/${encodeURIComponent(taskId)}/occurrences/${encodeURIComponent(date)}`,
    'PATCH',
    patch,
    signal,
  );
}

export function archiveV2Task(workspaceId: string, taskId: string): Promise<void> {
  return v2Request(
    `/api/v2/workspaces/${encodeURIComponent(workspaceId)}/tasks/${encodeURIComponent(taskId)}`,
    'DELETE',
  ) as Promise<void>;
}

export async function createV2Action(
  workspaceId: string,
  input: V2TaskCreateInput,
  signal?: AbortSignal,
): Promise<{ id: string }> {
  const result = await v2Request<{ id: string }>(
    `/api/v2/workspaces/${encodeURIComponent(workspaceId)}/actions`,
    'POST',
    input,
    signal,
  );
  if (!result?.id) throw new Error('Action was created without an identifier.');
  return result;
}

export function updateV2Action(
  workspaceId: string,
  actionId: string,
  patch: { status?: 'open' | 'in_progress' | 'done' | 'cancelled'; title?: string; priority?: 'none' | 'low' | 'normal' | 'high' },
  signal?: AbortSignal,
): Promise<unknown> {
  return v2Request(
    `/api/v2/workspaces/${encodeURIComponent(workspaceId)}/actions/${encodeURIComponent(actionId)}`,
    'PATCH',
    patch,
    signal,
  );
}

export function updateV2ActionOccurrence(
  workspaceId: string,
  actionId: string,
  date: string,
  patch: { status: 'open' | 'in_progress' | 'done' | 'cancelled' },
  signal?: AbortSignal,
): Promise<unknown> {
  return v2Request(
    `/api/v2/workspaces/${encodeURIComponent(workspaceId)}/actions/${encodeURIComponent(actionId)}/occurrences/${encodeURIComponent(date)}`,
    'PATCH',
    patch,
    signal,
  );
}

export function archiveV2Action(workspaceId: string, actionId: string): Promise<void> {
  return v2Request(
    `/api/v2/workspaces/${encodeURIComponent(workspaceId)}/actions/${encodeURIComponent(actionId)}`,
    'DELETE',
  ) as Promise<void>;
}
