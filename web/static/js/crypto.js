// Enkripsi jurnal di browser (SPEC 3.2). Hanya memakai Web Crypto API bawaan.
//
// Skema:
//   passphrase --PBKDF2-SHA-256, salt 16 byte--> KEK (AES-256-GCM, wrap/unwrap)
//   DEK: kunci acak AES-256-GCM, dibuat sekali per pengguna
//   server menyimpan: AES-GCM(KEK, DEK) + IV, dan nilai uji AES-GCM(DEK, konstanta)
//   setiap entri: AES-GCM(DEK, JSON) dengan IV acak 12 byte yang baru setiap kali simpan
//
// Associated data (AAD) mengikat setiap ciphertext ke tujuannya (pengguna,
// tanggal), sehingga blob tidak bisa dipindah diam-diam ke entri lain.
//
// Modul ini tidak menyentuh DOM agar bisa diuji di Node (tests/js).

export const FORMAT_VERSION = 1;
export const KDF = 'PBKDF2-SHA256';
export const ITERATIONS = 600_000;
export const MIN_PASSPHRASE_LENGTH = 12;

const VERIFIER_PLAINTEXT = 'mood-journal-verifier-v1';
const AES = { name: 'AES-GCM', length: 256 };

const encoder = new TextEncoder();
const decoder = new TextDecoder();
const subtle = () => globalThis.crypto.subtle;
const randomBytes = (n) => globalThis.crypto.getRandomValues(new Uint8Array(n));

export class WrongPassphraseError extends Error {
  constructor() {
    super('Passphrase salah.');
    this.name = 'WrongPassphraseError';
  }
}

export class PassphraseTooShortError extends Error {
  constructor() {
    super(`Passphrase minimal ${MIN_PASSPHRASE_LENGTH} karakter.`);
    this.name = 'PassphraseTooShortError';
  }
}

function aad(...parts) {
  return encoder.encode(['mj', `v${FORMAT_VERSION}`, ...parts].join('|'));
}

function checkPassphrase(passphrase) {
  if ([...passphrase].length < MIN_PASSPHRASE_LENGTH) throw new PassphraseTooShortError();
}

// NFC agar passphrase yang sama tetap menghasilkan kunci yang sama di
// perangkat yang menyusun huruf beraksen secara berbeda.
async function deriveKEK(passphrase, salt, iterations) {
  const base = await subtle().importKey(
    'raw', encoder.encode(passphrase.normalize('NFC')), 'PBKDF2', false, ['deriveKey']);
  return subtle().deriveKey(
    { name: 'PBKDF2', hash: 'SHA-256', salt, iterations }, base, AES, false, ['wrapKey', 'unwrapKey']);
}

async function wrapDEK(extractableDEK, passphrase, userId, iterations) {
  const salt = randomBytes(16);
  const wrapIv = randomBytes(12);
  const kek = await deriveKEK(passphrase, salt, iterations);
  const wrapped = await subtle().wrapKey('raw', extractableDEK, kek,
    { name: 'AES-GCM', iv: wrapIv, additionalData: aad('dek', `user:${userId}`) });
  return { kdf: KDF, iterations, salt, wrappedKey: new Uint8Array(wrapped), wrapIv };
}

// Salinan DEK yang tidak bisa diekspor, untuk disimpan di memori selama sesi.
async function toNonExtractable(extractableDEK) {
  const raw = new Uint8Array(await subtle().exportKey('raw', extractableDEK));
  try {
    return await subtle().importKey('raw', raw, AES, false, ['encrypt', 'decrypt']);
  } finally {
    raw.fill(0);
  }
}

async function unwrapDEK(passphrase, userId, material, extractable) {
  if (material.version !== FORMAT_VERSION || material.kdf !== KDF) {
    throw new Error('Format kunci jurnal tidak dikenal.');
  }
  const kek = await deriveKEK(passphrase, material.salt, material.iterations);
  let dek;
  try {
    dek = await subtle().unwrapKey('raw', material.wrappedKey, kek,
      { name: 'AES-GCM', iv: material.wrapIv, additionalData: aad('dek', `user:${userId}`) },
      AES, extractable, ['encrypt', 'decrypt']);
  } catch {
    throw new WrongPassphraseError(); // tag GCM tidak cocok: passphrase salah
  }

  let ok = false;
  try {
    const pt = await subtle().decrypt(
      { name: 'AES-GCM', iv: material.verifierIv, additionalData: aad('verifier', `user:${userId}`) },
      dek, material.verifier);
    ok = decoder.decode(pt) === VERIFIER_PLAINTEXT;
  } catch {
    ok = false;
  }
  if (!ok) throw new Error('Kunci jurnal tidak cocok dengan nilai ujinya (data kunci rusak).');
  return dek;
}

