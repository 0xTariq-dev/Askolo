/**
 * Runs once per browser session (gated by sessionStorage) to check whether
 * Google Calendar or Gmail is disconnected and fires warning notifications.
 */
import { useEffect, useRef } from 'react';
import { useGetGoogleStatus, getGetGoogleStatusQueryKey } from '@workspace/api-client-react';
import { useNotifications } from '@/contexts/notification-context';

const SESSION_KEY = 'askolo_google_status_checked';

export function useGoogleConnectionCheck() {
  const { addNotification } = useNotifications();
  const alreadyChecked = useRef(false);

  // Only enable the query if we haven't checked yet this session.
  const alreadyInSession = typeof sessionStorage !== 'undefined'
    ? sessionStorage.getItem(SESSION_KEY) === '1'
    : true;

  const { data } = useGetGoogleStatus({
    query: {
      queryKey: getGetGoogleStatusQueryKey(),
      enabled: !alreadyInSession && !alreadyChecked.current,
      staleTime: Infinity,
    },
  });

  useEffect(() => {
    if (!data || alreadyChecked.current || alreadyInSession) return;
    alreadyChecked.current = true;
    try { sessionStorage.setItem(SESSION_KEY, '1'); } catch { /* ignore */ }

    if (!data.calendarConnected) {
      addNotification({
        type: 'action',
        title: 'Google Calendar not connected',
        body: 'Connect Calendar on your Profile page to sync events and get smarter scheduling.',
        action: { label: 'Go to Profile', href: '/profile' },
        suppressKey: 'google-connection:calendar',
      });
    }
    if (!data.gmailConnected) {
      addNotification({
        type: 'action',
        title: 'Gmail not connected',
        body: 'Connect Gmail on your Profile page to enable email triage and AI reply drafting.',
        action: { label: 'Go to Profile', href: '/profile' },
        suppressKey: 'google-connection:gmail',
      });
    }
  }, [data, addNotification, alreadyInSession]);
}
