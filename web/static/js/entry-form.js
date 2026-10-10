// Layar catatan harian (SPEC 5, tampilan variasi B).
//
// Mood dan jam tidur dikirim biasa. Aktivitas dan catatan dienkripsi di sini
// sebelum dikirim. Draf otomatis disimpan di localStorage dalam bentuk
// terenkripsi saja.

import {
  decryptEntry, decryptValue, encryptEntry, encryptValue, fromBase64, toBase64,
} from './crypto.js';
import {
  ACTIVITY_ICONS, addActivity, addDays, DEFAULT_ACTIVITIES, formatSleep, initialSleep, isoDate,
  MAX_NOTE_LENGTH, MOODS, sleepPercent, SLEEP_STEP, stepSleep, weekStrip,
} from './entry-logic.js';
import { longDate } from './history-logic.js';
import { api, ApiError, toast } from './http.js';

const DRAFT_DELAY_MS = 800;

const draftKey = (userId, date) => `mj-draft:${userId}:${date}`;

function readStored(key) {
  try {
    const raw = localStorage.getItem(key);
    if (!raw) return null;
    const j = JSON.parse(raw);
    return { iv: fromBase64(j.iv), ciphertext: fromBase64(j.ct) };
  } catch {
    return null;
  }
}

function writeStored(key, blob) {
  try {
    localStorage.setItem(key, JSON.stringify({ iv: toBase64(blob.iv), ct: toBase64(blob.ciphertext) }));
  } catch { /* penyimpanan penuh atau diblokir: draf dilewati */ }
}

function removeStored(key) {
  try { localStorage.removeItem(key); } catch { /* abaikan */ }
}

async function loadActivities(userId, dek) {
  const b = await api('GET', '/api/aktivitas'); // null (204) = belum ada daftar sendiri
  if (!b) return [...DEFAULT_ACTIVITIES];
  return decryptValue(dek, userId, 'activities', { iv: fromBase64(b.iv), ciphertext: fromBase64(b.ct) });
}

async function loadEntries(userId, dek, from, to) {
  const res = await api('GET', `/api/entri?dari=${from}&sampai=${to}`);
  const list = await Promise.all(res.entries.map(async (e) => {
    let payload = null;
    try {
      payload = await decryptEntry(dek, userId, e.date,
        { iv: fromBase64(e.payloadIv), ciphertext: fromBase64(e.payloadCt) });
    } catch {
      payload = null; // tidak bisa didekripsi (mis. data rusak); mood & tidur tetap tampil
    }
    return { date: e.date, mood: e.mood, sleepMinutes: e.sleepMinutes, payload };
  }));
  return { entries: list, sleepBeforeFrom: res.sleepBeforeFrom };
}

