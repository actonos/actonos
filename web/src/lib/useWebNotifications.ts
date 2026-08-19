import { useEffect, useState, useCallback } from 'react';
import { useRealtime } from '@/components/providers/RealtimeProvider';
import type { NotificationItem } from '@/lib/types';

export type BrowserNotificationPermission = 'default' | 'granted' | 'denied' | 'unsupported';

// Shared across every useWebNotifications() instance (NotificationBell,
// NotificationsPage, etc. all mount this hook simultaneously). Without a
// module-level dedup key, each instance keeps its own lastNotifiedIdRef and
// independently fires a desktop notification for the same realtime event.
let lastDesktopNotificationId: string | null = null;

export function useWebNotifications() {
  const [permission, setPermission] = useState<BrowserNotificationPermission>(() => {
    if (typeof window === 'undefined' || !('Notification' in window)) {
      return 'unsupported';
    }
    return Notification.permission;
  });

  const { snapshot } = useRealtime();

  const requestPermission = useCallback(async (): Promise<BrowserNotificationPermission> => {
    if (typeof window === 'undefined' || !('Notification' in window)) {
      setPermission('unsupported');
      return 'unsupported';
    }
    try {
      const res = await Notification.requestPermission();
      setPermission(res);
      return res;
    } catch {
      return 'denied';
    }
  }, []);

  const showDesktopNotification = useCallback(
    (notif: NotificationItem) => {
      if (typeof window === 'undefined' || !('Notification' in window) || Notification.permission !== 'granted') {
        return;
      }
      try {
        const desktopNotif = new Notification(notif.title, {
          body: notif.message,
          icon: '/actonos_icon.png',
          tag: notif.id,
        });

        desktopNotif.onclick = () => {
          window.focus();
          if (notif.link) {
            window.location.hash = notif.link.startsWith('/') ? `#${notif.link}` : `#/${notif.link}`;
          }
          desktopNotif.close();
        };
      } catch (err) {
        console.warn('Failed to display desktop notification', err);
      }
    },
    []
  );

  // Trigger browser push when a new unread notification arrives via Realtime WebSocket
  useEffect(() => {
    const latest = snapshot?.latest_notification;
    if (!latest || latest.is_read || latest.id === lastDesktopNotificationId) {
      return;
    }

    lastDesktopNotificationId = latest.id;
    if (permission === 'granted') {
      showDesktopNotification(latest);
    }
  }, [snapshot?.latest_notification, permission, showDesktopNotification]);

  return {
    permission,
    requestPermission,
    showDesktopNotification,
    unreadCount: snapshot?.notifications_unread ?? 0,
  };
}
