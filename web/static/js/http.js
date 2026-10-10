// Helper bersama: konteks halaman, panggilan API JSON, navigasi htmx, toast.

export function ctx() {
  const el = document.getElementById('mj-ctx');
  return el ? { userId: el.dataset.userId, csrf: el.dataset.csrf } : null;
}

export class ApiError extends Error {
  constructor(status, message) {
    super(message);
    this.status = status;
  }
}

export async function api(method, path, body) {
  const c = ctx();
  const res = await fetch(path, {
    method,
    credentials: 'same-origin',
    headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': c ? c.csrf : '' },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  let data = null;
  try { data = await res.json(); } catch { /* 201/204 tanpa isi */ }
  if (!res.ok) throw new ApiError(res.status, (data && data.error) || `Kesalahan ${res.status}`);
  return data;
}

// Navigasi lewat htmx agar memori (kunci jurnal) tidak terhapus.
export async function navigate(url) {
  await window.htmx.ajax('GET', url, { target: 'body', swap: 'innerHTML' });
  history.pushState({ htmx: true }, '', url);
  window.scrollTo(0, 0);
}

export function toast(message) {
  let el = document.getElementById('mj-toast');
  if (!el) {
    el = document.createElement('div');
    el.id = 'mj-toast';
    el.className = 'toast';
    el.setAttribute('role', 'status');
    document.body.appendChild(el);
  }
  el.textContent = message;
  el.hidden = false;
  clearTimeout(el._t);
  el._t = setTimeout(() => { el.hidden = true; }, 6000);
}