export async function initEntryPage(root, c, dek) {
  const $ = (sel) => root.querySelector(sel);
  const userId = c.userId;
  const today = isoDate(new Date());
  const yesterday = addDays(today, -1);

  const [{ entries, sleepBeforeFrom }, activityList] = await Promise.all([
    loadEntries(userId, dek, addDays(today, -7), today),
    loadActivities(userId, dek),
  ]);

  const state = {
    date: today,
    mood: null,
    sleep: 0,
    selected: new Set(),
    note: '',
    activities: activityList,
    editingChips: false,
    dirty: false,
  };
  let draftTimer = null;

  // ---- Memuat tanggal ----

  async function loadDate(date) {
    state.date = date;
    const saved = entries.find((e) => e.date === date);
    state.mood = saved ? saved.mood : null;
    state.sleep = initialSleep(entries, date, sleepBeforeFrom);
    state.selected = new Set(saved && saved.payload ? saved.payload.activities : []);
    state.note = saved && saved.payload ? saved.payload.note : '';
    state.dirty = false;

    let notice = '';
    if (saved && !saved.payload) notice = 'Isi catatan untuk tanggal ini tidak bisa dibuka.';
    const draft = readStored(draftKey(userId, date));
    if (draft) {
      try {
        const d = await decryptValue(dek, userId, `draft:${date}`, draft);
        Object.assign(state, { mood: d.mood, sleep: d.sleep, note: d.note, dirty: true });
        state.selected = new Set(d.activities);
        notice = 'Draf yang belum disimpan dipulihkan.';
      } catch {
        removeStored(draftKey(userId, date));
      }
    }
    for (const a of state.selected) state.activities = addActivity(state.activities, a);

    $('[data-role="date-today"]').setAttribute('aria-pressed', String(date === today));
    $('[data-role="date-yesterday"]').setAttribute('aria-pressed', String(date === yesterday));
    $('[data-role="title"]').textContent = date === today ? 'Bagaimana harimu?' : 'Bagaimana harimu kemarin?';
    $('[data-role="date-label"]').textContent = longDate(date);
    markSaved(Boolean(saved));
    $('textarea[data-field="note"]').value = state.note;
    if (state.note) $('[data-role="note-box"]').open = true;
    setStatus(notice);
    $('[data-role="saved-summary"]').hidden = true;
    renderAll();
  }

  // ---- Tampilan ----

  function renderMood() {
    const m = MOODS[state.mood];
    $('[data-role="mood-face"]').textContent = m ? m.face : '◌';
    $('[data-role="mood-label"]').textContent = m ? m.label : 'Pilih mood';
    const range = $('input[data-field="mood"]');
    range.value = String(state.mood || 3);
    range.classList.toggle('untouched', !state.mood);
    range.setAttribute('aria-valuetext', m ? `${state.mood} dari 5, ${m.label}` : 'Belum dipilih');
    root.querySelectorAll('[data-role="mood-tick"]').forEach((b) => {
      b.setAttribute('aria-pressed', String(Number(b.dataset.mood) === state.mood));
    });
    if (state.mood) document.documentElement.dataset.mood = String(state.mood);
    else delete document.documentElement.dataset.mood;
    $('[data-action="save"]').disabled = !state.mood;
  }

  function renderSleep() {
    $('[data-role="sleep-value"]').textContent = formatSleep(state.sleep);
    $('[data-role="sleep-ring"]').style.setProperty('--pct', `${Math.min(100, sleepPercent(state.sleep))}%`);
  }

  function renderChips() {
    const box = $('[data-role="chips"]');
    box.replaceChildren();
    for (const name of state.activities) {
      const b = document.createElement('button');
      b.type = 'button';
      b.className = 'chip';
      b.dataset.chip = name;
      const custom = !DEFAULT_ACTIVITIES.includes(name);
      if (state.editingChips && custom) {
        b.classList.add('removable');
        b.setAttribute('aria-label', `Hapus aktivitas ${name}`);
        b.textContent = `${name} ✕`;
      } else {
        b.setAttribute('aria-pressed', String(state.selected.has(name)));
        b.textContent = ACTIVITY_ICONS[name] ? `${ACTIVITY_ICONS[name]} ${name}` : name;
      }
      box.appendChild(b);
    }
    $('[data-action="chip-edit"]').textContent = state.editingChips ? 'Selesai' : 'Ubah daftar';
  }

  function renderWeek(target) {
    target.replaceChildren();
    for (const d of weekStrip(entries, today)) {
      const cell = document.createElement('span');
      cell.className = 'day';
      const face = document.createElement('b');
      face.textContent = d.mood ? MOODS[d.mood].face : '◌';
      if (!d.mood) face.className = 'empty';
      cell.append(face, d.day);
      cell.title = d.mood ? `${d.date}: ${MOODS[d.mood].label}` : `${d.date}: belum diisi`;
      target.appendChild(cell);
    }
  }

  function renderAll() {
    renderMood();
    renderSleep();
    renderChips();
    renderWeek($('[data-role="week"]'));
  }

  function markSaved(isSaved) {
    $('[data-role="saved-note"]').hidden = !isSaved;
    $('[data-action="save"]').textContent = isSaved ? 'Simpan perubahan' : 'Simpan';
  }

  function setStatus(text) {
    const el = $('[data-role="status"]');
    el.textContent = text;
    el.hidden = !text;
  }

  // ---- Draf terenkripsi ----

  function changed() {
    state.dirty = true;
    clearTimeout(draftTimer);
    draftTimer = setTimeout(saveDraft, DRAFT_DELAY_MS);
  }

  async function saveDraft() {
    if (!root.isConnected || !state.dirty) return;
    const value = { mood: state.mood, sleep: state.sleep, activities: [...state.selected], note: state.note };
    writeStored(draftKey(userId, state.date), await encryptValue(dek, userId, `draft:${state.date}`, value));
  }

  // ---- Interaksi ----

  root.addEventListener('click', async (e) => {
    const t = e.target.closest('button');
    if (!t || !root.contains(t)) return;

    if (t.dataset.role === 'date-today' && state.date !== today) await loadDate(today);
    else if (t.dataset.role === 'date-yesterday' && state.date !== yesterday) await loadDate(yesterday);
    else if (t.dataset.role === 'mood-tick') {
      state.mood = Number(t.dataset.mood);
      renderMood();
      changed();
    } else if (t.dataset.action === 'sleep-minus' || t.dataset.action === 'sleep-plus') {
      state.sleep = stepSleep(state.sleep, t.dataset.action === 'sleep-plus' ? SLEEP_STEP : -SLEEP_STEP);
      renderSleep();
      changed();
    } else if (t.dataset.chip !== undefined) {
      const name = t.dataset.chip;
      if (state.editingChips && !DEFAULT_ACTIVITIES.includes(name)) {
        state.activities = state.activities.filter((a) => a !== name);
        state.selected.delete(name);
        await saveActivityList();
      } else if (state.selected.has(name)) {
        state.selected.delete(name);
      } else {
        state.selected.add(name);
      }
      renderChips();
      changed();
    } else if (t.dataset.action === 'chip-add') {
      await addChipFromInput();
    } else if (t.dataset.action === 'chip-edit') {
      state.editingChips = !state.editingChips;
      renderChips();
    } else if (t.dataset.action === 'save') {
      await save(t);
    }
  });

  $('input[data-field="mood"]').addEventListener('input', (e) => {
    state.mood = Number(e.target.value);
    renderMood();
    changed();
  });

  const note = $('textarea[data-field="note"]');
  note.maxLength = MAX_NOTE_LENGTH;
  note.addEventListener('input', () => {
    state.note = note.value;
    changed();
  });

  const chipInput = $('[data-role="chip-new"]');
  chipInput.addEventListener('keydown', async (e) => {
    if (e.key === 'Enter') {
      e.preventDefault();
      await addChipFromInput();
    }
  });

  async function addChipFromInput() {
    const before = state.activities;
    state.activities = addActivity(state.activities, chipInput.value);
    chipInput.value = '';
    if (state.activities !== before) {
      state.selected.add(state.activities[state.activities.length - 1]);
      renderChips();
      changed();
      await saveActivityList();
    }
  }

  async function saveActivityList() {
    try {
      const blob = await encryptValue(dek, userId, 'activities', state.activities);
      await api('PUT', '/api/aktivitas', { ct: toBase64(blob.ciphertext), iv: toBase64(blob.iv) });
    } catch (err) {
      console.error(err);
      toast('Daftar aktivitas belum tersimpan. Coba lagi nanti.');
    }
  }

  async function save(button) {
    if (!state.mood) return;
    button.disabled = true;
    setStatus('Menyimpan…');
    try {
      const payload = { activities: [...state.selected], note: state.note.trim() };
      const blob = await encryptEntry(dek, userId, state.date, payload);
      await api('PUT', `/api/entri/${state.date}`, {
        mood: state.mood, sleepMinutes: state.sleep, version: 1,
        payloadCt: toBase64(blob.ciphertext), payloadIv: toBase64(blob.iv),
      });
      clearTimeout(draftTimer);
      removeStored(draftKey(userId, state.date));
      state.dirty = false;

      const saved = { date: state.date, mood: state.mood, sleepMinutes: state.sleep, payload };
      const i = entries.findIndex((e) => e.date === state.date);
      if (i >= 0) entries[i] = saved;
      else entries.push(saved);

      setStatus('Tersimpan ✓');
      markSaved(true);
      renderWeek($('[data-role="week"]'));
      const summary = $('[data-role="saved-summary"]');
      renderWeek(summary.querySelector('[data-role="summary-week"]'));
      summary.hidden = false;
      summary.scrollIntoView({ behavior: 'smooth', block: 'nearest' });
      document.dispatchEvent(new CustomEvent('mj:entry-saved'));
    } catch (err) {
      console.error(err);
      setStatus('');
      toast(err instanceof ApiError ? err.message : 'Gagal menyimpan. Drafmu tetap aman di perangkat ini.');
    } finally {
      button.disabled = !state.mood;
    }
  }

  // Parameter URL: ?tanggal= (dari riwayat, hanya kemarin), ?mood=1..5 (dari
  // wajah yang diketuk di beranda), ?mulai=1 (langkah terakhir onboarding).
  const params = new URLSearchParams(location.search);
  const startDate = params.get('tanggal') === yesterday ? yesterday : today;
  await loadDate(startDate);
  const preset = Number(params.get('mood'));
  if (preset >= 1 && preset <= 5 && !entries.some((e) => e.date === startDate) && !state.dirty) {
    state.mood = preset;
    renderMood();
    changed();
  }
  const onboarding = document.querySelector('[data-role="onboarding"]');
  if (onboarding) onboarding.hidden = params.get('mulai') !== '1';
}
