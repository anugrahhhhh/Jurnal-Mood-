// Ekspor data tanpa DOM (diuji di tests/js). Isi catatan sudah didekripsi di
// browser; file hasil ekspor TIDAK terenkripsi dan tidak pernah dikirim ke server.

import { addDays, formatSleep, MOODS } from './entry-logic.js';

export const EXPORT_FORMAT = 1;

/** Rentang tanggal [dari, sampai] per potongan (API membatasi satu permintaan). */
export function dateChunks(first, last, size = 366) {
  const out = [];
  for (let from = first; from <= last; from = addDays(from, size)) {
    const to = addDays(from, size - 1);
    out.push([from, to < last ? to : last]);
  }
  return out;
}

function row(e) {
  return {
    date: e.date,
    mood: e.mood,
    moodLabel: MOODS[e.mood] ? MOODS[e.mood].label : '',
    sleepMinutes: e.sleepMinutes,
    activities: e.payload ? e.payload.activities || [] : [],
    note: e.payload ? e.payload.note || '' : '',
    readable: Boolean(e.payload),
  };
}

export function toExportJSON(entries, exportedAt) {
  return {
    app: 'Jurnal Mood',
    format: EXPORT_FORMAT,
    exportedAt,
    note: 'File ini berisi catatan yang sudah dibuka (tidak terenkripsi). Simpan di tempat aman.',
    entries: [...entries].sort((a, b) => (a.date < b.date ? -1 : 1)).map(row),
  };
}

// Sel yang diawali = + - @ (atau tab/CR) bisa dijalankan sebagai rumus oleh
// Excel/Sheets ("CSV injection"); diberi awalan ' agar tetap teks biasa.
function cell(value) {
  let s = String(value ?? '');
  if (/^[=+\-@\t\r]/.test(s)) s = `'${s}`;
  if (/[",;\n\r]/.test(s)) s = `"${s.replace(/"/g, '""')}"`;
  return s;
}

export function toCSV(entries) {
  const header = ['tanggal', 'mood', 'label_mood', 'tidur_menit', 'tidur', 'aktivitas', 'catatan'];
  const lines = [header.join(',')];
  for (const r of toExportJSON(entries, '').entries) {
    lines.push([
      r.date, r.mood, r.moodLabel, r.sleepMinutes, formatSleep(r.sleepMinutes),
      r.activities.join('; '), r.readable ? r.note : '(tidak bisa dibuka)',
    ].map(cell).join(','));
  }
  // BOM agar Excel membaca UTF-8 (emoji & huruf beraksen) dengan benar.
  return `﻿${lines.join('\r\n')}\r\n`;
}

export function exportFileName(ext, today) {
  return `jurnal-mood-${today}.${ext}`;
}

export function recoveryFileText(code, createdAt) {
  return [
    'JURNAL MOOD - KUNCI PEMULIHAN',
    '',
    `Kunci: ${code}`,
    `Dibuat: ${createdAt}`,
    '',
    'Gunakan kunci ini di halaman "Buka jurnal" -> "Lupa passphrase?" bila kamu lupa passphrase jurnal.',
    'Simpan di tempat aman, terpisah dari passphrase. Siapa pun yang punya kunci ini DAN password',
    'login-mu bisa membuka catatanmu. Pengelola situs tidak bisa memulihkannya untukmu.',
    '',
  ].join('\r\n');
}
