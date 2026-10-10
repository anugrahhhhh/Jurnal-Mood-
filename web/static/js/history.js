// Halaman riwayat: kalender mood per bulan + detail hari, grafik mood & tidur
// (terpisah), perbandingan aktivitas, dan tampilan tabel. Semua catatan
// didekripsi di perangkat ini.

import { renderMonth } from './calendar.js';
import { drawMood, drawSleep, shortDate } from './charts.js';
import { decryptEntry, fromBase64 } from './crypto.js';
import { ACTIVITY_ICONS, addDays, formatSleep, isoDate, MOODS } from './entry-logic.js';
import { activityMoodComparison, dailySeries, longDate, monthGrid, rangeSummary } from './history-logic.js';
import { api } from './http.js';

const HISTORY_DAYS = 366;

export async function loadDecrypted(c, dek, from, to) {
  const res = await api('GET', `/api/entri?dari=${from}&sampai=${to}`);
  const entries = await Promise.all(res.entries.map(async (e) => {
    let payload = null;
    if (dek) {
      try {
        payload = await decryptEntry(dek, c.userId, e.date,
          { iv: fromBase64(e.payloadIv), ciphertext: fromBase64(e.payloadCt) });
      } catch { payload = null; }
    }
    return { date: e.date, mood: e.mood, sleepMinutes: e.sleepMinutes, payload, locked: !dek };
  }));
  return entries;
}

function renderTable(tbody, entries, from) {
  tbody.replaceChildren();
  const rows = entries.filter((e) => e.date >= from).sort((a, b) => (a.date < b.date ? 1 : -1));
  for (const e of rows) {
    const tr = document.createElement('tr');
    const cells = [
      shortDate(e.date),
      `${MOODS[e.mood].face} ${MOODS[e.mood].label}`,
      formatSleep(e.sleepMinutes),
      e.payload ? e.payload.activities.join(', ') || '–' : '(tidak bisa dibuka)',
      e.payload ? e.payload.note || '' : '',
    ];
    for (const v of cells) {
      const td = document.createElement('td');
      td.textContent = v;
      tr.appendChild(td);
    }
    tbody.appendChild(tr);
  }
  if (!rows.length) {
    const tr = document.createElement('tr');
    const td = document.createElement('td');
    td.colSpan = 5;
    td.textContent = 'Belum ada catatan pada rentang ini.';
    tr.appendChild(td);
    tbody.appendChild(tr);
  }
}

function renderComparison(list, empty, entries) {
  list.replaceChildren();
  const rows = activityMoodComparison(entries);
  empty.hidden = rows.length > 0;
  for (const r of rows) {
    const li = document.createElement('li');
    const text = document.createElement('div');
    const name = document.createElement('strong');
    name.textContent = `${ACTIVITY_ICONS[r.activity] ? ACTIVITY_ICONS[r.activity] + ' ' : ''}${r.activity}`;
    const detail = document.createElement('span');
    detail.textContent = `Rata-rata mood ${r.withAvg} pada ${r.withDays} hari dengan aktivitas ini, `
      + `${r.withoutAvg} pada ${r.withoutDays} hari tanpa`;
    text.append(name, detail);
    // Selisih positif ditandai hijau; negatif sengaja netral (tanpa merah) agar tidak menghakimi.
    const badge = document.createElement('span');
    badge.className = `diff ${r.diff >= 0.3 ? 'up' : 'flat'}`;
    badge.textContent = `${r.diff > 0 ? '+' : r.diff < 0 ? '−' : '±'}${Math.abs(r.diff)}`;
    badge.title = 'Selisih rata-rata mood: dengan dikurangi tanpa aktivitas ini';
    li.append(text, badge);
    list.appendChild(li);
  }
}

function make(tag, cls, textContent) {
  const n = document.createElement(tag);
  if (cls) n.className = cls;
  if (textContent !== undefined) n.textContent = textContent;
  return n;
}

