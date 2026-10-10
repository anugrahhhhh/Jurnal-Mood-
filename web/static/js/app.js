// Titik masuk JavaScript. Dimuat sekali di <head>; karena navigasi memakai
// hx-boost, modul ini (dan kunci jurnal di memori) bertahan antarhalaman.
//
// Form jurnal tidak punya atribut name pada input passphrase, jadi tanpa
// JavaScript pun passphrase tidak mungkin ikut terkirim ke server.

import {
  changePassphrase, createJournalKey, materialFromJSON, materialToJSON,
  MIN_PASSPHRASE_LENGTH, PassphraseTooShortError, unlockJournalKey, WrongPassphraseError, wrapToJSON,
} from './crypto.js';
import { initDashboard } from './dashboard.js';
import { initEntryPage } from './entry-form.js';
import { initGentleCard } from './gentle.js';
import { initHistoryPage } from './history.js';
import { initReminders } from './reminders.js';
import { api, ApiError, ctx, navigate, toast } from './http.js';
import { passphraseStrength } from './entry-logic.js';
import { getKey, lock, setKey } from './session-key.js';

document.addEventListener('htmx:configRequest', (e) => {
  const c = ctx();
  if (c) e.detail.headers['X-CSRF-Token'] = c.csrf;
});

document.addEventListener('htmx:responseError', (e) => {
  const status = e.detail.xhr ? e.detail.xhr.status : 0;
  toast(status === 403
    ? 'Permintaan ditolak. Muat ulang halaman lalu coba lagi.'
    : 'Terjadi kesalahan. Coba lagi sebentar lagi.');
});

// Hanya path internal, untuk parameter ?lanjut=
function safeNext(value, fallback) {
  return value && /^\/(?!\/)[^\\]*$/.test(value) && !value.startsWith('/jurnal/') ? value : fallback;
}

// ---- Helper form ----

function field(form, name) {
  return form.querySelector(`[data-field="${name}"]`);
}

function setError(form, message) {
  const el = form.querySelector('[data-role="error"]');
  el.textContent = message || '';
  el.hidden = !message;
}

function setBusy(form, busy, label) {
  const btn = form.querySelector('button[type="submit"]');
  if (!btn.dataset.label) btn.dataset.label = btn.textContent;
  btn.disabled = busy;
  btn.textContent = busy ? label : btn.dataset.label;
  form.setAttribute('aria-busy', busy ? 'true' : 'false');
}

function clearSecrets(form) {
  form.querySelectorAll('input[data-secret]').forEach((i) => {
    i.value = '';
    setVisible(i, false);
  });
}

// ---- Kolom password: tombol tampilkan/sembunyikan & indikator kekuatan ----

function setVisible(input, show) {
  input.type = show ? 'text' : 'password';
  const btn = input.parentElement && input.parentElement.querySelector('.pw-toggle');
  if (btn) {
    btn.textContent = show ? 'Sembunyikan' : 'Tampilkan';
    btn.setAttribute('aria-pressed', String(show));
    btn.setAttribute('aria-label', show ? 'Sembunyikan isian' : 'Tampilkan isian');
  }
}

function renderStrength(input, meter) {
  const { level, label } = passphraseStrength(input.value);
  meter.dataset.level = input.value ? String(level) : 'kosong';
  meter.querySelector('span').textContent = input.value ? label : 'Minimal 12 karakter. Tips: 4 kata acak atau lebih.';
}

