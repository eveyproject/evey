/* eslint-disable no-restricted-globals */
self.addEventListener('push', function(event) {
  const data = event.data?.json() ?? {};
  const title = data.notification?.title ?? 'Evey';
  const options = {
    body: data.notification?.body ?? '',
    icon: '/favicon.ico',
    data: data.data ?? {},
  };
  event.waitUntil(self.registration.showNotification(title, options));
});

self.addEventListener('notificationclick', function(event) {
  event.notification.close();
  const url = event.notification.data?.url ?? '/';
  event.waitUntil(clients.openWindow(url));
});
