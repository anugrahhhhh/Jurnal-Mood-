// Beranda (dashboard). Mood & jam tidur tidak terenkripsi, jadi kartu ini
// tampil walau jurnal terkunci. Pola aktivitas butuh DEK (jurnal terbuka).

import { renderMonth } from './calendar.js';
import { drawMood } from './charts.js';
import { ACTIVITY_ICONS, addDays, dayLabel, formatDuration, formatSleep, isoDate, MOODS } from './entry-logic.js';
import { loadDecrypted } from './history.js';
import { activityMoodComparison, dailySeries, greeting, longDate, monthGrid, weekStats } from './history-logic.js';

// Selisih mood di bawah ini terlalu kecil untuk disebut pola.
const MIN_SHOWN_DIFF = 0.3;

const set = (root, role, value) => {
  const n = root.querySelector(`[data-role="${role}"]`);
  if (n) n.textContent = value;
  return n;
};

export async function initDashboard(root, c, dek) {
  const now = new Date();
  const today = isoDate(now);
  const monthStart = isoDate(new Date(now.getFullYear(), now.getMonth(), 1));
  const from = monthStart < addDays(today, -29) ? monthStart : addDays(today, -29);

  set(root, 'greeting', `${greeting(now.getHours())} 👋`);
  set(root, 'today-date', longDate(today));

  const entries = await loadDecrypted(c, dek, from, today);
  const byDate = new Map(entries.map((e) => [e.date, e]));

  // ---- Hari ini ----
  root.querySelector('[data-role="today-loading"]').hidden = true;
  const todayEntry = byDate.get(today);
  root.querySelector('[data-role="today-empty"]').hidden = Boolean(todayEntry);
  root.querySelector('[data-role="today-done"]').hidden = !todayEntry;
  const cta = root.querySelector('[data-role="cta"]');
  if (cta) cta.textContent = todayEntry ? '✏️ Ubah catatan hari ini' : '＋ Isi catatan hari ini';
  if (todayEntry) {
    set(root, 'today-face', MOODS[todayEntry.mood].face);
    set(root, 'today-label', `Mood hari ini: ${MOODS[todayEntry.mood].label}`);
    set(root, 'today-sleep', `🌙 ${formatSleep(todayEntry.sleepMinutes)} tidur`);
  }

  // ---- 7 hari terakhir ----
  const week = dailySeries(entries, addDays(today, -6), today);
  drawMood(root.querySelector('[data-role="week-chart"]'), week, { dayLabels: dayLabel });

  const stats = weekStats(entries, today);
  set(root, 'avg-sleep', stats.avgSleepMinutes === null ? '–' : formatSleep(stats.avgSleepMinutes));
  set(root, 'avg-sleep-delta', stats.sleepDelta === null ? 'Belum ada pembanding minggu lalu'
    : stats.sleepDelta === 0 ? 'Sama dengan minggu lalu'
      : `${stats.sleepDelta > 0 ? '+' : '−'}${formatDuration(stats.sleepDelta)} dari minggu lalu`);
  set(root, 'avg-mood', stats.avgMood === null ? '–'
    : `${stats.avgMood} / 5 ${MOODS[Math.min(5, Math.max(1, Math.round(stats.avgMood)))].face}`);
  set(root, 'filled-days', `${stats.filledDays} dari 7 hari terakhir diisi`);

  // ---- Kalender bulan ini ----
  const y = now.getFullYear();
  const m = now.getMonth();
  set(root, 'month-title', monthGrid(y, m).title);
  renderMonth(root.querySelector('[data-role="month-cal"]'), y, m, byDate, { today, mini: true });

  // ---- Aktivitas yang sering bersama mood baik (30 hari, perlu DEK) ----
  const box = root.querySelector('[data-role="top-activities"]');
  if (box && dek) {
    box.replaceChildren();
    const rows = activityMoodComparison(entries.filter((e) => e.date >= addDays(today, -29)))
      .filter((r) => r.diff >= MIN_SHOWN_DIFF).slice(0, 4);
    if (!rows.length) {
      const p = document.createElement('p');
      p.className = 'muted';
      p.textContent = 'Belum ada pola yang jelas. Pola muncul setelah aktivitas tercatat minimal 3 hari dan ada perbedaan mood yang berarti.';
      box.appendChild(p);
    }
    for (const r of rows) {
      const chip = document.createElement('span');
      chip.className = 'chip';
      chip.append(`${ACTIVITY_ICONS[r.activity] ? ACTIVITY_ICONS[r.activity] + ' ' : ''}${r.activity} `);
      const b = document.createElement('b');
      b.textContent = `+${r.diff}`;
      chip.appendChild(b);
      chip.title = `Rata-rata mood ${r.withAvg} dengan aktivitas ini, ${r.withoutAvg} tanpa aktivitas ini`;
      box.appendChild(chip);
    }
  }
}
