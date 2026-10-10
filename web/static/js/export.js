// Unduh semua catatan (JSON atau CSV). Didekripsi di perangkat ini; file
// hasil ekspor tidak terenkripsi dan tidak pernah dikirim ke server.

import { isoDate } from './entry-logic.js';
import { dateChunks, exportFileName, toCSV, toExportJSON } from './export-logic.js';
import { loadDecrypted } from './history.js';
import { api, toast } from './http.js';
import { download } from './recovery-ui.js';

export async function exportAll(c, dek, format, statusEl) {
  const say = (t) => { if (statusEl) { statusEl.textContent = t; statusEl.hidden = !t; } };
  say('Menyiapkan file…');
  const today = isoDate(new Date());
  const { first } = await api('GET', '/api/entri/rentang');
  if (!first) {
    say('');
    toast('Belum ada catatan untuk diunduh.');
    return;
  }
  const entries = [];
  for (const [from, to] of dateChunks(first, today)) {
    entries.push(...await loadDecrypted(c, dek, from, to));
  }
  const unreadable = entries.filter((e) => !e.payload).length;
  if (format === 'csv') {
    download(exportFileName('csv', today), toCSV(entries), 'text/csv;charset=utf-8');
  } else {
    download(exportFileName('json', today), `${JSON.stringify(toExportJSON(entries, new Date().toISOString()), null, 2)}\n`,
      'application/json;charset=utf-8');
  }
  say(`${entries.length} catatan diunduh${unreadable ? ` (${unreadable} tidak bisa dibuka)` : ''}. Simpan file itu di tempat aman.`);
}
