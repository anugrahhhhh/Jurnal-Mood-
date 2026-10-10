// Kartu pesan lembut (SPEC 4): muncul setelah 3 hari berturut-turut dengan
// mood rendah. Dihitung di browser; tidak ada permintaan ke server saat
// kartu muncul atau ditutup, dan tidak ada flag yang disimpan di server.

import { addDays, isoDate } from './entry-logic.js';
import { recentLowRun, shouldShowGentleCard } from './history-logic.js';
import { api } from './http.js';

const dismissKey = (userId) => `mj-gentle-dismissed:${userId}`;

function readDismissed(userId) {
  try { return localStorage.getItem(dismissKey(userId)); } catch { return null; }
}

export async function initGentleCard(card, c) {
  const today = isoDate(new Date());
  // Hanya mood (tidak terenkripsi) yang dibutuhkan; ini permintaan riwayat
  // biasa yang sama dengan halaman lain, bukan sinyal ke server.
  const res = await api('GET', `/api/entri?dari=${addDays(today, -13)}&sampai=${today}`);
  const run = recentLowRun(res.entries, today);
  if (!shouldShowGentleCard(run, readDismissed(c.userId))) {
    card.hidden = true;
    return;
  }
  card.hidden = false;
  const close = card.querySelector('[data-action="gentle-close"]');
  close.onclick = () => {
    try { localStorage.setItem(dismissKey(c.userId), run.start); } catch { /* abaikan */ }
    card.hidden = true;
  };
}
