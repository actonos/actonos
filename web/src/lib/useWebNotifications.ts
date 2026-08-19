import { useEffect, useState, useCallback, useRef } from 'react';
import { useRealtime } from '@/components/providers/RealtimeProvider';
import { api } from '@/lib/api';
import type { NotificationItem, PushSubscriptionPayload } from '@/lib/types';

export type BrowserNotificationPermission = 'default' | 'granted' | 'denied' | 'unsupported';

// Helper to convert base64 VAPID public key to Uint8Array for PushManager
function urlBase64ToUint8Array(base64String: string): Uint8Array {
  const padding = '='.repeat((4 - (base64String.length % 4)) % 4);
  const base64 = (base64String + padding).replace(/-/g, '+').replace(/_/g, '/');
  const rawData = window.atob(base64);
  const outputArray = new Uint8Array(rawData.length);
  for (let i = 0; i < rawData.length; ++i) {
    outputArray[i] = rawData.charCodeAt(i);
  }
  return outputArray;
}

// Module-level deduplication key for realtime events
let lastDesktopNotificationId: string | null = null;
let swRegistrationPromise: Promise<ServiceWorkerRegistration | null> | null = null;

function getServiceWorkerRegistration(): Promise<ServiceWorkerRegistration | null> {
  if (typeof window === 'undefined' || !('serviceWorker' in navigator)) {
    return Promise.resolve(null);
  }
  if (!swRegistrationPromise) {
    swRegistrationPromise = navigator.serviceWorker
      .register('/sw.js', { scope: '/' })
      .then(() => {
        return navigator.serviceWorker.ready;
      })
      .catch((err) => {
        console.warn('[ActonOS SW] Registration failed:', err);
        return null;
      });
  }
  return swRegistrationPromise;
}

function playNotificationSound(type: string = 'info') {
  try {
    const AudioCtx = window.AudioContext || (window as unknown as { webkitAudioContext: typeof AudioContext }).webkitAudioContext;
    if (!AudioCtx) return;
    const ctx = new AudioCtx();
    const now = ctx.currentTime;

    const osc = ctx.createOscillator();
    const gain = ctx.createGain();

    osc.type = 'sine';
    if (type === 'approval' || type === 'error') {
      // 2-tone chime for critical/approval alerts
      osc.frequency.setValueAtTime(659.25, now); // E5
      osc.frequency.setValueAtTime(880.0, now + 0.12); // A5
      gain.gain.setValueAtTime(0.3, now);
      gain.gain.exponentialRampToValueAtTime(0.001, now + 0.45);
      osc.connect(gain);
      gain.connect(ctx.destination);
      osc.start(now);
      osc.stop(now + 0.45);
    } else {
      // Pleasant chime for general updates
      osc.frequency.setValueAtTime(783.99, now); // G5
      osc.frequency.setValueAtTime(1046.5, now + 0.09); // C6
      gain.gain.setValueAtTime(0.25, now);
      gain.gain.exponentialRampToValueAtTime(0.001, now + 0.38);
      osc.connect(gain);
      gain.connect(ctx.destination);
      osc.start(now);
      osc.stop(now + 0.38);
    }
  } catch {
    // Audio autoplay may be restricted if user has not interacted with page
  }
}

