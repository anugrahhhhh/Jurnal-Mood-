// Service worker Jurnal Mood: hanya untuk pengingat harian.
//
// Sengaja TANPA handler "fetch" dan tanpa cache: halaman bisa berisi teks
// jurnal yang sudah didekripsi, dan tidak boleh tersimpan di perangkat.
// Teks notifikasi selalu sama dan tidak menyebut mood atau isi jurnal,
// karena bisa terlihat di layar kunci.

const TITLE = 'Jurnal Mood';
const BODY = 'Kalau sempat, catat harimu hari ini. Cukup satu menit.';

self.addEventListener('install', () => self.skipWaiting());
self.addEventListener('activate', (e) => e.waitUntil(self.clients.claim()));

self.addEventListener('push', (e) => {
  e.waitUntil(self.registration.showNotification(TITLE, {
    body: BODY,
    icon: '/static/icons/icon-192.png',
    badge: '/static/icons/badge-96.png',
    tag: 'mj-pengingat', // pengingat baru menggantikan yang lama, tidak menumpuk
  }));
});

self.addEventListener('notificationclick', (e) => {
  e.notification.close();
  e.waitUntil((async () => {
    const all = await self.clients.matchAll({ type: 'window', includeUncontrolled: true });
    const open = all.find((c) => new URL(c.url).origin === self.location.origin);
    if (open) {
      await open.focus();
      return open.navigate('/hari-ini');
    }
    return self.clients.openWindow('/hari-ini');
  })());
});
