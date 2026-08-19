// ActonOS Service Worker - Background Web Push & Notification Manager
// Provides offline capability, background push wake-up, interactive action buttons, and window focus navigation.

const CACHE_NAME = 'actonos-sw-v1';

// 1. Install Event
self.addEventListener('install', (event) => {
  // Activate immediately without waiting for old workers to shut down
  self.skipWaiting();
});

// 2. Activate Event
self.addEventListener('activate', (event) => {
  event.waitUntil(
    (async () => {
      // Claim all open clients immediately
      await self.clients.claim();
    })()
  );
});

// 3. Web Push Background Event
self.addEventListener('push', (event) => {
  let data = {};
  if (event.data) {
    try {
      data = event.data.json();
    } catch {
      data = {
        title: 'ActonOS Notification',
        message: event.data.text(),
      };
    }
  }

  const title = data.title || 'ActonOS Alert';
  const notifType = data.type || 'info';
  const link = data.link || '/notifications';

  // Custom vibration patterns based on urgency
  let vibratePattern = [100, 50, 100];
  if (notifType === 'approval' || notifType === 'error') {
    vibratePattern = [200, 100, 200, 100, 200];
  }

  // Interactive action buttons
  const actions = [];
  if (notifType === 'approval') {
    actions.push({ action: 'open', title: 'Review / Phê duyệt' });
    actions.push({ action: 'dismiss', title: 'Dismiss / Bỏ qua' });
  } else {
    actions.push({ action: 'open', title: 'View / Xem' });
    actions.push({ action: 'dismiss', title: 'Close / Đóng' });
  }

  const options = {
    body: data.message || 'You have a new update in ActonOS.',
    icon: '/actonos_icon.png',
    badge: '/actonos_icon.png',
    tag: data.id || `actonos-push-${Date.now()}`,
    data: {
      id: data.id,
      link: link,
      type: notifType,
      category: data.category || 'system',
      timestamp: Date.now(),
    },
    vibrate: vibratePattern,
    requireInteraction: notifType === 'approval' || notifType === 'error',
    actions: actions,
  };

  event.waitUntil(
    (async () => {
      // Broadcast payload to all open ActonOS tabs if any
      const clientList = await self.clients.matchAll({ type: 'window', includeUncontrolled: true });
      for (const client of clientList) {
        client.postMessage({
          type: 'PUSH_NOTIFICATION_RECEIVED',
          payload: data,
        });
      }

      // Display system desktop notification
      return self.registration.showNotification(title, options);
    })()
  );
});

// 4. Notification Click Event
self.addEventListener('notificationclick', (event) => {
  event.notification.close();

  const action = event.action;
  if (action === 'dismiss') {
    return;
  }

  const rawLink = event.notification.data?.link || '/notifications';
  // Normalize link for hash router (e.g. "/missions" -> "#/missions")
  let cleanRoute = rawLink;
  if (cleanRoute.startsWith('/')) {
    cleanRoute = `#${cleanRoute}`;
  } else if (!cleanRoute.startsWith('#')) {
    cleanRoute = `#/${cleanRoute}`;
  }

  event.waitUntil(
    (async () => {
      const clientList = await self.clients.matchAll({ type: 'window', includeUncontrolled: true });
      
      // If a window is already open, focus it and navigate
      for (const client of clientList) {
        if ('focus' in client) {
          await client.focus();
          client.postMessage({
            type: 'NAVIGATE_ROUTE',
            route: cleanRoute,
            link: rawLink,
          });
          // In case client is on another page, adjust hash
          if (client.navigate && client.url) {
            try {
              const url = new URL(client.url);
              url.hash = cleanRoute;
              await client.navigate(url.href);
            } catch {
              // Ignore navigate errors
            }
          }
          return;
        }
      }

      // If no window is open, open a new window to ActonOS
      if (self.clients.openWindow) {
        const rootUrl = new URL(`/${cleanRoute}`, self.location.origin).href;
        return self.clients.openWindow(rootUrl);
      }
    })()
  );
});

// 5. Message Event from foreground UI
self.addEventListener('message', (event) => {
  if (!event.data) return;

  if (event.data.type === 'SKIP_WAITING') {
    self.skipWaiting();
  }

  if (event.data.type === 'SHOW_DESKTOP_NOTIFICATION') {
    const { title, options } = event.data;
    self.registration.showNotification(title || 'ActonOS', options || {});
  }
});
