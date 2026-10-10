// Panel "Simpan kunci pemulihanmu". Kunci hanya ada di memori halaman ini dan
// di DOM selama panel tampil; tidak pernah dikirim ke server atau disimpan
// di penyimpanan browser.

import { recoveryFileText } from './export-logic.js';
import { longDate } from './history-logic.js';
import { isoDate } from './entry-logic.js';
import { toast } from './http.js';

function download(name, text, type = 'text/plain;charset=utf-8') {
  const url = URL.createObjectURL(new Blob([text], { type }));
  const a = document.createElement('a');
  a.href = url;
  a.download = name;
  document.body.appendChild(a);
  a.click();
  a.remove();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
}

export { download };

/**
 * Menampilkan panel kunci pemulihan.
 * @param {HTMLElement} panel  elemen [data-role="recovery-panel"]
 * @param {string} code
 * @param {() => Promise<void>} onDone  dipanggil setelah pengguna menyatakan sudah menyimpan
 */
export function showRecoveryPanel(panel, code, onDone) {
  const $ = (s) => panel.querySelector(s);
  // Dua bagian (3 + 3 kelompok) agar terpotong rapi di layar sempit, tidak di tengah kelompok.
  const box = $('[data-role="recovery-code"]');
  const groups = code.split('-');
  box.replaceChildren(...[groups.slice(0, 3), groups.slice(3)].map((g, i) => {
    const part = document.createElement('span');
    part.textContent = (i ? '-' : '') + g.join('-');
    return part;
  }));
  const saved = $('[data-field="recovery-saved"]');
  const done = $('[data-action="recovery-done"]');
  saved.checked = false;
  done.disabled = true;
  panel.hidden = false;
  panel.scrollIntoView({ block: 'start' });

  const created = longDate(isoDate(new Date()));
  const handlers = {
    'recovery-copy': async () => {
      try {
        await navigator.clipboard.writeText(code);
        toast('Kunci pemulihan disalin. Tempel di tempat yang aman.');
      } catch {
        toast('Tidak bisa menyalin otomatis. Pilih teks kunci lalu salin manual.');
      }
    },
    'recovery-download': () => download('jurnal-mood-kunci-pemulihan.txt', recoveryFileText(code, created)),
    'recovery-print': () => window.print(),
  };
  panel.onclick = async (e) => {
    const b = e.target.closest('[data-action]');
    if (b && handlers[b.dataset.action]) await handlers[b.dataset.action]();
  };
  saved.onchange = () => { done.disabled = !saved.checked; };
  done.onclick = async () => {
    done.disabled = true;
    $('[data-role="recovery-code"]').textContent = '';
    await onDone();
  };
}
