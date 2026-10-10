// Logika layar harian tanpa DOM, agar bisa diuji di Node (tests/js).

export const MOODS = {
  1: { label: 'Sangat buruk', face: '😣' },
  2: { label: 'Buruk', face: '😕' },
  3: { label: 'Biasa', face: '😐' },
  4: { label: 'Baik', face: '🙂' },
  5: { label: 'Sangat baik', face: '😄' },
};

export const DEFAULT_ACTIVITIES = ['Olahraga', 'Kerja', 'Kuliah', 'Teman', 'Keluarga', 'Istirahat', 'Hiburan', 'Masak'];
export const ACTIVITY_ICONS = {
  Olahraga: '🏃', Kerja: '💼', Kuliah: '📚', Teman: '👥',
  Keluarga: '🏠', Istirahat: '🛌', Hiburan: '🎮', Masak: '🍳',
};

export const SLEEP_STEP = 30;
export const SLEEP_MAX = 24 * 60;
export const SLEEP_DEFAULT = 8 * 60;
export const MAX_ACTIVITY_NAME = 30;
export const MAX_NOTE_LENGTH = 4000;

const DAY_LABELS = ['Min', 'Sen', 'Sel', 'Rab', 'Kam', 'Jum', 'Sab'];

/** Tanggal lokal (zona waktu browser) sebagai 'YYYY-MM-DD'. */
export function isoDate(d) {
  const pad = (n) => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
}

function parseIso(iso) {
  const [y, m, d] = iso.split('-').map(Number);
  return new Date(y, m - 1, d);
}

/** Menambah n hari pada tanggal ISO, aman untuk pergantian bulan/tahun. */
export function addDays(iso, n) {
  const d = parseIso(iso);
  d.setDate(d.getDate() + n);
  return isoDate(d);
}

export function dayLabel(iso) {
  return DAY_LABELS[parseIso(iso).getDay()];
}

export function stepSleep(minutes, delta) {
  const v = Math.round((minutes + delta) / SLEEP_STEP) * SLEEP_STEP;
  return Math.min(SLEEP_MAX, Math.max(0, v));
}

export function formatSleep(minutes) {
  const h = Math.floor(minutes / 60);
  const m = minutes % 60;
  return m ? `${h} j ${m} m` : `${h} j`;
}

/**
 * Nilai awal tombol tidur: entri tanggal itu sendiri jika ada; jika tidak,
 * entri terakhir sebelum tanggal itu; jika tidak ada juga, nilai dari server
 * (entri terakhir sebelum rentang) atau 8 jam.
 */
export function initialSleep(entries, date, sleepBeforeFrom) {
  const own = entries.find((e) => e.date === date);
  if (own) return own.sleepMinutes;
  const earlier = entries.filter((e) => e.date < date).sort((a, b) => (a.date < b.date ? 1 : -1));
  if (earlier.length) return earlier[0].sleepMinutes;
  return sleepBeforeFrom ?? SLEEP_DEFAULT;
}

/** Tujuh hari yang berakhir di endDate, mood null jika tidak diisi. Tanpa hitungan streak. */
export function weekStrip(entries, endDate) {
  const byDate = new Map(entries.map((e) => [e.date, e.mood]));
  const out = [];
  for (let i = 6; i >= 0; i--) {
    const date = addDays(endDate, -i);
    out.push({ date, day: dayLabel(date), mood: byDate.get(date) ?? null });
  }
  return out;
}

export function normalizeActivityName(name) {
  return name.replace(/\s+/g, ' ').trim().slice(0, MAX_ACTIVITY_NAME);
}

/** Menambah aktivitas tanpa duplikat (tidak peka huruf besar/kecil). */
export function addActivity(list, name) {
  const clean = normalizeActivityName(name);
  if (!clean) return list;
  if (list.some((a) => a.toLowerCase() === clean.toLowerCase())) return list;
  return [...list, clean];
}

export function sleepPercent(minutes) {
  return Math.round((minutes / (12 * 60)) * 100); // cincin penuh = 12 jam
}

/** Durasi ringkas untuk selisih: "30 m", "1 j", "1 j 30 m" (tanpa "0 j"). */
export function formatDuration(minutes) {
  const m = Math.abs(minutes);
  const h = Math.floor(m / 60);
  const r = m % 60;
  if (!h) return `${r} m`;
  return r ? `${h} j ${r} m` : `${h} j`;
}

/**
 * Perkiraan kekuatan passphrase untuk umpan balik di layar (bukan jaminan).
 * level: 0 terlalu pendek (< 12), 1 cukup, 2 kuat (>= 20 karakter atau >= 4 kata).
 */
export function passphraseStrength(value) {
  const len = [...value].length;
  const words = value.trim().split(/\s+/).filter((w) => [...w].length >= 3).length;
  if (len < 12) return { level: 0, label: `Terlalu pendek (${len}/12 karakter)` };
  if (len >= 20 || words >= 4) return { level: 2, label: 'Kuat' };
  return { level: 1, label: 'Cukup. Tambah kata agar lebih kuat.' };
}