function enhanceForms() {
  document.querySelectorAll('main input[type="password"]:not([data-secret])').forEach((input) => {
    input.dataset.secret = '1';
    const wrap = document.createElement('span');
    wrap.className = 'pw-wrap';
    input.replaceWith(wrap);
    wrap.appendChild(input);
    const btn = document.createElement('button');
    btn.type = 'button';
    btn.className = 'pw-toggle';
    wrap.appendChild(btn);
    setVisible(input, false);
    btn.addEventListener('click', () => {
      setVisible(input, input.type === 'password');
      input.focus();
    });

    if (input.hasAttribute('data-strength')) {
      const meter = document.createElement('span');
      meter.className = 'strength';
      meter.setAttribute('aria-live', 'polite');
      meter.append(document.createElement('i'), document.createElement('span'));
      wrap.after(meter);
      renderStrength(input, meter);
      input.addEventListener('input', () => renderStrength(input, meter));
    }
  });
}

// Kesalahan isian yang pesannya aman dan berguna untuk ditampilkan apa adanya.
class FormError extends Error {}

function messageFor(err) {
  if (err instanceof FormError || err instanceof WrongPassphraseError || err instanceof PassphraseTooShortError) {
    return err.message;
  }
  if (err instanceof ApiError) {
    if (err.status === 401) return 'Sesi login berakhir. Silakan masuk lagi.';
    return err.message;
  }
  console.error(err);
  return 'Terjadi kesalahan. Coba lagi.';
}

// ---- Form jurnal ----

const handlers = {
  async 'journal-create'(form, c) {
    const pass = field(form, 'passphrase').value;
    if ([...pass].length < MIN_PASSPHRASE_LENGTH) throw new PassphraseTooShortError();
    if (pass !== field(form, 'passphrase-confirm').value) throw new FormError('Konfirmasi passphrase tidak sama.');
    if (!field(form, 'understand').checked) throw new FormError('Centang pernyataan di atas untuk melanjutkan.');

    setBusy(form, true, 'Membuat kunci… (beberapa detik)');
    const { material, dek } = await createJournalKey(pass, c.userId);
    await api('POST', '/api/jurnal/kunci', materialToJSON(material));
    setKey(c.userId, dek);
    clearSecrets(form);
    await navigate('/hari-ini?mulai=1'); // langkah 3 onboarding: catatan pertama
  },

  async 'journal-unlock'(form, c) {
    const pass = field(form, 'passphrase').value;
    setBusy(form, true, 'Membuka…');
    const material = materialFromJSON(await api('GET', '/api/jurnal/kunci'));
    const dek = await unlockJournalKey(pass, c.userId, material);
    setKey(c.userId, dek);
    clearSecrets(form);
    const next = safeNext(new URLSearchParams(location.search).get('lanjut'), '/hari-ini');
    await navigate(next);
  },

  async 'journal-change'(form, c) {
    const oldPass = field(form, 'old-passphrase').value;
    const newPass = field(form, 'new-passphrase').value;
    if (newPass !== field(form, 'new-passphrase-confirm').value) throw new FormError('Konfirmasi passphrase baru tidak sama.');
    if (newPass === oldPass) throw new FormError('Passphrase baru harus berbeda dari yang lama.');
    const loginPassword = field(form, 'login-password').value;

    setBusy(form, true, 'Mengganti… (beberapa detik)');
    const material = materialFromJSON(await api('GET', '/api/jurnal/kunci'));
    const { wrap, dek } = await changePassphrase(oldPass, newPass, c.userId, material);
    await api('PUT', '/api/jurnal/kunci', { loginPassword, ...wrapToJSON(wrap) });
    setKey(c.userId, dek);
    clearSecrets(form);
    await navigate('/beranda?passphrase=diganti');
  },
};

document.addEventListener('submit', async (e) => {
  const form = e.target.closest('form[data-form]');
  if (!form || !handlers[form.dataset.form]) return;
  e.preventDefault();
  const c = ctx();
  if (!c) return;

  setError(form, '');
  try {
    await handlers[form.dataset.form](form, c);
  } catch (err) {
    setError(form, messageFor(err));
    if (err instanceof WrongPassphraseError) {
      const input = form.querySelector('input[data-secret]');
      if (input) { input.value = ''; input.focus(); }
    }
  } finally {
    if (document.body.contains(form)) setBusy(form, false);
  }
});

