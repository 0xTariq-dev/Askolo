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
  startsAt: string;
  endsAt: string | null;
  allDay: boolean;
  timeZone: string;
  sourceProvider: string | null;
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

export type V2TaskCreateInput = {
  kind: 'action' | 'chore' | 'habit' | 'commitment';
  title: string;
  details?: string;
  status?: 'open' | 'in_progress' | 'done' | 'cancelled';
  priority?: 'low' | 'medium' | 'high';
  dueOn?: string;
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

export function updateV2Task(
  workspaceId: string,
  taskId: string,
  patch: { status?: 'open' | 'in_progress' | 'done' | 'cancelled'; title?: string; priority?: 'low' | 'medium' | 'high' },
  signal?: AbortSignal,
): Promise<unknown> {
  return v2Request(
    `/api/v2/workspaces/${encodeURIComponent(workspaceId)}/tasks/${encodeURIComponent(taskId)}`,
    'PATCH',
    patch,
    signal,
  );
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