/**
 * Membuat kunci jurnal baru.
 * @returns {{material: object, dek: CryptoKey}} material untuk dikirim ke
 *   server (lihat materialToJSON) dan DEK non-extractable untuk sesi ini.
 */
export async function createJournalKey(passphrase, userId, { iterations = ITERATIONS } = {}) {
  checkPassphrase(passphrase);
  const extractable = await subtle().generateKey(AES, true, ['encrypt', 'decrypt']);
  const wrap = await wrapDEK(extractable, passphrase, userId, iterations);
  const verifierIv = randomBytes(12);
  const verifier = new Uint8Array(await subtle().encrypt(
    { name: 'AES-GCM', iv: verifierIv, additionalData: aad('verifier', `user:${userId}`) },
    extractable, encoder.encode(VERIFIER_PLAINTEXT)));
  const dek = await toNonExtractable(extractable);
  return { material: { version: FORMAT_VERSION, ...wrap, verifier, verifierIv }, dek };
}

/** Membuka kunci jurnal. Melempar WrongPassphraseError jika passphrase salah. */
export async function unlockJournalKey(passphrase, userId, material) {
  return unwrapDEK(passphrase, userId, material, false);
}

/**
 * Mengganti passphrase: DEK yang sama dibungkus ulang dengan passphrase baru.
 * Entri lama tidak perlu dienkripsi ulang.
 * @returns {{wrap: object, dek: CryptoKey}}
 */
export async function changePassphrase(oldPassphrase, newPassphrase, userId, material,
  { iterations = ITERATIONS } = {}) {
  checkPassphrase(newPassphrase);
  const extractable = await unwrapDEK(oldPassphrase, userId, material, true);
  const wrap = await wrapDEK(extractable, newPassphrase, userId, iterations);
  const dek = await toNonExtractable(extractable);
  return { wrap, dek };
}

// ---- Enkripsi data ----

async function encryptJSON(dek, context, value) {
  const iv = randomBytes(12);
  const ct = await subtle().encrypt({ name: 'AES-GCM', iv, additionalData: aad(...context) },
    dek, encoder.encode(JSON.stringify(value)));
  return { iv, ciphertext: new Uint8Array(ct) };
}

async function decryptJSON(dek, context, { iv, ciphertext }) {
  const pt = await subtle().decrypt({ name: 'AES-GCM', iv, additionalData: aad(...context) }, dek, ciphertext);
  return JSON.parse(decoder.decode(pt));
}

const entryContext = (userId, date) => ['entry', `user:${userId}`, `date:${date}`];

/** Mengenkripsi isi entri (aktivitas + catatan). date: 'YYYY-MM-DD'. */
export function encryptEntry(dek, userId, date, payload) {
  return encryptJSON(dek, entryContext(userId, date), payload);
}

export function decryptEntry(dek, userId, date, blob) {
  return decryptJSON(dek, entryContext(userId, date), blob);
}

/** Untuk data terenkripsi lain milik pengguna, mis. draf atau daftar chip. */
export function encryptValue(dek, userId, purpose, value) {
  return encryptJSON(dek, [purpose, `user:${userId}`], value);
}

export function decryptValue(dek, userId, purpose, blob) {
  return decryptJSON(dek, [purpose, `user:${userId}`], blob);
}

// ---- Konversi untuk API (base64 standar, sama dengan []byte di Go) ----

export function toBase64(bytes) {
  let s = '';
  for (const b of bytes) s += String.fromCharCode(b);
  return btoa(s);
}

export function fromBase64(str) {
  const s = atob(str);
  const out = new Uint8Array(s.length);
  for (let i = 0; i < s.length; i++) out[i] = s.charCodeAt(i);
  return out;
}

export function wrapToJSON(w) {
  return {
    kdf: w.kdf, iterations: w.iterations,
    salt: toBase64(w.salt), wrappedKey: toBase64(w.wrappedKey), wrapIv: toBase64(w.wrapIv),
  };
}

export function materialToJSON(m) {
  return {
    version: m.version, ...wrapToJSON(m),
    verifier: toBase64(m.verifier), verifierIv: toBase64(m.verifierIv),
  };
}

export function materialFromJSON(j) {
  return {
    version: j.version, kdf: j.kdf, iterations: j.iterations,
    salt: fromBase64(j.salt), wrappedKey: fromBase64(j.wrappedKey), wrapIv: fromBase64(j.wrapIv),
    verifier: fromBase64(j.verifier), verifierIv: fromBase64(j.verifierIv),
  };
}
