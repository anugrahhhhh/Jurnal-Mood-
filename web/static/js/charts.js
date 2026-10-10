// Grafik SVG tanpa library, dipakai riwayat & dashboard. Mood (1–5) dan jam
// tidur sengaja digambar di dua grafik terpisah: skalanya berbeda, dan satu
// grafik dengan dua sumbu menyesatkan. Teks selalu lewat textContent.

import { formatSleep, MOODS } from './entry-logic.js';
import { MONTH_NAMES } from './history-logic.js';

const SVG = 'http://www.w3.org/2000/svg';
const PAD = { top: 12, right: 10, bottom: 26, left: 34 };
let W = 340;
let H = 170;

// Ukuran SVG mengikuti lebar wadah (1 unit = 1 px), sehingga teks sumbu tetap
// 11px di HP maupun laptop. Grafik digambar ulang saat lebarnya berubah.
function sizeFor(wrap) {
  W = Math.round(Math.min(1100, Math.max(260, wrap.clientWidth || 340)));
  H = W < 420 ? 170 : 200;
}

const observed = new WeakMap();
const resizer = typeof ResizeObserver === 'function' ? new ResizeObserver((items) => {
  for (const it of items) {
    const st = observed.get(it.target);
    if (st && Math.abs(it.contentRect.width - st.width) > 8) st.redraw();
  }
}) : null;

function remember(wrap, redraw) {
  if (!resizer) return;
  observed.set(wrap, { width: wrap.clientWidth, redraw });
  resizer.observe(wrap);
}

export const shortDate = (iso) => {
  const [, m, d] = iso.split('-').map(Number);
  return `${d} ${MONTH_NAMES[m - 1].slice(0, 3)}`;
};

function el(name, attrs = {}, parent) {
  const node = document.createElementNS(SVG, name);
  for (const [k, v] of Object.entries(attrs)) node.setAttribute(k, String(v));
  if (parent) parent.appendChild(node);
  return node;
}

function text(parent, x, y, value, cls, anchor = 'middle') {
  const t = el('text', { x, y, class: cls, 'text-anchor': anchor }, parent);
  t.textContent = value;
  return t;
}

function frame(series) {
  const band = (W - PAD.left - PAD.right) / series.length;
  return { band, x: (i) => PAD.left + band * i + band / 2 };
}

function xLabels(svg, series, x, dayLabels) {
  const idx = series.length <= 7 ? series.map((_, i) => i) : [0, Math.floor((series.length - 1) / 2), series.length - 1];
  for (const i of idx) {
    const label = series.length <= 7 && dayLabels ? dayLabels(series[i].date) : shortDate(series[i].date);
    text(svg, x(i), H - 8, label, 'axis');
  }
}

// Area sentuh per hari, lebih besar dari tanda grafiknya.
function hitAreas(svg, series, band, x, onHover) {
  series.forEach((p, i) => {
    const r = el('rect', { x: x(i) - band / 2, y: PAD.top, width: band, height: H - PAD.top - PAD.bottom, class: 'hit' }, svg);
    r.addEventListener('pointerenter', () => onHover(p, i));
    r.addEventListener('pointermove', () => onHover(p, i));
  });
}

function makeTooltip(wrap) {
  const tip = document.createElement('div');
  tip.className = 'chart-tip';
  tip.hidden = true;
  wrap.appendChild(tip);
  wrap.addEventListener('pointerleave', () => { tip.hidden = true; });
  return (lines, xFrac) => {
    tip.replaceChildren(...lines.map((l, i) => {
      const s = document.createElement(i === 0 ? 'strong' : 'span');
      s.textContent = l;
      return s;
    }));
    tip.hidden = false;
    tip.style.left = `${Math.min(82, Math.max(18, xFrac * 100))}%`;
  };
}

