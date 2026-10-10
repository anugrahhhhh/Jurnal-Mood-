// Perjalanan pengguna lengkap di setiap ukuran layar (lihat projects di
// playwright.config.mjs). Selain alurnya, setiap halaman diperiksa:
//   - tidak ada error JavaScript atau pelanggaran CSP di console,
//   - tidak ada scroll horizontal (tata letak muat di layar),
//   - navigasi yang benar tampil: tab bar di layar < 900px, sidebar di >= 900px.
// Screenshot setiap langkah disimpan di test-results/shots/ untuk ditinjau.
import fs from 'node:fs';
import path from 'node:path';

import { expect, test } from '@playwright/test';

const SHOTS = path.resolve('test-results', 'shots');

const PASSWORD = 'kuda-laut-biru-42';
const PASSPHRASE = 'kopi jendela hujan sepeda';
const NOTE = 'Catatan rahasia uji e2e: hari yang tenang.';

function watchErrors(page) {
  const errors = [];
  page.on('console', (msg) => { if (msg.type() === 'error') errors.push(`console: ${msg.text()}`); });
  page.on('pageerror', (err) => errors.push(`pageerror: ${err.message}`));
  // Respons gagal dicatat dengan URL-nya. 401/403/422 dari API adalah bagian alur
  // yang diuji (mis. passphrase salah tidak memanggil server), jadi hanya >= 404 & bukan 422.
  page.on('response', (res) => {
    if (res.status() >= 404 && res.status() !== 422) errors.push(`http ${res.status()}: ${new URL(res.url()).pathname}`);
  });
  return errors;
}

async function checkLayout(page, name, testInfo, { signedIn }) {
  await page.waitForLoadState('networkidle');
  const { scrollWidth, innerWidth } = await page.evaluate(() => ({
    scrollWidth: document.documentElement.scrollWidth, innerWidth: window.innerWidth,
  }));
  expect(scrollWidth, `${name}: halaman tidak boleh melebar ke samping`).toBeLessThanOrEqual(innerWidth + 1);
  await expect(page.locator('a[href="/bantuan"]:visible').first(), `${name}: tautan bantuan krisis harus terlihat`).toBeVisible();
  if (signedIn) {
    const wide = innerWidth >= 900;
    await expect(page.locator('.sidebar'), `${name}: sidebar`).toBeVisible({ visible: wide });
    await expect(page.locator('.tabbar'), `${name}: tab bar`).toBeVisible({ visible: !wide });
  }
  await page.screenshot({ path: path.join(SHOTS, `${testInfo.project.name}-${name}.png`), fullPage: true });
}

// Klik menu navigasi yang sedang tampil (sidebar atau tab bar).
async function nav(page, href) {
  await page.locator(`.sidebar a[href="${href}"]:visible, .tabbar a[href="${href}"]:visible`).first().click();
  await expect(page).toHaveURL(new RegExp(`${href.replace('/', '\\/')}(\\?.*)?$`));
}

