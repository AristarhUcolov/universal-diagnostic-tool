import { t } from './i18n.js';

// Pages build their content with optional parts (cond ? node : null).
// The DOM's append() would print "null" for those, so skip empty values.
const nativeAppend = Element.prototype.append;
Element.prototype.append = function (...nodes) {
  nativeAppend.apply(this, nodes.flat(Infinity).filter(n => n !== null && n !== undefined && n !== false));
};

// Where the program can be supported.
export const supportLinks = [
  { name: 'Ko-fi', url: 'https://ko-fi.com/AristarhUcolov', label: 'ko-fi.com/AristarhUcolov', key: 'support.kofi', color: '#ff5e5b' },
  { name: 'Buy Me a Coffee', url: 'https://buymeacoffee.com/aristarh.ucolov', label: 'buymeacoffee.com/aristarh.ucolov', key: 'support.bmc', color: '#ffdd00' },
  { name: 'DonationAlerts', url: 'https://www.donationalerts.com/c/aristarh_ucolov', label: 'donationalerts.com/c/aristarh_ucolov', key: 'support.da', color: '#f57d07' },
];

// el builds DOM elements: el('div.card', {onclick}, child, 'text', ...).
export function el(spec, attrs, ...children) {
  const [tagPart, ...classes] = spec.split('.');
  const [tag, id] = tagPart.split('#');
  const node = document.createElement(tag || 'div');
  if (id) node.id = id;
  if (classes.length) node.className = classes.join(' ');
  if (attrs && (typeof attrs !== 'object' || attrs instanceof Node || Array.isArray(attrs))) {
    children.unshift(attrs);
    attrs = null;
  }
  for (const [k, v] of Object.entries(attrs || {})) {
    if (v === undefined || v === null || v === false) continue;
    if (k.startsWith('on')) node.addEventListener(k.slice(2), v);
    else if (k === 'style' && typeof v === 'object') Object.assign(node.style, v);
    else if (k === 'dataset') Object.assign(node.dataset, v);
    else if (k in node && typeof v !== 'string') node[k] = v;
    else node.setAttribute(k, v === true ? '' : v);
  }
  append(node, children);
  return node;
}

function append(node, children) {
  for (const c of children.flat(Infinity)) {
    if (c === null || c === undefined || c === false) continue;
    node.appendChild(c instanceof Node ? c : document.createTextNode(String(c)));
  }
}

export function clear(node) {
  while (node.firstChild) node.removeChild(node.firstChild);
  return node;
}

export function toast(msg, kind = '') {
  const box = document.getElementById('toasts');
  const n = el('div.toast' + (kind ? '.' + kind : ''), msg);
  box.appendChild(n);
  setTimeout(() => n.remove(), kind === 'bad' ? 9000 : 4000);
}

// confirmBox shows a modal question and resolves to true or false.
export function confirmBox(title, text, okText, danger) {
  const dlg = document.getElementById('dialog');
  document.getElementById('dlg-title').textContent = title;
  clear(document.getElementById('dlg-body')).appendChild(el('p', text));
  const ok = document.getElementById('dlg-ok');
  ok.textContent = okText || t('common.ok');
  ok.className = danger ? 'danger' : 'primary';
  document.getElementById('dlg-cancel').textContent = t('common.cancel');
  return new Promise(resolve => {
    dlg.onclose = () => resolve(dlg.returnValue === 'ok');
    dlg.showModal();
  });
}

// errText explains an error in the interface language where the server
// gave a reason code (an output test that needs the engine stopped).
export function errText(e) {
  if (e && e.code) {
    const key = 'err.' + e.code;
    const s = t(key);
    if (s !== key) return s;
  }
  return e && e.message ? e.message : String(e);
}

// pageHeader is the title row of a page: title, a lead line and the
// page's main actions on the right.
export function pageHeader(title, lead, ...actions) {
  return el('div.page-head', el('div', el('h1', title), lead ? el('p.secondary.lead', lead) : null),
    actions.length ? el('div.row.page-actions', actions) : null);
}

// busy disables a button while an async action runs.
export async function busy(btn, fn) {
  const old = btn.textContent;
  btn.disabled = true;
  btn.textContent = old + ' …';
  try {
    return await fn();
  } catch (e) {
    toast(errText(e), 'bad');
    return undefined;
  } finally {
    btn.disabled = false;
    btn.textContent = old;
  }
}

export function fmt(v, digits) {
  if (v === null || v === undefined || Number.isNaN(v)) return '—';
  const d = digits === undefined ? 2 : digits;
  let s = Number(v).toFixed(d);
  if (s.includes('.')) s = s.replace(/0+$/, '').replace(/\.$/, '');
  return s === '-0' ? '0' : s;
}

// convert applies the unit preference to a value.
export function convert(value, unit, units) {
  if (units !== 'imperial') return { v: value, unit };
  switch (unit) {
    case '°C': return { v: value * 9 / 5 + 32, unit: '°F' };
    case 'km/h': return { v: value * 0.621371, unit: 'mph' };
    case 'kPa': return { v: value * 0.145038, unit: 'psi' };
    case 'km': return { v: value * 0.621371, unit: 'mi' };
    case 'l/100km': return { v: value > 0 ? 235.215 / value : 0, unit: 'mpg' };
    case 'l/h': return { v: value * 0.264172, unit: 'gal/h' };
    default: return { v: value, unit };
  }
}

export function statusBadge(status) {
  const cls = { verified: 'good', partial: 'info', unverified: 'warn', generic: 'info' }[status] || '';
  return el('span.badge' + (cls ? '.' + cls : ''), t('status.' + status) || status);
}

export function copyText(text) {
  if (navigator.clipboard) navigator.clipboard.writeText(text).then(() => toast(t('common.copied'), 'good'));
}
