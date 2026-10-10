import assert from 'node:assert/strict';
import { test } from 'node:test';

import {
  addActivity, addDays, formatSleep, initialSleep, isoDate, normalizeActivityName,
  SLEEP_DEFAULT, stepSleep, weekStrip,
} from '../../web/static/js/entry-logic.js';

test('isoDate memakai tanggal lokal', () => {
  assert.equal(isoDate(new Date(2026, 0, 5, 23, 59)), '2026-01-05');
});

test('addDays aman untuk pergantian bulan dan tahun', () => {
  assert.equal(addDays('2026-03-01', -1), '2026-02-28');
  assert.equal(addDays('2028-03-01', -1), '2028-02-29');
  assert.equal(addDays('2026-12-31', 1), '2027-01-01');
  assert.equal(addDays('2026-10-10', -6), '2026-10-04');
});

test('stepSleep kelipatan 30 menit dan dibatasi 0–24 jam', () => {
  assert.equal(stepSleep(480, 30), 510);
  assert.equal(stepSleep(0, -30), 0);
  assert.equal(stepSleep(1440, 30), 1440);
  assert.equal(stepSleep(445, 0), 450);
});

test('formatSleep', () => {
  assert.equal(formatSleep(450), '7 j 30 m');
  assert.equal(formatSleep(480), '8 j');
  assert.equal(formatSleep(0), '0 j');
});

test('initialSleep: entri sendiri, lalu entri sebelumnya, lalu server, lalu default', () => {
  const entries = [
    { date: '2026-10-07', sleepMinutes: 360 },
    { date: '2026-10-09', sleepMinutes: 420 },
    { date: '2026-10-10', sleepMinutes: 510 },
  ];
  assert.equal(initialSleep(entries, '2026-10-10', 300), 510);
  assert.equal(initialSleep(entries.slice(0, 2), '2026-10-10', 300), 420);
  assert.equal(initialSleep([], '2026-10-10', 300), 300);
  assert.equal(initialSleep([], '2026-10-10', null), SLEEP_DEFAULT);
  assert.equal(initialSleep([{ date: '2026-10-11', sleepMinutes: 600 }], '2026-10-10', null), SLEEP_DEFAULT,
    'entri sesudah tanggal itu tidak dipakai');
});

test('weekStrip: 7 hari berurutan, hari kosong bernilai null, tanpa streak', () => {
  const strip = weekStrip([{ date: '2026-10-10', mood: 4 }, { date: '2026-10-06', mood: 2 }], '2026-10-10');
  assert.equal(strip.length, 7);
  assert.equal(strip[0].date, '2026-10-04');
  assert.equal(strip[6].date, '2026-10-10');
  assert.equal(strip[6].mood, 4);
  assert.equal(strip[2].mood, 2);
  assert.equal(strip[5].mood, null);
  assert.equal(strip[6].day, 'Sab'); // 10 Okt 2026 = Sabtu
  assert.ok(!('streak' in strip[6]));
});

test('aktivitas: dirapikan, dibatasi, tanpa duplikat', () => {
  assert.equal(normalizeActivityName('  baca   buku '), 'baca buku');
  assert.equal(normalizeActivityName('x'.repeat(50)).length, 30);
  const list = ['Olahraga'];
  assert.deepEqual(addActivity(list, 'olahraga'), list);
  assert.deepEqual(addActivity(list, '   '), list);
  assert.deepEqual(addActivity(list, 'Baca buku'), ['Olahraga', 'Baca buku']);
  assert.deepEqual(list, ['Olahraga'], 'daftar asli tidak diubah');
});

test('formatDuration tanpa "0 j"', async () => {
  const { formatDuration } = await import('../../web/static/js/entry-logic.js');
  assert.equal(formatDuration(30), '30 m');
  assert.equal(formatDuration(-30), '30 m');
  assert.equal(formatDuration(60), '1 j');
  assert.equal(formatDuration(90), '1 j 30 m');
  assert.equal(formatDuration(0), '0 m');
});

test('passphraseStrength', async () => {
  const { passphraseStrength } = await import('../../web/static/js/entry-logic.js');
  assert.equal(passphraseStrength('pendek').level, 0);
  assert.match(passphraseStrength('pendek').label, /6\/12/);
  assert.equal(passphraseStrength('abcdefghijkl').level, 1);
  assert.equal(passphraseStrength('kopi jendela hujan sepeda').level, 2);
  assert.equal(passphraseStrength('a'.repeat(20)).level, 2);
  assert.equal(passphraseStrength('kafé'.repeat(3)).level, 1, 'dihitung per karakter, bukan byte');
});
