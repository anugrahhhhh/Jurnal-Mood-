import assert from 'node:assert/strict';
import { test } from 'node:test';

import { dateChunks, exportFileName, recoveryFileText, toCSV, toExportJSON } from '../../web/static/js/export-logic.js';

const e = (date, mood, sleepMinutes, activities, note) =>
  ({ date, mood, sleepMinutes, payload: activities ? { activities, note } : null });

test('dateChunks membagi rentang tanpa celah atau tumpang tindih', () => {
  assert.deepEqual(dateChunks('2026-10-01', '2026-10-01'), [['2026-10-01', '2026-10-01']]);
  const chunks = dateChunks('2024-01-01', '2026-10-11', 366);
  assert.equal(chunks[0][0], '2024-01-01');
  assert.equal(chunks.at(-1)[1], '2026-10-11');
  for (let i = 1; i < chunks.length; i++) {
    const prevEnd = new Date(chunks[i - 1][1]);
    const start = new Date(chunks[i][0]);
    assert.equal((start - prevEnd) / 86400000, 1, 'potongan berurutan tanpa celah');
  }
});

test('toExportJSON: urut tanggal, label mood, entri tak terbaca ditandai', () => {
  const j = toExportJSON([e('2026-10-02', 2, 420, null), e('2026-10-01', 4, 450, ['Olahraga'], 'tenang')], '2026-10-11T00:00:00Z');
  assert.equal(j.app, 'Jurnal Mood');
  assert.equal(j.format, 1);
  assert.deepEqual(j.entries.map((r) => r.date), ['2026-10-01', '2026-10-02']);
  assert.deepEqual(j.entries[0], { date: '2026-10-01', mood: 4, moodLabel: 'Baik', sleepMinutes: 450, activities: ['Olahraga'], note: 'tenang', readable: true });
  assert.equal(j.entries[1].readable, false);
});

test('toCSV: BOM, header, escaping, dan pengamanan CSV injection', () => {
  const csv = toCSV([
    e('2026-10-01', 5, 480, ['Olahraga', 'Teman'], 'Hari "luar biasa",\nbaris kedua'),
    e('2026-10-02', 1, 360, ['=HYPERLINK("http://jahat")'], '+62 812 hubungi'),
    e('2026-10-03', 3, 420, null),
  ]);
  assert.ok(csv.startsWith('﻿tanggal,mood,label_mood,tidur_menit,tidur,aktivitas,catatan\r\n'));
  const lines = csv.slice(1).split('\r\n');
  // Titik koma juga dikutip: Excel berbahasa Indonesia memakai ; sebagai pemisah kolom.
  assert.equal(lines[1], '2026-10-01,5,Sangat baik,480,8 j,"Olahraga; Teman","Hari ""luar biasa"",\nbaris kedua"');
  assert.ok(lines[2].includes(`"'=HYPERLINK(""http://jahat"")"`), 'rumus diberi awalan petik');
  assert.ok(lines[2].endsWith(`'+62 812 hubungi`), 'awalan + juga diamankan');
  assert.ok(lines[3].endsWith('(tidak bisa dibuka)'));
});

test('nama file dan teks kunci pemulihan', () => {
  assert.equal(exportFileName('csv', '2026-10-11'), 'jurnal-mood-2026-10-11.csv');
  const t = recoveryFileText('AAAA-BBBB-CCCC-DDDD-EEEE-FFFF', '11 Oktober 2026');
  assert.ok(t.includes('Kunci: AAAA-BBBB-CCCC-DDDD-EEEE-FFFF'));
  assert.ok(t.includes('Lupa passphrase'));
});
