// The command palette (Ctrl+K): any page, action, polled value or trouble
// code from a few letters, without the mouse.
import { api, token } from './api.js';
import { t, tr, unitName, lang, fill } from './i18n.js';
import { el, clear, icon, toast, errText } from './ui.js';
import {
  store, connected, currentECU, stopPolling, startLive, trySimulator, disconnect, saveSettings,
  switchLanguage, checkUpdates, showNews,
} from './app.js';
import { groupName } from './pages/common.js';

const needsCar = new Set(['dashboard', 'dtc', 'live', 'info', 'tests', 'checks', 'gauges', 'monitors', 'terminal']);
const codeLike = /^([pcbu][0-9a-f]{4}|\d{3,5}|[0-9a-f]{4,6})$/i;

let dlg = null;

export function openPalette() {
  if (dlg) { dlg.querySelector('input').focus(); return; }
  const input = el('input', { type: 'text', placeholder: t('cmd.placeholder'), 'aria-label': t('cmd.title'), autocomplete: 'off', spellcheck: false });
  const list = el('div.pal-list', { role: 'listbox' });
  dlg = el('dialog.palette', { 'aria-label': t('cmd.title') },
    el('div.pal-search', icon('search'), input, el('kbd', 'Esc')),
    list,
    el('div.pal-foot',
      el('span', el('kbd', '↑'), el('kbd', '↓'), t('cmd.move')),
      el('span', el('kbd', 'Enter'), t('cmd.run')),
      el('span', el('kbd', 'Ctrl K'), t('cmd.title'))));
  let items = [];
  let sel = 0;
  let params = null;

  const close = () => { if (dlg) dlg.close(); };
  // Going to another page (the Back button) closes the palette too.
  window.addEventListener('hashchange', close);
  dlg.addEventListener('close', () => { window.removeEventListener('hashchange', close); dlg.remove(); dlg = null; });
  // A click on the darkened page around the palette closes it.
  dlg.addEventListener('click', e => { if (e.target === dlg) close(); });

  function paint() {
    const q = input.value.trim();
    items = build(q, params);
    sel = Math.min(sel, Math.max(0, items.length - 1));
    clear(list);
    if (!items.length) { list.append(el('div.pal-empty', t('cmd.nothing'))); return; }
    let group = '';
    items.forEach((it, i) => {
      if (it.group !== group) { group = it.group; list.append(el('div.pal-group', group)); }
      it.node = el('button.pal-item' + (i === sel ? '.sel' : ''), {
        type: 'button', role: 'option', tabindex: -1,
        onclick: () => run(it),
        onmousemove: () => { if (sel !== i) { sel = i; mark(); } },
      }, icon(it.icon), el('span', it.label), it.hint ? el('span.hint', it.hint) : null);
      list.append(it.node);
    });
  }

  function mark() {
    items.forEach((it, i) => it.node.classList.toggle('sel', i === sel));
    if (items[sel]) items[sel].node.scrollIntoView({ block: 'nearest' });
  }

  async function run(it) {
    close();
    try { await it.run(); } catch (e) { toast(errText(e), 'bad'); }
  }

  input.addEventListener('input', () => { sel = 0; paint(); });
  input.addEventListener('keydown', e => {
    const n = items.length;
    if (e.key === 'ArrowDown' && n) { e.preventDefault(); sel = (sel + 1) % n; mark(); }
    else if (e.key === 'ArrowUp' && n) { e.preventDefault(); sel = (sel - 1 + n) % n; mark(); }
    else if (e.key === 'Enter') { e.preventDefault(); if (items[sel]) run(items[sel]); }
  });

  document.body.appendChild(dlg);
  dlg.showModal();
  input.focus();
  paint();
  const e = currentECU();
  if (connected() && e) {
    api.get(`/api/ecu/${e.id}/params`).then(r => { params = r.params || []; if (dlg) paint(); }).catch(() => {});
  }
}