function renderDetail(box, iso, entry, today) {
  box.replaceChildren();
  const yesterday = addDays(today, -1);
  const editable = iso === today || iso === yesterday;
  const editHref = iso === today ? '/hari-ini' : `/hari-ini?tanggal=${iso}`;

  const h = make('h2', null, longDate(iso));
  box.appendChild(h);

  if (!entry) {
    box.appendChild(make('p', 'muted', 'Belum ada catatan untuk tanggal ini.'));
    if (editable) {
      const a = make('a', 'btn small', 'Isi sekarang');
      a.href = editHref;
      box.appendChild(a);
    }
    return;
  }

  const head = make('div', 'today-done');
  const face = make('span', 'big-face', MOODS[entry.mood].face);
  face.setAttribute('aria-hidden', 'true');
  const info = make('div');
  info.append(make('strong', null, MOODS[entry.mood].label), make('div', 'muted', `🌙 ${formatSleep(entry.sleepMinutes)} tidur`));
  head.append(face, info);
  box.appendChild(head);

  if (!entry.payload) {
    box.appendChild(make('p', 'muted', 'Isi catatan untuk tanggal ini tidak bisa dibuka.'));
  } else {
    const acts = entry.payload.activities || [];
    if (acts.length) {
      const chips = make('div', 'chips');
      for (const a of acts) chips.appendChild(make('span', 'chip', ACTIVITY_ICONS[a] ? `${ACTIVITY_ICONS[a]} ${a}` : a));
      box.appendChild(chips);
    }
    const note = (entry.payload.note || '').trim();
    const p = make('p', note ? 'note' : 'muted', note || 'Tanpa catatan.');
    p.classList.add('detail-note');
    box.appendChild(p);
    box.appendChild(make('p', 'hint', '🔒 Dibuka di perangkat ini'));
  }
  if (editable) {
    const a = make('a', 'btn secondary small', 'Ubah catatan');
    a.href = editHref;
    box.appendChild(a);
  }
}

export async function initHistoryPage(root, c, dek) {
  const $ = (s) => root.querySelector(s);
  const today = isoDate(new Date());
  const earliest = addDays(today, -(HISTORY_DAYS - 1));
  const entries = await loadDecrypted(c, dek, earliest, today);
  const byDate = new Map(entries.map((e) => [e.date, e]));

  const now = new Date();
  const minYM = (() => { const [y, m] = earliest.split('-').map(Number); return y * 12 + (m - 1); })();
  const maxYM = now.getFullYear() * 12 + now.getMonth();
  let ym = maxYM;
  let selected = today;

  function renderCalendar() {
    const year = Math.floor(ym / 12);
    const month = ym % 12;
    $('[data-role="cal-title"]').textContent = monthGrid(year, month).title;
    $('[data-action="cal-prev"]').disabled = ym <= minYM;
    $('[data-action="cal-next"]').disabled = ym >= maxYM;
    renderMonth($('[data-role="calendar"]'), year, month, byDate, {
      today, selected,
      onSelect: (iso) => {
        selected = iso;
        renderCalendar();
        renderDetail($('[data-role="day-detail"]'), iso, byDate.get(iso), today);
      },
    });
  }

  function render(days) {
    const from = addDays(today, -(days - 1));
    const inRange = entries.filter((e) => e.date >= from);
    const series = dailySeries(inRange, from, today);
    const sum = rangeSummary(series);
    root.querySelectorAll('[data-range]').forEach((b) => b.setAttribute('aria-pressed', String(Number(b.dataset.range) === days)));
    $('[data-role="summary"]').textContent = sum.filledDays
      ? `${sum.filledDays} dari ${sum.totalDays} hari diisi · rata-rata mood ${sum.avgMood} · rata-rata tidur ${formatSleep(sum.avgSleepMinutes)}`
      : `Belum ada catatan dalam ${days} hari terakhir.`;
    drawMood($('[data-role="mood-chart"]'), series);
    drawSleep($('[data-role="sleep-chart"]'), series);
    renderTable($('[data-role="table"] tbody'), inRange, from);
    renderComparison($('[data-role="comparison"]'), $('[data-role="comparison-empty"]'), inRange);
  }

  root.addEventListener('click', (e) => {
    const b = e.target.closest('[data-range]');
    if (b) { render(Number(b.dataset.range)); return; }
    if (e.target.closest('[data-action="cal-prev"]') && ym > minYM) { ym -= 1; renderCalendar(); }
    if (e.target.closest('[data-action="cal-next"]') && ym < maxYM) { ym += 1; renderCalendar(); }
  });

  renderCalendar();
  renderDetail($('[data-role="day-detail"]'), selected, byDate.get(selected), today);
  render(30);
}
