import { createContext, useCallback, useContext, useState, type ReactNode } from 'react';

export type NotificationType = 'info' | 'warning' | 'action';

export interface AppNotification {
  id: string;
  type: NotificationType;
  title: string;
  body: string;
  timestamp: Date;
  read: boolean;
  action?: { label: string; href: string };
  suppressKey?: string;
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
  }, []);

  const markAllRead = useCallback(() => {
    setNotifications((prev) => prev.map((n) => ({ ...n, read: true })));
  }, []);

  const clearAll = useCallback(() => setNotifications([]), []);

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
