import { useEffect, useRef, useState, useCallback } from 'react';
import { useRealtime } from '@/components/providers/RealtimeProvider';
import type { NotificationItem } from '@/lib/types';

export type BrowserNotificationPermission = 'default' | 'granted' | 'denied' | 'unsupported';

export function useWebNotifications() {
  const [permission, setPermission] = useState<BrowserNotificationPermission>(() => {
    if (typeof window === 'undefined' || !('Notification' in window)) {
      return 'unsupported';
    }
    return Notification.permission;
  });

  const { snapshot } = useRealtime();
  const lastNotifiedIdRef = useRef<string | null>(null);

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
    if (!latest || latest.is_read || latest.id === lastNotifiedIdRef.current) {
      return;
    }

    lastNotifiedIdRef.current = latest.id;
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
