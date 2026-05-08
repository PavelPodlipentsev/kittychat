self.addEventListener('push', e => {
  const data = e.data ? e.data.json() : { title: 'KittyChat 🐱', body: 'Новое сообщение' };
  e.waitUntil(
    self.registration.showNotification(data.title || 'KittyChat 🐱', {
      body: data.body || 'Новое сообщение',
      icon: '/icon.png'
    })
  );
});

self.addEventListener('notificationclick', e => {
  e.notification.close();
  e.waitUntil(clients.openWindow('/'));
});