export function useWebNotifications() {
  const [permission, setPermission] = useState<BrowserNotificationPermission>(() => {
    if (typeof window === 'undefined' || !('Notification' in window)) {
      return 'unsupported';
    }
    return Notification.permission;
  });

  const [isPushSupported, setIsPushSupported] = useState<boolean>(false);
  const [isPushSubscribed, setIsPushSubscribed] = useState<boolean>(false);
  const [isServiceWorkerReady, setIsServiceWorkerReady] = useState<boolean>(false);

  const { snapshot } = useRealtime();
  const isSubscribingRef = useRef<boolean>(false);

  // Register push subscription with backend
  const subscribeToPush = useCallback(async (reg?: ServiceWorkerRegistration): Promise<boolean> => {
    if (isSubscribingRef.current) return false;
    try {
      isSubscribingRef.current = true;
      const swReg = reg || (await getServiceWorkerRegistration());
      if (!swReg || !swReg.pushManager) {
        return false;
      }

      // 1. Fetch VAPID public key from backend
      const { public_key } = await api.getVAPIDPublicKey();
      if (!public_key) {
        console.warn('[ActonOS WebPush] No VAPID public key available on server');
        return false;
      }

      // 2. Check existing subscription or subscribe
      const convertedKey = urlBase64ToUint8Array(public_key);
      let sub = await swReg.pushManager.getSubscription();

      if (!sub) {
        sub = await swReg.pushManager.subscribe({
          userVisibleOnly: true,
          applicationServerKey: convertedKey,
        });
      }

      // 3. Serialize and send to backend
      const rawSub = sub.toJSON();
      if (rawSub.endpoint && rawSub.keys?.p256dh && rawSub.keys?.auth) {
        const payload: PushSubscriptionPayload = {
          endpoint: rawSub.endpoint,
          keys: {
            p256dh: rawSub.keys.p256dh,
            auth: rawSub.keys.auth,
          },
          user_agent: navigator.userAgent,
        };
        await api.subscribePush(payload);
        setIsPushSubscribed(true);
        console.info('[ActonOS WebPush] Push subscription synced successfully with daemon');
        return true;
      }
      return false;
    } catch (err) {
      console.warn('[ActonOS WebPush] Subscription error, attempting fresh subscription:', err);
      // In case of key mismatch error, try unsubscribing old and re-subscribing
      try {
        const swReg = reg || (await getServiceWorkerRegistration());
        if (swReg?.pushManager) {
          const oldSub = await swReg.pushManager.getSubscription();
          if (oldSub) await oldSub.unsubscribe();
          const { public_key } = await api.getVAPIDPublicKey();
          if (public_key) {
            const convertedKey = urlBase64ToUint8Array(public_key);
            const freshSub = await swReg.pushManager.subscribe({
              userVisibleOnly: true,
              applicationServerKey: convertedKey,
            });
            const rawSub = freshSub.toJSON();
            if (rawSub.endpoint && rawSub.keys?.p256dh && rawSub.keys?.auth) {
              await api.subscribePush({
                endpoint: rawSub.endpoint,
                keys: { p256dh: rawSub.keys.p256dh, auth: rawSub.keys.auth },
                user_agent: navigator.userAgent,
              });
              setIsPushSubscribed(true);
              return true;
            }
          }
        }
      } catch (retryErr) {
        console.error('[ActonOS WebPush] Retry subscription failed:', retryErr);
      }
      return false;
    } finally {
      isSubscribingRef.current = false;
    }
  }, []);

  // Initialize Service Worker & check Push Subscription status
  useEffect(() => {
    if (typeof window === 'undefined') return;

    const pushSupported = 'serviceWorker' in navigator && 'PushManager' in window && 'Notification' in window;
    setIsPushSupported(pushSupported);

    if ('serviceWorker' in navigator) {
      getServiceWorkerRegistration().then(async (reg) => {
        if (!reg) return;
        setIsServiceWorkerReady(true);

        // Auto-subscribe or sync existing subscription when permission is granted
        if (Notification.permission === 'granted' && reg.pushManager) {
          try {
            await subscribeToPush(reg);
          } catch (err) {
            console.warn('[ActonOS SW] Auto push sync error:', err);
          }
        } else if (reg.pushManager) {
          try {
            const existingSub = await reg.pushManager.getSubscription();
            setIsPushSubscribed(!!existingSub);
          } catch (err) {
            console.warn('[ActonOS SW] Failed to check push subscription:', err);
          }
        }
      });

      // Handle messages from Service Worker (e.g. user clicked notification or push arrived)
      const handleMessage = (event: MessageEvent) => {
        if (event.data?.type === 'NAVIGATE_ROUTE' && event.data.route) {
          window.location.hash = event.data.route;
        }
        if (event.data?.type === 'PUSH_NOTIFICATION_RECEIVED') {
          playNotificationSound(event.data?.payload?.type || 'info');
        }
      };
      navigator.serviceWorker.addEventListener('message', handleMessage);
      return () => {
        navigator.serviceWorker.removeEventListener('message', handleMessage);
      };
    }
  }, [permission, subscribeToPush]);

  // Unsubscribe from push
  const unsubscribeFromPush = useCallback(async (): Promise<boolean> => {
    try {
      const swReg = await getServiceWorkerRegistration();
      if (!swReg || !swReg.pushManager) return false;

      const sub = await swReg.pushManager.getSubscription();
      if (sub) {
        const endpoint = sub.endpoint;
        await sub.unsubscribe();
        await api.unsubscribePush(endpoint);
      }
      setIsPushSubscribed(false);
      return true;
    } catch (err) {
      console.warn('[ActonOS WebPush] Failed to unsubscribe:', err);
      return false;
    }
  }, []);

  // Request browser permission and trigger push subscription
  const requestPermission = useCallback(async (): Promise<BrowserNotificationPermission> => {
    if (typeof window === 'undefined' || !('Notification' in window)) {
      setPermission('unsupported');
      return 'unsupported';
    }
    try {
      const res = await Notification.requestPermission();
      setPermission(res);
      if (res === 'granted') {
        const swReg = await getServiceWorkerRegistration();
        if (swReg) {
          await subscribeToPush(swReg);
        }
      }
      return res;
    } catch {
      return 'denied';
    }
  }, [subscribeToPush]);

  // Display notification via Service Worker registration (with fallback to window Notification)
  const showDesktopNotification = useCallback(
    async (notif: NotificationItem) => {
      if (typeof window === 'undefined' || !('Notification' in window) || Notification.permission !== 'granted') {
        return;
      }
      try {
        playNotificationSound(notif.type);

        const swReg = await getServiceWorkerRegistration();
        if (swReg && 'showNotification' in swReg) {
          await swReg.showNotification(notif.title, {
            body: notif.message,
            icon: '/actonos_icon.png',
            badge: '/actonos_icon.png',
            tag: notif.id,
            data: {
              id: notif.id,
              link: notif.link || '/notifications',
              type: notif.type,
            },
            requireInteraction: true,
            silent: false,
          } as NotificationOptions);
          return;
        }

        // Standard Notification fallback if Service Worker not available
        const desktopNotif = new Notification(notif.title, {
          body: notif.message,
          icon: '/actonos_icon.png',
          tag: notif.id,
          requireInteraction: true,
          silent: false,
        });

        desktopNotif.onclick = () => {
          window.focus();
          if (notif.link) {
            window.location.hash = notif.link.startsWith('/') ? `#${notif.link}` : `#/${notif.link}`;
          }
          desktopNotif.close();
        };
      } catch (err) {
        console.warn('[ActonOS Notification] Failed to display notification:', err);
      }
    },
    []
  );

  // Send a test push notification via backend
  const testPush = useCallback(async (params?: { title?: string; message?: string; link?: string }) => {
    return api.testPushNotification(params);
  }, []);

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
    isPushSupported,
    isPushSubscribed,
    isServiceWorkerReady,
    requestPermission,
    subscribeToPush,
    unsubscribeFromPush,
    testPush,
    showDesktopNotification,
    unreadCount: snapshot?.notifications_unread ?? 0,
  };
}