test('perjalanan lengkap: daftar → passphrase → catat → beranda → riwayat → kunci', async ({ page }, testInfo) => {
  const errors = watchErrors(page);
  const email = `e2e-${testInfo.project.name}-${Date.now()}@contoh.id`;

  // Landing page
  await page.goto('/');
  await expect(page.getByRole('heading', { level: 1 })).toContainText('Catat harimu');
  await checkLayout(page, '01-landing', testInfo, { signedIn: false });
  await page.locator('#faq details').first().locator('summary').click();
  await expect(page.locator('#faq details').first()).toHaveAttribute('open', '');

  // Daftar (terbuka, tanpa kode undangan)
  await page.getByRole('link', { name: 'Daftar gratis' }).first().click();
  await expect(page).toHaveURL(/\/daftar$/);
  await expect(page.locator('input[name="invite"]')).toHaveCount(0);
  await checkLayout(page, '02-daftar', testInfo, { signedIn: false });
  await page.getByLabel('Email').fill(email);
  await page.getByLabel('Password login').fill(PASSWORD);
  await page.getByLabel('Ulangi password').fill(PASSWORD);
  await page.locator('input[name="setuju"]').check();
  await page.getByRole('button', { name: 'Buat akun' }).click();

  // Langkah 2: passphrase jurnal
  await expect(page).toHaveURL(/\/jurnal\/buat$/);
  await expect(page.locator('.steps-bar [aria-current="step"]')).toContainText('Passphrase');
  await checkLayout(page, '03-passphrase', testInfo, { signedIn: true });
  await page.locator('[data-field="passphrase"]').fill(PASSPHRASE);
  await expect(page.locator('.strength')).toHaveAttribute('data-level', '2');
  // Tombol tampilkan/sembunyikan mengubah jenis kolom.
  const toggle = page.locator('[data-field="passphrase"] + .pw-toggle');
  await toggle.click();
  await expect(page.locator('[data-field="passphrase"]')).toHaveAttribute('type', 'text');
  await toggle.click();
  await expect(page.locator('[data-field="passphrase"]')).toHaveAttribute('type', 'password');
  await page.locator('[data-field="passphrase-confirm"]').fill(PASSPHRASE);
  await page.locator('[data-field="understand"]').check();
  await page.getByRole('button', { name: /Buat .* buka jurnal/ }).click();

  // Kunci pemulihan ditampilkan sekali; tombol lanjut baru aktif setelah dicentang.
  const panel = page.locator('[data-role="recovery-panel"]');
  await expect(panel).toBeVisible({ timeout: 30_000 });
  const recoveryCode = (await panel.locator('[data-role="recovery-code"]').textContent()).trim();
  expect(recoveryCode).toMatch(/^[0-9A-Z]{4}(-[0-9A-Z]{4}){5}$/);
  await checkLayout(page, '03b-kunci-pemulihan', testInfo, { signedIn: true });
  await expect(panel.locator('[data-action="recovery-done"]')).toBeDisabled();
  await panel.locator('[data-field="recovery-saved"]').check();
  await panel.locator('[data-action="recovery-done"]').click();

  // Langkah 3: catatan pertama
  await expect(page).toHaveURL(/\/hari-ini\?mulai=1$/, { timeout: 30_000 });
  await expect(page.locator('[data-role="onboarding"]')).toBeVisible();
  await expect(page.locator('html')).toHaveAttribute('data-journal', 'unlocked');
  await page.locator('[data-role="mood-tick"][data-mood="4"]').click();
  await expect(page.locator('[data-role="mood-label"]')).toHaveText('Baik');
  await expect(page.locator('html')).toHaveAttribute('data-mood', '4');
  await page.locator('[data-action="sleep-plus"]').click();
  await page.locator('[data-role="chips"] .chip', { hasText: 'Olahraga' }).click();
  await page.locator('[data-role="note-box"] summary').click();
  await page.locator('textarea[data-field="note"]').fill(NOTE);
  await checkLayout(page, '04-hari-ini', testInfo, { signedIn: true });
  await page.locator('[data-action="save"]').click();
  await expect(page.locator('[data-role="status"]')).toHaveText('Tersimpan ✓');
  await expect(page.locator('[data-role="saved-note"]')).toBeVisible();
  await expect(page.locator('[data-action="save"]')).toHaveText('Simpan perubahan');
  await expect(page.locator('[data-role="saved-summary"]')).toBeVisible();

  // Beranda (dashboard)
  await nav(page, '/beranda');
  await expect(page.locator('[data-role="today-done"]')).toBeVisible();
  await expect(page.locator('[data-role="today-label"]')).toContainText('Baik');
  await expect(page.locator('[data-role="avg-mood"]')).toContainText('4');
  await expect(page.locator('[data-role="top-activities"]')).toBeVisible();
  await expect(page.locator('[data-role="week-chart"] svg')).toBeVisible();
  await checkLayout(page, '05-beranda', testInfo, { signedIn: true });

  // Riwayat: kalender + detail hari (catatan didekripsi di perangkat)
  await nav(page, '/riwayat');
  const today = await page.evaluate(() => {
    const d = new Date();
    const p = (n) => String(n).padStart(2, '0');
    return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}`;
  });
  const cell = page.locator(`[data-role="calendar"] .cell[data-date="${today}"]`);
  await expect(cell).toHaveAttribute('data-mood', '4');
  await cell.click();
  await expect(page.locator('[data-role="day-detail"]')).toContainText(NOTE);
  await expect(page.locator('[data-role="mood-chart"] svg')).toBeVisible();
  await page.getByRole('button', { name: 'Tahun' }).click();
  await expect(page.getByRole('button', { name: 'Tahun' })).toHaveAttribute('aria-pressed', 'true');
  await checkLayout(page, '06-riwayat', testInfo, { signedIn: true });

  // Kunci jurnal → buka lagi (passphrase salah dulu)
  await page.locator('[data-action="lock"]:visible').first().click();
  await expect(page).toHaveURL(/\/jurnal\/buka\?/);
  await expect(page.locator('html')).toHaveAttribute('data-journal', 'locked');
  await page.locator('[data-field="passphrase"]').fill('passphrase yang salah sekali');
  await page.locator('main form button[type="submit"]').click();
  await expect(page.locator('[data-role="error"]')).toHaveText('Passphrase salah.', { timeout: 30_000 });
  await page.locator('[data-field="passphrase"]').fill(PASSPHRASE);
  await page.locator('main form button[type="submit"]').click();
  await expect(page).toHaveURL(/\/riwayat$/, { timeout: 30_000 });
  await expect(page.locator(`[data-role="calendar"] .cell[data-date="${today}"]`)).toHaveAttribute('data-mood', '4');

  // Lupa passphrase: pulihkan dengan kunci pemulihan (diketik huruf kecil, tanpa strip).
  const NEW_PASSPHRASE = 'gunung pasir laut bintang';
  await page.locator('[data-action="lock"]:visible').first().click();
  await expect(page).toHaveURL(/\/jurnal\/buka\?/);
  await page.getByRole('link', { name: 'Pakai kunci pemulihan' }).click();
  await expect(page).toHaveURL(/\/jurnal\/pulihkan$/);
  await page.locator('[data-field="recovery-code"]').fill(recoveryCode.toLowerCase().replace(/-/g, ' '));
  await page.locator('[data-field="new-passphrase"]').fill(NEW_PASSPHRASE);
  await page.locator('[data-field="new-passphrase-confirm"]').fill(NEW_PASSPHRASE);
  await page.locator('[data-field="login-password"]').fill(PASSWORD);
  await checkLayout(page, '06b-pulihkan', testInfo, { signedIn: true });
  await page.locator('main form button[type="submit"]').click();
  await expect(page).toHaveURL(/\/hari-ini$/, { timeout: 30_000 });
  await expect(page.locator('html')).toHaveAttribute('data-journal', 'unlocked');
  await expect(page.locator('[data-role="saved-note"]')).toBeVisible(); // entri lama terbaca dengan kunci yang sama

  // Pengaturan (di HP satu-satunya tempat tombol Keluar)
  await nav(page, '/pengaturan');
  await expect(page.locator('form[action="/akun/hapus"]'), 'form hapus akun tertutup secara bawaan').toBeHidden();
  // Ekspor: file dibuat di perangkat ini dan berisi catatan yang sudah dibuka.
  const [dl] = await Promise.all([page.waitForEvent('download'), page.locator('[data-action="export-json"]').click()]);
  expect(dl.suggestedFilename()).toMatch(/^jurnal-mood-\d{4}-\d{2}-\d{2}\.json$/);
  const exported = JSON.parse(fs.readFileSync(await dl.path(), 'utf8'));
  expect(exported.entries.map((r) => r.note)).toContain(NOTE);
  const [dlCsv] = await Promise.all([page.waitForEvent('download'), page.locator('[data-action="export-csv"]').click()]);
  expect(fs.readFileSync(await dlCsv.path(), 'utf8')).toContain('Catatan rahasia uji e2e');
  await expect(page.locator('[data-role="export-status"]')).toContainText('catatan diunduh');

  await page.locator('main a[href="/akun/password"]').click();
  await expect(page.getByRole('heading', { level: 1 })).toHaveText('Ganti password login');
  await checkLayout(page, '07b-ganti-password', testInfo, { signedIn: true });
  await nav(page, '/pengaturan');
  await checkLayout(page, '07-pengaturan', testInfo, { signedIn: true });

  // Mode gelap
  await page.emulateMedia({ colorScheme: 'dark' });
  await nav(page, '/beranda');
  const bg = await page.evaluate(() => getComputedStyle(document.body).backgroundColor);
  const [r, g, b] = bg.match(/\d+/g).map(Number);
  expect(0.2126 * r + 0.7152 * g + 0.0722 * b, `latar mode gelap harus gelap (${bg})`).toBeLessThan(80);
  await checkLayout(page, '08-beranda-gelap', testInfo, { signedIn: true });
  await page.emulateMedia({ colorScheme: 'light' });

  // Keluar
  await nav(page, '/pengaturan');
  await page.locator('main form[action="/keluar"] button').click();
  await expect(page).toHaveURL(/\/masuk$/);

  expect(errors, 'tidak boleh ada error JavaScript atau pelanggaran CSP').toEqual([]);
});

test('halaman publik tampil rapi tanpa error', async ({ page }, testInfo) => {
  const errors = watchErrors(page);
  for (const [path, name] of [['/masuk', 'masuk'], ['/privasi', 'privasi'], ['/bantuan', 'bantuan']]) {
    await page.goto(path);
    await checkLayout(page, `publik-${name}`, testInfo, { signedIn: false });
    if (path === '/masuk') {
      await page.locator('.forgot summary').click();
      await expect(page.locator('.forgot p')).toContainText('password sementara');
    }
  }
  await page.emulateMedia({ colorScheme: 'dark' });
  await page.goto('/');
  await checkLayout(page, 'publik-landing-gelap', testInfo, { signedIn: false });
  expect(errors).toEqual([]);
});
