// Pengaturan pengingat harian (Web Push) untuk perangkat ini.

import { api } from './http.js';

const isIOS = () => /iPad|iPhone|iPod/.test(navigator.userAgent)
  || (navigator.platform === 'MacIntel' && navigator.maxTouchPoints > 1);
const isStandalone = () => window.matchMedia('(display-mode: standalone)').matches || navigator.standalone === true;

function urlBase64ToBytes(b64) {
  const s = atob((b64 + '='.repeat((4 - (b64.length % 4)) % 4)).replace(/-/g, '+').replace(/_/g, '/'));
  return Uint8Array.from(s, (ch) => ch.charCodeAt(0));
}

export async function initReminders(root) {
  const $ = (s) => root.querySelector(s);
  const status = $('[data-role="reminder-status"]');
  const form = $('[data-role="reminder-form"]');
  const timeInput = $('[data-field="reminder-time"]');
  const enableBtn = $('[data-action="reminder-enable"]');
  const disableBtn = $('[data-action="reminder-disable"]');
  const tz = Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC';
  $('[data-role="reminder-tz"]').textContent = tz;

  const say = (text) => { status.textContent = text; };

  if (!('serviceWorker' in navigator) || !('PushManager' in window) || !('Notification' in window)) {
    say(isIOS() && !isStandalone()
      ? 'Di iPhone/iPad, pengingat hanya bisa dipakai setelah situs ini ditambahkan ke Layar Utama: '
        + 'tekan tombol Bagikan di Safari → "Tambahkan ke Layar Utama", lalu buka dari ikon itu.'
      : 'Browser ini tidak mendukung notifikasi web.');
    return;
  }

  const info = await api('GET', '/api/pengingat');
  if (!info.enabled) {
    say('Pengingat belum diaktifkan oleh pengelola situs.');
    return;
  }

  const reg = await navigator.serviceWorker.register('/sw.js', { scope: '/' });
  await navigator.serviceWorker.ready;

  async function render() {
    const sub = await reg.pushManager.getSubscription();
    const mine = sub && info.subscriptions.find((s) => s.endpoint === sub.endpoint);
    form.hidden = false;
    if (mine) {
      timeInput.value = mine.time;
      say(`Pengingat aktif di perangkat ini setiap hari pukul ${mine.time}.`);
      enableBtn.textContent = 'Simpan jam';
      disableBtn.hidden = false;
    } else if (Notification.permission === 'denied') {
      say('Notifikasi untuk situs ini diblokir di pengaturan browser. Izinkan dulu, lalu muat ulang halaman.');
      form.hidden = true;
    } else {
      say('Pengingat belum aktif di perangkat ini.');
      enableBtn.textContent = 'Aktifkan pengingat';
      disableBtn.hidden = true;
    }
  }

  async function busy(btn, fn) {
    btn.disabled = true;
    try {
      await fn();
    } catch (err) {
      console.error(err);
      say(err.message || 'Terjadi kesalahan. Coba lagi.');
    } finally {
      btn.disabled = false;
    }
  }

  enableBtn.addEventListener('click', () => busy(enableBtn, async () => {
    if (Notification.permission !== 'granted') {
      const perm = await Notification.requestPermission();
      if (perm !== 'granted') {
        say('Izin notifikasi tidak diberikan, jadi pengingat tidak diaktifkan.');
        return;
      }
    }
    let sub = await reg.pushManager.getSubscription();
    if (!sub) {
      sub = await reg.pushManager.subscribe({
        userVisibleOnly: true,
        applicationServerKey: urlBase64ToBytes(info.publicKey),
      });
    }
    const json = sub.toJSON();
    await api('PUT', '/api/pengingat', {
      endpoint: json.endpoint, keys: { p256dh: json.keys.p256dh, auth: json.keys.auth },
      time: timeInput.value || '21:00', timezone: tz,
    });
    const i = info.subscriptions.findIndex((s) => s.endpoint === json.endpoint);
    const entry = { endpoint: json.endpoint, time: timeInput.value || '21:00' };
    if (i >= 0) info.subscriptions[i] = entry; else info.subscriptions.push(entry);
    await render();
  }));

  disableBtn.addEventListener('click', () => busy(disableBtn, async () => {
    const sub = await reg.pushManager.getSubscription();
    if (sub) {
      await api('DELETE', '/api/pengingat', { endpoint: sub.endpoint });
      await sub.unsubscribe();
      info.subscriptions = info.subscriptions.filter((s) => s.endpoint !== sub.endpoint);
    }
    await render();
  }));

  await render();
}
