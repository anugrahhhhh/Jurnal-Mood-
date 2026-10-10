// Kalender mood satu bulan. Warna menunjukkan skor mood; identitas tidak
// bergantung pada warna saja: setiap tanggal punya label untuk pembaca layar
// dan tooltip, dan detailnya tampil saat dipilih.

import { MOODS } from './entry-logic.js';
import { longDate, monthGrid } from './history-logic.js';

const DOW = ['Sen', 'Sel', 'Rab', 'Kam', 'Jum', 'Sab', 'Min'];

/**
 * @param {HTMLElement} box   wadah .cal
 * @param {number} year
 * @param {number} month      0–11
 * @param {Map<string, {mood:number}>} byDate
 * @param {{today:string, selected?:string, mini?:boolean, onSelect?:(iso:string)=>void}} opts
 */
export function renderMonth(box, year, month, byDate, opts) {
  const { cells } = monthGrid(year, month);
  box.replaceChildren();
  if (!opts.mini) {
    for (const d of DOW) {
      const h = document.createElement('div');
      h.className = 'dow';
      h.textContent = d;
      h.setAttribute('aria-hidden', 'true');
      box.appendChild(h);
    }
  }
  for (const iso of cells) {
    if (!iso) {
      const blank = document.createElement('span');
      blank.className = 'cell blank';
      blank.setAttribute('aria-hidden', 'true');
      box.appendChild(blank);
      continue;
    }
    const entry = byDate.get(iso);
    const future = iso > opts.today;
    const cell = document.createElement(opts.mini ? 'span' : 'button');
    cell.className = 'cell';
    if (!opts.mini) cell.type = 'button';
    cell.dataset.date = iso;
    if (entry) cell.dataset.mood = String(entry.mood);
    if (iso === opts.today) cell.classList.add('today');
    if (future) cell.classList.add('future');
    const label = `${longDate(iso)}: ${entry ? MOODS[entry.mood].label : future ? 'belum tiba' : 'belum diisi'}`;
    cell.title = label;
    if (opts.mini) {
      cell.setAttribute('role', 'img');
      cell.setAttribute('aria-label', label);
    } else {
      cell.setAttribute('aria-label', label);
      cell.setAttribute('aria-pressed', String(iso === opts.selected));
      cell.textContent = String(Number(iso.slice(8)));
      if (future) cell.disabled = true;
      else if (opts.onSelect) cell.addEventListener('click', () => opts.onSelect(iso));
    }
    box.appendChild(cell);
  }
}

