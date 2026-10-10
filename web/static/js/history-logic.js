// Logika riwayat tanpa DOM (diuji di tests/js). Semua dihitung di browser:
// aktivitas terenkripsi, dan kartu pesan lembut tidak boleh diketahui server.

import { addDays } from './entry-logic.js';

export const LOW_MOOD_MAX = 2;      // "rendah" = skor 1 atau 2
export const LOW_RUN_DAYS = 3;      // berturut-turut
export const MIN_CORRELATION_DAYS = 3;

/** Satu titik per tanggal dari..sampai; null untuk hari yang tidak diisi. */
export function dailySeries(entries, from, to) {
  const byDate = new Map(entries.map((e) => [e.date, e]));
  const out = [];
  for (let d = from; d <= to; d = addDays(d, 1)) {
    const e = byDate.get(d);
    out.push({ date: d, mood: e ? e.mood : null, sleepMinutes: e ? e.sleepMinutes : null });
  }
  return out;
}

/**
 * Rangkaian mood rendah terbaru: tanggal kalender berurutan, semuanya diisi
 * dengan skor <= 2, dan berakhir hari ini atau kemarin. Hari yang tidak
 * diisi memutus rangkaian. Mengembalikan {start, end, length} atau null.
 */
export function recentLowRun(entries, today) {
  const low = new Set(entries.filter((e) => e.mood <= LOW_MOOD_MAX).map((e) => e.date));
  let end = low.has(today) ? today : low.has(addDays(today, -1)) ? addDays(today, -1) : null;
  if (!end) return null;
  let start = end;
  while (low.has(addDays(start, -1))) start = addDays(start, -1);
  let length = 1;
  for (let d = start; d < end; d = addDays(d, 1)) length++;
  return length >= LOW_RUN_DAYS ? { start, end, length } : null;
}

/**
 * Kartu tampil untuk rangkaian yang belum ditutup. Ditutup dicatat per
 * tanggal awal rangkaian, jadi kartu muncul lagi hanya untuk rangkaian baru.
 */
export function shouldShowGentleCard(run, dismissedStart) {
  return Boolean(run) && run.start !== dismissedStart;
}

const round1 = (n) => Math.round(n * 10) / 10;
const avg = (xs) => xs.reduce((a, b) => a + b, 0) / xs.length;

/**
 * Rata-rata mood pada hari dengan aktivitas X dibanding hari tanpa X.
 * Hanya entri yang isinya bisa didekripsi yang dihitung. Aktivitas dengan
 * data terlalu sedikit di salah satu sisi dilewati. Ini pola, bukan sebab.
 */
export function activityMoodComparison(entries, minDays = MIN_CORRELATION_DAYS) {
  const usable = entries.filter((e) => e.payload && Array.isArray(e.payload.activities));
  const names = new Set(usable.flatMap((e) => e.payload.activities));
  const rows = [];
  for (const name of names) {
    const withX = usable.filter((e) => e.payload.activities.includes(name)).map((e) => e.mood);
    const withoutX = usable.filter((e) => !e.payload.activities.includes(name)).map((e) => e.mood);
    if (withX.length < minDays || withoutX.length < minDays) continue;
    const a = avg(withX);
    const b = avg(withoutX);
    rows.push({
      activity: name,
      withAvg: round1(a), withDays: withX.length,
      withoutAvg: round1(b), withoutDays: withoutX.length,
      diff: round1(a - b),
    });
  }
  return rows.sort((x, y) => Math.abs(y.diff) - Math.abs(x.diff) || x.activity.localeCompare(y.activity));
}

/** Ringkasan rentang: rata-rata mood & tidur dari hari yang diisi. */
export function rangeSummary(series) {
  const filled = series.filter((p) => p.mood !== null);
  if (!filled.length) return { filledDays: 0, totalDays: series.length, avgMood: null, avgSleepMinutes: null };
  return {
    filledDays: filled.length,
    totalDays: series.length,
    avgMood: round1(avg(filled.map((p) => p.mood))),
    avgSleepMinutes: Math.round(avg(filled.map((p) => p.sleepMinutes)) / 30) * 30,
  };
}

export const MONTH_NAMES = ['Januari', 'Februari', 'Maret', 'April', 'Mei', 'Juni', 'Juli', 'Agustus',
  'September', 'Oktober', 'November', 'Desember'];
export const DAY_NAMES = ['Minggu', 'Senin', 'Selasa', 'Rabu', 'Kamis', 'Jumat', 'Sabtu'];

/**
 * Sel kalender satu bulan, minggu dimulai Senin. Sel kosong (null) mengisi
 * awal dan akhir agar jumlah sel kelipatan 7. month: 0–11.
 */
export function monthGrid(year, month) {
  const first = new Date(year, month, 1);
  const days = new Date(year, month + 1, 0).getDate();
  const offset = (first.getDay() + 6) % 7; // Senin = 0
  const pad = (n) => String(n).padStart(2, '0');
  const cells = Array(offset).fill(null);
  for (let d = 1; d <= days; d++) cells.push(`${year}-${pad(month + 1)}-${pad(d)}`);
  while (cells.length % 7) cells.push(null);
  return { title: `${MONTH_NAMES[month]} ${year}`, cells };
}

/** "Sabtu, 10 Oktober 2026" dari tanggal ISO. */
export function longDate(iso) {
  const [y, m, d] = iso.split('-').map(Number);
  const dt = new Date(y, m - 1, d);
  return `${DAY_NAMES[dt.getDay()]}, ${d} ${MONTH_NAMES[m - 1]} ${y}`;
}

/**
 * Statistik 7 hari terakhir (berakhir today) dibanding 7 hari sebelumnya.
 * Rata-rata hanya dari hari yang diisi; null jika tidak ada data.
 */
export function weekStats(entries, today) {
  const cur = rangeSummary(dailySeries(entries, addDays(today, -6), today));
  const prev = rangeSummary(dailySeries(entries, addDays(today, -13), addDays(today, -7)));
  const sleepDelta = cur.avgSleepMinutes !== null && prev.avgSleepMinutes !== null
    ? cur.avgSleepMinutes - prev.avgSleepMinutes : null;
  return { ...cur, prevAvgMood: prev.avgMood, prevAvgSleepMinutes: prev.avgSleepMinutes, sleepDelta };
}

/** Sapaan menurut jam lokal. */
export function greeting(hour) {
  if (hour >= 4 && hour < 11) return 'Selamat pagi';
  if (hour >= 11 && hour < 15) return 'Selamat siang';
  if (hour >= 15 && hour < 18) return 'Selamat sore';
  return 'Selamat malam';
}
