// Jalankan: node --test tests/js
import assert from 'node:assert/strict';
import { test } from 'node:test';

import {
  changePassphrase, createJournalKey, decryptEntry, decryptValue, encryptEntry, encryptValue,
  ITERATIONS, materialFromJSON, materialToJSON, PassphraseTooShortError, unlockJournalKey,
  WrongPassphraseError, wrapToJSON,
} from '../../web/static/js/crypto.js';

// Iterasi kecil agar tes cepat; nilai produksi diuji terpisah di bawah.
const FAST = { iterations: 1_000 };
const PASS = 'kopi jendela hujan sepeda';
const USER = '42';

test('membuat lalu membuka kunci dengan passphrase yang benar', async () => {
  const { material, dek } = await createJournalKey(PASS, USER, FAST);
  const unlocked = await unlockJournalKey(PASS, USER, material);

  const blob = await encryptEntry(dek, USER, '2026-10-10', { note: 'halo' });
  assert.deepEqual(await decryptEntry(unlocked, USER, '2026-10-10', blob), { note: 'halo' });
});

test('DEK di memori tidak bisa diekspor', async () => {
  const { material, dek } = await createJournalKey(PASS, USER, FAST);
  const unlocked = await unlockJournalKey(PASS, USER, material);
  for (const key of [dek, unlocked]) {
    assert.equal(key.extractable, false);
    await assert.rejects(crypto.subtle.exportKey('raw', key));
  }
});

test('passphrase salah ditolak', async () => {
  const { material } = await createJournalKey(PASS, USER, FAST);
  await assert.rejects(unlockJournalKey('kopi jendela hujan sepedA', USER, material), WrongPassphraseError);
});

test('material kunci terikat ke pengguna (AAD)', async () => {
  const { material } = await createJournalKey(PASS, USER, FAST);
  await assert.rejects(unlockJournalKey(PASS, '43', material), WrongPassphraseError);
});

test('passphrase terlalu pendek ditolak', async () => {
  await assert.rejects(createJournalKey('pendek', USER, FAST), PassphraseTooShortError);
  const { material } = await createJournalKey(PASS, USER, FAST);
  await assert.rejects(changePassphrase(PASS, 'pendek', USER, material, FAST), PassphraseTooShortError);
});

test('passphrase dinormalisasi NFC', async () => {
  const composed = 'kafe café di sudut';            // é = U+00E9
  const decomposed = 'kafe café di sudut';    // e + U+0301
  const { material } = await createJournalKey(composed, USER, FAST);
  await unlockJournalKey(decomposed, USER, material); // tidak melempar
});

test('IV entri selalu baru, juga untuk isi yang sama', async () => {
  const { dek } = await createJournalKey(PASS, USER, FAST);
  const seen = new Set();
  for (let i = 0; i < 50; i++) {
    const { iv } = await encryptEntry(dek, USER, '2026-10-10', { note: 'sama' });
    assert.equal(iv.length, 12);
    seen.add(Buffer.from(iv).toString('hex'));
  }
  assert.equal(seen.size, 50);
});

test('entri terikat ke tanggal dan pengguna (AAD)', async () => {
  const { dek } = await createJournalKey(PASS, USER, FAST);
  const blob = await encryptEntry(dek, USER, '2026-10-10', { note: 'rahasia' });
  await assert.rejects(decryptEntry(dek, USER, '2026-10-11', blob));
  await assert.rejects(decryptEntry(dek, '43', '2026-10-10', blob));
});

test('ciphertext yang diubah ditolak', async () => {
  const { dek } = await createJournalKey(PASS, USER, FAST);
  const blob = await encryptEntry(dek, USER, '2026-10-10', { note: 'rahasia' });
  blob.ciphertext[0] ^= 1;
  await assert.rejects(decryptEntry(dek, USER, '2026-10-10', blob));
});

test('ciphertext tidak memuat teks asli', async () => {
  const { dek } = await createJournalKey(PASS, USER, FAST);
  const { ciphertext } = await encryptEntry(dek, USER, '2026-10-10', { note: 'rahasia-sekali' });
  assert.ok(!Buffer.from(ciphertext).toString('latin1').includes('rahasia'));
});

test('ganti passphrase: entri lama tetap terbaca, passphrase lama tidak berlaku', async () => {
  const { material, dek } = await createJournalKey(PASS, USER, FAST);
  const oldEntry = await encryptEntry(dek, USER, '2026-10-01', { note: 'entri lama' });

  const NEW = 'gunung pasir laut bintang';
  const { wrap, dek: newDek } = await changePassphrase(PASS, NEW, USER, material, FAST);
  assert.equal(newDek.extractable, false);

  const updated = { ...material, ...wrap }; // nilai uji (verifier) tidak berubah
  assert.notDeepEqual(updated.salt, material.salt, 'salt harus baru');

  const reopened = await unlockJournalKey(NEW, USER, updated);
  assert.deepEqual(await decryptEntry(reopened, USER, '2026-10-01', oldEntry), { note: 'entri lama' });
  await assert.rejects(unlockJournalKey(PASS, USER, updated), WrongPassphraseError);
});

test('ganti passphrase menolak passphrase lama yang salah', async () => {
  const { material } = await createJournalKey(PASS, USER, FAST);
  await assert.rejects(changePassphrase('bukan passphrase ini', 'gunung pasir laut bintang', USER, material, FAST),
    WrongPassphraseError);
});

test('encryptValue terikat ke tujuannya', async () => {
  const { dek } = await createJournalKey(PASS, USER, FAST);
  const blob = await encryptValue(dek, USER, 'draft', { mood: 4 });
  assert.deepEqual(await decryptValue(dek, USER, 'draft', blob), { mood: 4 });
  await assert.rejects(decryptValue(dek, USER, 'activities', blob));
});

test('format JSON cocok dengan validasi server', async () => {
  const { material } = await createJournalKey(PASS, USER, FAST);
  const json = materialToJSON(material);
  assert.deepEqual(Object.keys(json).sort(),
    ['iterations', 'kdf', 'salt', 'verifier', 'verifierIv', 'version', 'wrapIv', 'wrappedKey']);
  assert.equal(json.version, 1);
  assert.equal(json.kdf, 'PBKDF2-SHA256');

  const back = materialFromJSON(json);
  assert.equal(back.salt.length, 16);
  assert.equal(back.wrapIv.length, 12);
  assert.equal(back.wrappedKey.length, 48);
  assert.equal(back.verifierIv.length, 12);
  await unlockJournalKey(PASS, USER, back);

  assert.deepEqual(Object.keys(wrapToJSON(material)).sort(), ['iterations', 'kdf', 'salt', 'wrapIv', 'wrappedKey']);
});

test('nilai iterasi produksi 600.000 dan dipakai secara default', async () => {
  assert.equal(ITERATIONS, 600_000);
  const { material } = await createJournalKey(PASS, USER);
  assert.equal(material.iterations, 600_000);
});
