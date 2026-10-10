import assert from 'node:assert/strict';
import { test } from 'node:test';

import {
  activityMoodComparison, dailySeries, rangeSummary, recentLowRun, shouldShowGentleCard,
} from '../../web/static/js/history-logic.js';

const e = (date, mood, sleepMinutes = 420, activities) =>
  ({ date, mood, sleepMinutes, payload: activities ? { activities, note: '' } : null });

test('dailySeries mengisi hari kosong dengan null, termasuk lintas bulan', () => {
  const s = dailySeries([e('2026-09-30', 3), e('2026-10-02', 4)], '2026-09-29', '2026-10-02');
  assert.deepEqual(s.map((p) => p.date), ['2026-09-29', '2026-09-30', '2026-10-01', '2026-10-02']);
  assert.deepEqual(s.map((p) => p.mood), [null, 3, null, 4]);
});

test('recentLowRun: 3 hari berturut-turut skor <= 2 yang berakhir hari ini', () => {
  const run = recentLowRun([e('2026-10-08', 2), e('2026-10-09', 1), e('2026-10-10', 2)], '2026-10-10');
  assert.deepEqual(run, { start: '2026-10-08', end: '2026-10-10', length: 3 });
});

test('recentLowRun: boleh berakhir kemarin (hari ini belum diisi)', () => {
  const run = recentLowRun([e('2026-10-07', 1), e('2026-10-08', 2), e('2026-10-09', 2)], '2026-10-10');
  assert.equal(run.end, '2026-10-09');
});

test('recentLowRun: tidak memicu untuk 2 hari, hari bolong, skor 3, atau rangkaian lama', () => {
  assert.equal(recentLowRun([e('2026-10-09', 1), e('2026-10-10', 1)], '2026-10-10'), null);
  assert.equal(recentLowRun([e('2026-10-07', 1), e('2026-10-09', 1), e('2026-10-10', 1)], '2026-10-10'), null,
    'hari yang tidak diisi memutus rangkaian');
  assert.equal(recentLowRun([e('2026-10-08', 2), e('2026-10-09', 3), e('2026-10-10', 2)], '2026-10-10'), null);
  assert.equal(recentLowRun([e('2026-10-01', 1), e('2026-10-02', 1), e('2026-10-03', 1)], '2026-10-10'), null,
    'rangkaian yang sudah lewat tidak memicu');
  assert.equal(recentLowRun([e('2026-10-08', 2), e('2026-10-09', 2), e('2026-10-10', 4)], '2026-10-10'), null,
    'hari ini membaik: rangkaian tidak lagi terbaru');
});

test('recentLowRun lintas pergantian bulan', () => {
  const run = recentLowRun([e('2026-09-30', 1), e('2026-10-01', 2), e('2026-10-02', 1)], '2026-10-02');
  assert.deepEqual(run, { start: '2026-09-30', end: '2026-10-02', length: 3 });
});

test('kartu tidak muncul lagi setelah ditutup, kecuali untuk rangkaian baru', () => {
  const run = { start: '2026-10-08', end: '2026-10-10', length: 3 };
  assert.equal(shouldShowGentleCard(run, null), true);
  assert.equal(shouldShowGentleCard(run, '2026-10-08'), false);
  assert.equal(shouldShowGentleCard({ ...run, end: '2026-10-11', length: 4 }, '2026-10-08'), false,
    'rangkaian yang sama bertambah panjang: tetap tertutup');
  assert.equal(shouldShowGentleCard({ start: '2026-10-20', end: '2026-10-22', length: 3 }, '2026-10-08'), true);
  assert.equal(shouldShowGentleCard(null, null), false);
});

test('perbandingan aktivitas: rata-rata dengan vs tanpa, minimal 3 hari tiap sisi', () => {
  const entries = [
    e('2026-10-01', 5, 420, ['Olahraga']), e('2026-10-02', 4, 420, ['Olahraga', 'Kerja']),
    e('2026-10-03', 4, 420, ['Olahraga']), e('2026-10-04', 2, 420, ['Kerja']),
    e('2026-10-05', 3, 420, []), e('2026-10-06', 2, 420, ['Kerja']),
    e('2026-10-07', 1, 420), // tidak bisa didekripsi: dilewati
  ];
  const rows = activityMoodComparison(entries);
  const olahraga = rows.find((r) => r.activity === 'Olahraga');
  assert.deepEqual(olahraga, {
    activity: 'Olahraga', withAvg: 4.3, withDays: 3, withoutAvg: 2.3, withoutDays: 3, diff: 2,
  });
  const kerja = rows.find((r) => r.activity === 'Kerja');
  assert.equal(kerja.withDays, 3);
  assert.equal(rows[0].activity, 'Olahraga', 'diurutkan dari selisih terbesar');
});

test('perbandingan aktivitas dilewati jika data terlalu sedikit', () => {
  const rows = activityMoodComparison([e('2026-10-01', 5, 420, ['Olahraga']), e('2026-10-02', 3, 420, [])]);
  assert.deepEqual(rows, []);
});

test('rangeSummary', () => {
  const s = dailySeries([e('2026-10-01', 4, 420), e('2026-10-03', 2, 480)], '2026-10-01', '2026-10-07');
  assert.deepEqual(rangeSummary(s), { filledDays: 2, totalDays: 7, avgMood: 3, avgSleepMinutes: 450 });
  assert.equal(rangeSummary(dailySeries([], '2026-10-01', '2026-10-02')).avgMood, null);
});

test('monthGrid: minggu mulai Senin, sel kosong di awal & akhir', async () => {
  const { monthGrid } = await import('../../web/static/js/history-logic.js');
  const oct = monthGrid(2026, 9); // 1 Okt 2026 = Kamis
  assert.equal(oct.title, 'Oktober 2026');
  assert.deepEqual(oct.cells.slice(0, 4), [null, null, null, '2026-10-01']);
  assert.equal(oct.cells.length % 7, 0);
  assert.equal(oct.cells.filter(Boolean).length, 31);
  const feb = monthGrid(2027, 1); // Februari 2027: 1 Feb = Senin, 28 hari
  assert.equal(feb.cells[0], '2027-02-01');
  assert.equal(feb.cells.length, 28);
  assert.equal(monthGrid(2028, 1).cells.filter(Boolean).length, 29, 'tahun kabisat');
});

test('longDate dan greeting', async () => {
  const { longDate, greeting } = await import('../../web/static/js/history-logic.js');
  assert.equal(longDate('2026-10-10'), 'Sabtu, 10 Oktober 2026');
  assert.equal(greeting(6), 'Selamat pagi');
  assert.equal(greeting(12), 'Selamat siang');
  assert.equal(greeting(16), 'Selamat sore');
  assert.equal(greeting(21), 'Selamat malam');
  assert.equal(greeting(2), 'Selamat malam');
});

test('weekStats: 7 hari terakhir vs 7 hari sebelumnya', async () => {
  const { weekStats } = await import('../../web/static/js/history-logic.js');
  const s = weekStats([
    e('2026-10-10', 4, 480), e('2026-10-08', 2, 420), // minggu ini
    e('2026-10-02', 3, 360),                          // minggu lalu
  ], '2026-10-10');
  assert.equal(s.filledDays, 2);
  assert.equal(s.avgMood, 3);
  assert.equal(s.avgSleepMinutes, 450);
  assert.equal(s.prevAvgSleepMinutes, 360);
  assert.equal(s.sleepDelta, 90);
  const empty = weekStats([], '2026-10-10');
  assert.equal(empty.avgMood, null);
  assert.equal(empty.sleepDelta, null);
});
