self.addEventListener('push', (event) => {
    const data = event.data?.json() ?? {};
    event.waitUntil(
        self.clients.matchAll({ type: 'window', includeUncontrolled: true }).then((clients) => {
            // Don't show a notification if the app is already in the foreground
            if (clients.some((c) => c.visibilityState === 'visible')) return;
            return self.registration.showNotification(data.title ?? 'Conclave', {
                body: data.body,
                icon: '/favicon.svg',
                badge: '/favicon.svg',
                data: { url: data.url ?? '/' }
            });
        })
    );
});

self.addEventListener('notificationclick', (event) => {
    event.notification.close();
    event.waitUntil(
        self.clients.matchAll({ type: 'window' }).then((clients) => {
            const existing = clients.find((c) => 'focus' in c);
            if (existing) return existing.focus();
            return self.clients.openWindow(event.notification.data?.url ?? '/');
        })
    );
});

// Close any open notifications when the page marks a channel/DM as read.
// Payload: { type: 'mark-read' } — closes all notifications since we can't
// match by channel without storing extra metadata per notification.
self.addEventListener('message', (event) => {
    if (event.data?.type === 'mark-read') {
        self.registration.getNotifications().then((notifications) => {
            notifications.forEach((n) => n.close());
        });
    }
});
