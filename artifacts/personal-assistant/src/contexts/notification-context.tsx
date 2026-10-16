import { createContext, useCallback, useContext, useEffect, useState, type ReactNode } from 'react';

export type NotificationType = 'info' | 'warning' | 'action';
const basePath = import.meta.env.BASE_URL.replace(/\/$/, '');

export interface AppNotification {
  id: string;
  type: NotificationType;
  title: string;
  body: string;
  timestamp: Date;
  read: boolean;
  action?: { label: string; href: string };
  suppressKey?: string;
  persisted?: boolean;
}

export interface AddNotificationInput {
  type: NotificationType;
  title: string;
  body: string;
  action?: { label: string; href: string };
  suppressKey?: string;
}

export const SUPPRESS_KEY_PREFIX = 'askolo_suppress_';

export function isNotificationSuppressed(suppressKey: string): boolean {
  if (typeof localStorage === 'undefined') return false;
  try {
    return localStorage.getItem(`${SUPPRESS_KEY_PREFIX}${suppressKey}`) === '1';
  } catch {
    return false;
  }
}

export function suppressNotificationKey(suppressKey: string): void {
  if (typeof localStorage === 'undefined') return;
  try {
    localStorage.setItem(`${SUPPRESS_KEY_PREFIX}${suppressKey}`, '1');
  } catch { /* ignore */ }
}

interface NotificationContextValue {
  notifications: AppNotification[];
  unreadCount: number;
  addNotification: (n: AddNotificationInput) => void;
  markRead: (id: string) => void;
  markAllRead: () => void;
  clearAll: () => void;
  suppressNotification: (id: string, suppressKey: string) => void;
  isSuppressed: (suppressKey: string) => boolean;
}

const NotificationContext = createContext<NotificationContextValue | null>(null);

export function NotificationProvider({ children }: { children: ReactNode }) {
  const [notifications, setNotifications] = useState<AppNotification[]>([]);

  const refreshPersisted = useCallback(async () => {
    try {
      const response = await fetch(`${basePath}/api/v2/notifications`, {
        credentials: 'include',
        headers: { Accept: 'application/json' },
      });
      if (!response.ok) return;
      const rows = await response.json() as Array<{
        id: string; workspaceId: string; workspaceName: string; eventKey: string;
        resourceId: string; title: string; createdAt: string; readAt: string | null;
      }>;
      const persisted: AppNotification[] = rows.map((row) => ({
        id: row.id,
        type: 'action',
        title: row.eventKey === 'assignments'
          ? 'Task assigned to you'
          : row.eventKey === 'membership' ? 'Space membership updated' : 'Space update',
        body: `${row.title} · ${row.workspaceName}`,
        timestamp: new Date(row.createdAt),
        read: Boolean(row.readAt),
        action: row.eventKey === 'membership'
          ? { label: 'View members', href: `${basePath}/settings/space/members` }
          : { label: 'View task', href: `${basePath}/agenda` },
        persisted: true,
      }));
      setNotifications((current) => [...persisted, ...current.filter((item) => !item.persisted)]);
    } catch {
      // Notification refresh is best-effort; local notifications remain usable.
    }
  }, []);

  useEffect(() => {
    void refreshPersisted();
    const timer = window.setInterval(() => void refreshPersisted(), 60_000);
    return () => window.clearInterval(timer);
  }, [refreshPersisted]);

  const addNotification = useCallback((input: AddNotificationInput) => {
    if (input.suppressKey && isNotificationSuppressed(input.suppressKey)) return;

    const n: AppNotification = {
      id: `n-${Date.now()}-${Math.random().toString(36).slice(2)}`,
      ...input,
      timestamp: new Date(),
      read: false,
    };
    setNotifications((prev) => [n, ...prev]);
  }, []);

  const markRead = useCallback((id: string) => {
    setNotifications((prev) =>
      prev.map((n) => (n.id === id ? { ...n, read: true } : n)),
    );
    void fetch(`${basePath}/api/v2/notifications/${encodeURIComponent(id)}/read`, {
      method: 'POST', credentials: 'include',
    });
  }, []);

  const markAllRead = useCallback(() => {
    setNotifications((prev) => prev.map((n) => ({ ...n, read: true })));
    void fetch(`${basePath}/api/v2/notifications/read-all`, {
      method: 'POST', credentials: 'include',
    });
  }, []);

  const clearAll = useCallback(() => {
    setNotifications([]);
    void fetch(`${basePath}/api/v2/notifications`, {
      method: 'DELETE', credentials: 'include',
    });
  }, []);

  const suppressNotification = useCallback((id: string, suppressKey: string) => {
    suppressNotificationKey(suppressKey);
    setNotifications((prev) =>
      prev.map((n) => (n.id === id ? { ...n, read: true } : n)),
    );
  }, []);

  const isSuppressed = useCallback((suppressKey: string) => isNotificationSuppressed(suppressKey), []);

  const unreadCount = notifications.filter((n) => !n.read).length;

  return (
    <NotificationContext.Provider
      value={{ notifications, unreadCount, addNotification, markRead, markAllRead, clearAll, suppressNotification, isSuppressed }}
    >
      {children}
    </NotificationContext.Provider>
  );
}

export function useNotifications() {
  const ctx = useContext(NotificationContext);
  if (!ctx) throw new Error('useNotifications must be used inside NotificationProvider');
  return ctx;
}
