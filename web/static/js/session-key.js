// Kunci jurnal selama sesi (SPEC 3.3): hanya di memori JavaScript.
// Tidak pernah ditulis ke localStorage, sessionStorage, IndexedDB, atau cookie.
//
// Navigasi antarhalaman memakai hx-boost, sehingga memori ini bertahan.
// Memuat ulang halaman, keluar, atau tidak aktif 15 menit akan menghapusnya.

export const AUTO_LOCK_MS = 15 * 60 * 1000;
const CHECK_EVERY_MS = 15 * 1000;

let state = null; // { userId, dek }
let lastActivity = 0;
let timer = null;

const ACTIVITY_EVENTS = ['pointerdown', 'keydown', 'touchstart', 'wheel'];

function markActive() {
  lastActivity = Date.now();
}

function checkIdle() {
  if (state && Date.now() - lastActivity > AUTO_LOCK_MS) lock({ reason: 'idle' });
}

function setIndicator(unlocked) {
  document.documentElement.dataset.journal = unlocked ? 'unlocked' : 'locked';
}

export function setKey(userId, dek) {
  state = { userId: String(userId), dek };
  markActive();
  setIndicator(true);
  if (!timer) {
    ACTIVITY_EVENTS.forEach((e) => document.addEventListener(e, markActive, { passive: true, capture: true }));
    document.addEventListener('visibilitychange', checkIdle);
    timer = setInterval(checkIdle, CHECK_EVERY_MS);
  }
}

/** DEK untuk pengguna ini, atau null jika jurnal terkunci. */
export function getKey(userId) {
  return state && state.userId === String(userId) ? state.dek : null;
}

/**
 * Mengunci jurnal. Navigasi penuh (bukan htmx) dipakai agar isi halaman yang
 * mungkin berisi teks terdekripsi ikut terhapus dari memori.
 */
export function lock({ reason = 'manual' } = {}) {
  state = null;
  setIndicator(false);
  const next = location.pathname + location.search;
  const q = new URLSearchParams({ terkunci: reason });
  if (next.startsWith('/') && !next.startsWith('/jurnal/')) q.set('lanjut', next);
  location.assign('/jurnal/buka?' + q.toString());
}
