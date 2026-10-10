// Bar publik: menu lipat di layar sempit dan bayangan saat halaman digulir.
// Menu memakai <details>, jadi tetap bisa dibuka tanpa JavaScript; di sini
// hanya ditutup otomatis (klik tautan, klik di luar, Escape, layar melebar).
// Semua listener didelegasikan ke document agar tetap jalan setelah hx-boost
// mengganti isi <body>.

const WIDE = window.matchMedia('(min-width: 980px)');

function openMenu() {
  return document.querySelector('.pubmenu[open]');
}

function closeMenu(focusToggle = false) {
  const menu = openMenu();
  if (!menu) return;
  menu.open = false;
  if (focusToggle) menu.querySelector('summary')?.focus();
}

document.addEventListener('click', (e) => {
  const menu = openMenu();
  if (!menu) return;
  if (!menu.contains(e.target) || e.target.closest('.pubmenu-panel a')) closeMenu();
});

document.addEventListener('keydown', (e) => {
  if (e.key === 'Escape' && openMenu()) closeMenu(true);
});

WIDE.addEventListener('change', (e) => { if (e.matches) closeMenu(); });

function markScrolled() {
  document.querySelector('[data-role="pubbar"]')?.classList.toggle('scrolled', window.scrollY > 4);
}
window.addEventListener('scroll', markScrolled, { passive: true });
document.addEventListener('DOMContentLoaded', markScrolled);
document.addEventListener('htmx:afterSettle', markScrolled);