document.addEventListener('click', (e) => {
  if (e.target.closest('[data-action="lock"]')) {
    e.preventDefault();
    lock({ reason: 'manual' });
  }
});

// ---- Inisialisasi setiap halaman ----

function init() {
  const c = ctx();
  const unlocked = Boolean(c && getKey(c.userId));
  document.documentElement.dataset.journal = unlocked ? 'unlocked' : 'locked';
  delete document.documentElement.dataset.mood; // warna mood hanya di layar harian

  // Halaman yang butuh jurnal terbuka: arahkan ke halaman buka kunci.
  if (document.querySelector('[data-requires="unlocked"]') && !unlocked) {
    const q = new URLSearchParams({ lanjut: location.pathname + location.search });
    navigate('/jurnal/buka?' + q.toString());
    return;
  }

  const notice = new URLSearchParams(location.search).get('terkunci');
  const lockedMsg = document.querySelector('[data-role="locked-reason"]');
  if (lockedMsg && notice) {
    lockedMsg.textContent = notice === 'idle'
      ? 'Jurnal dikunci otomatis karena tidak ada aktivitas selama 15 menit.'
      : 'Jurnal sudah dikunci.';
    lockedMsg.hidden = false;
  }

  enhanceForms();
  const autofocus = document.querySelector('main [data-autofocus]');
  if (autofocus) autofocus.focus();

  const fail = (msg) => (err) => {
    console.error(err);
    toast(msg);
  };

  const entryPage = document.querySelector('[data-page="entry"]');
  if (entryPage && unlocked) {
    initEntryPage(entryPage, c, getKey(c.userId)).catch(fail('Gagal memuat catatan. Muat ulang halaman lalu coba lagi.'));
  }
  const dashboard = document.querySelector('[data-page="dashboard"]');
  if (dashboard && c) {
    initDashboard(dashboard, c, unlocked ? getKey(c.userId) : null).catch(fail('Gagal memuat beranda. Muat ulang halaman lalu coba lagi.'));
  }
  const historyPage = document.querySelector('[data-page="history"]');
  if (historyPage && unlocked) {
    initHistoryPage(historyPage, c, getKey(c.userId)).catch(fail('Gagal memuat riwayat. Muat ulang halaman lalu coba lagi.'));
  }
  const remindersPage = document.querySelector('[data-page="reminders"]');
  if (remindersPage && c) {
    initReminders(remindersPage).catch((err) => {
      console.error(err);
      const s = remindersPage.querySelector('[data-role="reminder-status"]');
      if (s) s.textContent = 'Pengingat tidak bisa dimuat di browser ini.';
    });
  }

  showGentleCard();
  clearDeletedAccountStorage();
}

// Setelah akun dihapus: buang draf terenkripsi dan status kartu milik akun itu
// dari localStorage. Data akun lain di perangkat yang sama tidak disentuh.
function clearDeletedAccountStorage() {
  const el = document.querySelector('[data-clear-user]');
  const id = el && el.dataset.clearUser;
  if (!id || !/^\d+$/.test(id) || id === '0') return;
  try {
    for (const key of Object.keys(localStorage)) {
      if (key.startsWith(`mj-draft:${id}:`) || key === `mj-gentle-dismissed:${id}`) localStorage.removeItem(key);
    }
  } catch { /* penyimpanan diblokir: tidak ada yang perlu dibersihkan */ }
}

function showGentleCard() {
  const c = ctx();
  const card = document.querySelector('[data-role="gentle-card"]');
  if (c && card) initGentleCard(card, c).catch((err) => console.error(err));
}

document.addEventListener('mj:entry-saved', showGentleCard);

document.addEventListener('DOMContentLoaded', init);
document.addEventListener('htmx:afterSettle', (e) => {
  if (e.detail.target === document.body) init();
});
document.addEventListener('htmx:historyRestore', init);