// score orders the matches: a label starting with the query first, then
// one with a word starting with it, then any other. -1 means no match.
function score(q, ...texts) {
  if (!q) return 2;
  let best = -1;
  for (const s of texts) {
    if (!s) continue;
    const v = s.toLowerCase();
    const i = v.indexOf(q);
    if (i < 0) continue;
    const sc = i === 0 ? 0 : /[\s(«"/.-]/.test(v[i - 1]) ? 1 : 2;
    if (best < 0 || sc < best) best = sc;
  }
  return best;
}

function build(raw, params) {
  const q = raw.toLowerCase();
  const groups = [];
  const add = (group, list) => {
    const found = list.map(it => ({ ...it, group, s: score(q, it.label, it.hint, it.key) })).filter(it => it.s >= 0);
    found.sort((a, b) => a.s - b.s);
    groups.push(...found);
  };

  // Actions that fit the moment.
  const acts = [];
  if (connected()) {
    acts.push(store.status.live
      ? { icon: 'stop', label: t('cmd.liveStop'), run: () => stopPolling() }
      : { icon: 'play', label: t('cmd.liveStart'), run: () => startLive() });
    acts.push(store.status.recording
      ? { icon: 'stop', label: t('live.recordStop'), run: () => api.post('/api/record/stop') }
      : { icon: 'rec', label: t('live.record'), run: () => api.post('/api/record/start') });
    acts.push({ icon: 'engine', label: t('cmd.readCodes'), run: () => { location.hash = '#/dtc'; } });
    acts.push({ icon: 'printer', label: t('dtc.report'), run: () => window.open(`/api/report?lang=${lang()}&t=${encodeURIComponent(token)}`, '_blank') });
    acts.push({ icon: 'unplug', label: t('connect.disconnect'), run: () => disconnect() });
  } else if (store.status.state !== 'connecting') {
    const last = store.settings.last;
    if (last && last.adapter && last.adapter !== 'sim') {
      const body = Object.fromEntries(Object.entries(last).filter(([, v]) => v !== '' && v !== null && v !== undefined));
      acts.push({ icon: 'plug', label: t('cmd.connectLast'), hint: [last.adapter, last.port].filter(Boolean).join(' · '), run: () => api.post('/api/connect', body) });
    }
    acts.push({ icon: 'flask', label: t('common.trySim'), run: () => trySimulator() });
  }
  acts.push(
    { icon: 'theme', label: t('cmd.themeDark'), run: () => saveSettings({ theme: 'dark' }) },
    { icon: 'theme', label: t('cmd.themeLight'), run: () => saveSettings({ theme: 'light' }) },
    { icon: 'theme', label: t('cmd.themeAuto'), run: () => saveSettings({ theme: 'auto' }) },
    { icon: 'languages', label: lang() === 'ru' ? 'Switch to English' : 'Переключить на русский', key: 'english русский language язык', run: () => switchLanguage() },
    { icon: 'sparkle', label: t('news.title'), run: () => showNews(false) },
    { icon: 'download', label: t('upd.check'), run: checkNow },
    { icon: 'folder', label: t('cmd.openLogs'), run: () => api.post('/api/open-folder', { kind: 'logs' }) });
  add(t('cmd.actions'), acts);

  // Every page of the menu.
  const pages = [...document.querySelectorAll('#nav a')].map(a => ({
    icon: a.querySelector('use').getAttribute('href').replace('#i-', ''),
    label: a.textContent.trim(),
    key: a.dataset.page,
    hint: needsCar.has(a.dataset.page) && !connected() ? t('cmd.needsCar') : '',
    run: () => { location.hash = a.getAttribute('href'); },
  }));
  add(t('cmd.pages'), pages);

  // The values of the connected control unit.
  if (params && q.length >= 2) {
    add(t('cmd.values'), params.map(p => ({
      icon: 'chart', label: tr(p.name), key: p.id,
      hint: [unitName(p.unit), p.group ? groupName(p.group) : ''].filter(Boolean).join(' · '),
      run: () => { location.hash = '#/live?q=' + encodeURIComponent(tr(p.name)); },
    })).filter(p => score(q, p.label, p.key) >= 0).slice(0, 8));
  }

  // A trouble code or a word to look up in the offline database.
  if (q.length >= 3 || codeLike.test(raw)) {
    groups.push({
      group: t('cmd.codes'), icon: 'search', label: fill(t('cmd.findCode'), { q: raw }), hint: t('nav.reference'),
      run: () => { location.hash = '#/reference?q=' + encodeURIComponent(raw); },
    });
  }
  return groups;
}

async function checkNow() {
  const u = await checkUpdates(true);
  if (!u || u.error) toast(fill(t('upd.failed'), { e: u ? u.error : '—' }), 'bad');
  else if (!u.available) toast(fill(t('upd.none'), { v: u.current }), 'good');
  else toast(fill(t('upd.title'), { v: u.latest }), 'good');
}
