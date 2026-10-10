// Tes end-to-end di Chrome yang terpasang (channel: 'chrome'), sehingga tidak
// perlu mengunduh browser. Server e2e berjalan di WSL pada port 8090 dengan
// database moodjournal_e2e, terpisah dari data pengembangan.
//
// Jalankan dari folder proyek:  npm run e2e
import { defineConfig, devices } from '@playwright/test';

const chrome = { browserName: 'chromium', channel: 'chrome' };

export default defineConfig({
  testDir: '.',
  timeout: 120_000,
  expect: { timeout: 15_000 },
  workers: 1,
  retries: 0,
  reporter: [['list']],
  outputDir: '../../test-results',
  use: {
    baseURL: 'http://127.0.0.1:8090',
    headless: true,
    locale: 'id-ID',
    timezoneId: 'Asia/Jakarta',
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
  },
  webServer: {
    command: 'wsl -d Ubuntu -- bash scripts/dev.sh e2e-server',
    cwd: '../..',
    url: 'http://127.0.0.1:8090/healthz',
    reuseExistingServer: true,
    timeout: 240_000,
  },
  projects: [
    { name: 'hp', use: { ...devices['Pixel 7'], ...chrome } },
    {
      name: 'ipad-potret',
      use: { ...chrome, viewport: { width: 820, height: 1180 }, deviceScaleFactor: 2, isMobile: true, hasTouch: true,
        userAgent: devices['iPad (gen 7)'].userAgent },
    },
    {
      name: 'ipad-lanskap',
      use: { ...chrome, viewport: { width: 1180, height: 820 }, deviceScaleFactor: 2, isMobile: true, hasTouch: true,
        userAgent: devices['iPad (gen 7)'].userAgent },
    },
    { name: 'laptop', use: { ...chrome, viewport: { width: 1366, height: 800 } } },
  ],
});
