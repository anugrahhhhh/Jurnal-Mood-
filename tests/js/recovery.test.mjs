import assert from 'node:assert/strict';
import { test } from 'node:test';

import {
  addRecoveryKey, changePassphrase, createJournalKey, decryptEntry, encryptEntry, generateRecoveryCode,
  materialFromJSON, materialToJSON, normalizeRecoveryCode, recoverWithCode, unlockJournalKey,
  WrongPassphraseError, WrongRecoveryCodeError,
} from '../../web/static/js/crypto.js';

const FAST = { iterations: 1_000, recoveryIterations: 1_000 };
const PASS = 'kopi jendela hujan sepeda';
const NEW = 'gunung pasir laut bintang';
const USER = '7';

test('kode pemulihan: 24 karakter Crockford dalam 6 kelompok, acak', () => {
  const seen = new Set();
  for (let i = 0; i < 200; i++) {
    const c = generateRecoveryCode();
    assert.match(c, /^[0-9A-HJKMNP-TV-Z]{4}(-[0-9A-HJKMNP-TV-Z]{4}){5}$/);
    seen.add(c);
  }
  assert.equal(seen.size, 200);
});

test('normalisasi: huruf kecil, spasi, strip, O/I/L', () => {
  const c = generateRecoveryCode();
  const messy = ` ${c.toLowerCase().replace(/-/g, ' ')} `;
  assert.equal(normalizeRecoveryCode(messy), c.replace(/-/g, ''));
  assert.equal(normalizeRecoveryCode('OIL0-0000-0000-0000-0000-0000'), '011000000000000000000000');
  assert.throws(() => normalizeRecoveryCode('terlalu-pendek'), WrongRecoveryCodeError);
  assert.throws(() => normalizeRecoveryCode('UUUU-0000-0000-0000-0000-0000'), WrongRecoveryCodeError, 'U bukan huruf Crockford');
});

test('jurnal baru selalu punya kunci pemulihan yang bisa memulihkan passphrase', async () => {
  const { material, dek, recoveryCode } = await createJournalKey(PASS, USER, FAST);
  assert.ok(material.recovery, 'material berisi bungkus pemulihan');
  const old = await encryptEntry(dek, USER, '2026-10-01', { note: 'entri lama' });

  const { wrap, dek: newDek } = await recoverWithCode(recoveryCode.toLowerCase(), NEW, USER, material, FAST);
  assert.equal(newDek.extractable, false);
  const updated = { ...material, ...wrap };
  const reopened = await unlockJournalKey(NEW, USER, updated);
  assert.deepEqual(await decryptEntry(reopened, USER, '2026-10-01', old), { note: 'entri lama' });
  await assert.rejects(unlockJournalKey(PASS, USER, updated), WrongPassphraseError, 'passphrase lama tidak berlaku');
  // Kunci pemulihan tetap berlaku setelah dipakai (DEK sama).
  await recoverWithCode(recoveryCode, 'samudra hijau senja pagi', USER, updated, FAST);
});

test('kunci pemulihan salah, milik pengguna lain, atau tidak ada', async () => {
  const { material, recoveryCode } = await createJournalKey(PASS, USER, FAST);
  await assert.rejects(recoverWithCode(generateRecoveryCode(), NEW, USER, material, FAST), WrongRecoveryCodeError);
  await assert.rejects(recoverWithCode(recoveryCode, NEW, '8', material, FAST), WrongRecoveryCodeError, 'terikat ke pengguna (AAD)');
  await assert.rejects(recoverWithCode(recoveryCode, NEW, USER, { ...material, recovery: null }, FAST), WrongRecoveryCodeError);
  await assert.rejects(recoverWithCode(recoveryCode, 'pendek', USER, material, FAST), /minimal 12/);
});

test('ganti passphrase tidak merusak kunci pemulihan', async () => {
  const { material, recoveryCode } = await createJournalKey(PASS, USER, FAST);
  const { wrap } = await changePassphrase(PASS, NEW, USER, material, FAST);
  const updated = { ...material, ...wrap };
  await recoverWithCode(recoveryCode, 'samudra hijau senja pagi', USER, updated, FAST);
});

test('membuat kunci pemulihan baru menggantikan yang lama', async () => {
  const { material, recoveryCode: oldCode } = await createJournalKey(PASS, USER, FAST);
  const { code, wrap } = await addRecoveryKey(PASS, USER, material, { iterations: 1_000 });
  assert.notEqual(code, oldCode);
  const updated = { ...material, recovery: wrap };
  await recoverWithCode(code, NEW, USER, updated, FAST);
  await assert.rejects(recoverWithCode(oldCode, NEW, USER, updated, FAST), WrongRecoveryCodeError);
  await assert.rejects(addRecoveryKey('passphrase yang salah', USER, material), WrongPassphraseError);
});

test('format JSON pemulihan bolak-balik dan cocok dengan validasi server', async () => {
  const { material, recoveryCode } = await createJournalKey(PASS, USER, FAST);
  const j = materialToJSON(material);
  assert.deepEqual(Object.keys(j.recovery).sort(), ['iterations', 'salt', 'wrapIv', 'wrappedKey']);
  const back = materialFromJSON(j);
  assert.equal(back.recovery.salt.length, 16);
  assert.equal(back.recovery.wrapIv.length, 12);
  assert.equal(back.recovery.wrappedKey.length, 48);
  await recoverWithCode(recoveryCode, NEW, USER, back, FAST);
  assert.equal(materialFromJSON({ ...j, recovery: null }).recovery, null);
});