/** Garis mood harian. Garis terputus di hari yang tidak diisi. */
export function drawMood(wrap, series, opts = {}) {
  const { dayLabels } = opts;
  remember(wrap, () => drawMood(wrap, series, opts));
  sizeFor(wrap);
  const width = W; // dikunci: tooltip dihitung nanti, saat W mungkin sudah berubah
  wrap.replaceChildren();
  const svg = el('svg', { viewBox: `0 0 ${W} ${H}`, role: 'img', 'aria-label': 'Grafik mood harian, skala 1 sampai 5' });
  wrap.appendChild(svg);
  const { band, x } = frame(series);
  const y = (m) => PAD.top + ((5 - m) / 4) * (H - PAD.top - PAD.bottom);

  for (let m = 1; m <= 5; m++) {
    el('line', { x1: PAD.left, x2: W - PAD.right, y1: y(m), y2: y(m), class: 'grid' }, svg);
    text(svg, PAD.left - 8, y(m) + 5, MOODS[m].face, 'axis face', 'end');
  }
  xLabels(svg, series, x, dayLabels);

  let d = '';
  let pen = false;
  series.forEach((p, i) => {
    if (p.mood === null) { pen = false; return; }
    d += `${pen ? 'L' : 'M'}${x(i).toFixed(1)},${y(p.mood).toFixed(1)}`;
    pen = true;
  });
  if (d) el('path', { d, class: 'line mood' }, svg);
  const r = series.length > 120 ? 2 : series.length > 31 ? 3 : 4;
  series.forEach((p, i) => {
    if (p.mood !== null) el('circle', { cx: x(i), cy: y(p.mood), r, class: 'dot mood' }, svg);
  });

  const show = makeTooltip(wrap);
  hitAreas(svg, series, band, x, (p, i) => show(
    [shortDate(p.date), p.mood ? `Mood: ${MOODS[p.mood].label} (${p.mood})` : 'Belum diisi'], x(i) / width));
}

/** Batang jam tidur harian: maks 24px, ujung data membulat, jarak 2px. */
export function drawSleep(wrap, series, opts = {}) {
  const { dayLabels } = opts;
  remember(wrap, () => drawSleep(wrap, series, opts));
  sizeFor(wrap);
  const width = W; // dikunci: tooltip dihitung nanti, saat W mungkin sudah berubah
  wrap.replaceChildren();
  const svg = el('svg', { viewBox: `0 0 ${W} ${H}`, role: 'img', 'aria-label': 'Grafik jam tidur harian' });
  wrap.appendChild(svg);
  const { band, x } = frame(series);
  const maxH = Math.max(12, Math.ceil(Math.max(0, ...series.map((p) => p.sleepMinutes || 0)) / 60 / 4) * 4);
  const base = H - PAD.bottom;
  const y = (min) => base - (min / 60 / maxH) * (base - PAD.top);

  for (let h = 0; h <= maxH; h += 4) {
    el('line', { x1: PAD.left, x2: W - PAD.right, y1: y(h * 60), y2: y(h * 60), class: 'grid' }, svg);
    text(svg, PAD.left - 8, y(h * 60) + 4, `${h}j`, 'axis', 'end');
  }
  xLabels(svg, series, x, dayLabels);

  const w = Math.max(1, Math.min(24, band - 2));
  series.forEach((p, i) => {
    if (!p.sleepMinutes) return;
    const top = y(p.sleepMinutes);
    const r = Math.min(4, w / 2, base - top);
    const x0 = x(i) - w / 2;
    el('path', {
      d: `M${x0},${base}V${top + r}Q${x0},${top} ${x0 + r},${top}H${x0 + w - r}Q${x0 + w},${top} ${x0 + w},${top + r}V${base}Z`,
      class: 'bar sleep',
    }, svg);
  });

  const show = makeTooltip(wrap);
  hitAreas(svg, series, band, x, (p, i) => show(
    [shortDate(p.date), p.sleepMinutes !== null ? `Tidur: ${formatSleep(p.sleepMinutes)}` : 'Belum diisi'], x(i) / width));
}
