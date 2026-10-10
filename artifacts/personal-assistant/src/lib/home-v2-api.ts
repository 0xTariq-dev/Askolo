const basePath = import.meta.env?.BASE_URL?.replace(/\/$/, '') ?? '';

export interface V2HomeResponse {
  date: string;
  timeZone: string;
  generatedAt: string;
  scope: 'workspace' | 'all_spaces';
  focusActions: Array<{
    workspaceId: string;
    workspaceName: string;
    taskId: string;
    occurrenceOn: string | null;
    title: string;
    status: string;
    priority: string;
    selection: string;
  }>;
  emailSummary: {
    unreadInboxCount: number;
    actionNeededCount: number;
    lastSyncedAt: string | null;
    freshness: 'current' | 'stale' | 'syncing' | 'error' | 'unavailable';
    spaces?: Array<{
      workspaceId: string;
      workspaceName: string;
      unreadInboxCount: number;
      actionNeededCount: number;
      lastSyncedAt: string | null;
      freshness: 'current' | 'stale' | 'syncing' | 'error' | 'unavailable';
    }>;
  };
  calendarSummary: {
    eventCount: number;
    nextEvent: { title: string; startsAt: string } | null;
  };
}

export async function getV2Home(workspaceId: string | null, signal?: AbortSignal): Promise<V2HomeResponse> {
  const path = workspaceId
    ? `/api/v2/workspaces/${encodeURIComponent(workspaceId)}/home`
    : '/api/v2/home';
  const response = await fetch(`${basePath}${path}`, {
    credentials: 'include',
    headers: { Accept: 'application/json' },
    signal,
  });

  if (!response.ok) {
    throw new Error(response.status === 401
      ? 'Sign in to load Home.'
      : 'The new Home view is temporarily unavailable.');
  }

  return response.json() as Promise<V2HomeResponse>;
}
