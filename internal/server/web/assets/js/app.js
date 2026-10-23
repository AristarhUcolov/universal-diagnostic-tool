import { api, events } from './api.js';
import { setLang, lang, t, tr, fill, count, unitName } from './i18n.js';
import { el, clear, toast, errText, icon, emptyState, button, busy, showDialog } from './ui.js';
import * as connect from './pages/connect.js';
import * as dashboard from './pages/dashboard.js';
import * as dtcPage from './pages/dtc.js';
import * as live from './pages/live.js';
import * as info from './pages/info.js';
import * as tests from './pages/tests.js';
import * as terminal from './pages/terminal.js';
import * as bus from './pages/bus.js';
import * as journal from './pages/journal.js';
import * as reference from './pages/reference.js';
import * as settingsPage from './pages/settings.js';
import * as support from './pages/support.js';
import * as checks from './pages/checks.js';
import * as gauges from './pages/gauges.js';
import * as monitors from './pages/monitors.js';
import { startAlerts } from './alerts.js';
import { openPalette } from './palette.js';
import { releases } from './changelog.js';
import { keyParams } from './pages/common.js';

const pages = { connect, dashboard, dtc: dtcPage, live, info, tests, checks, gauges, monitors, terminal, bus, journal, reference, settings: settingsPage, support };
const needsConnection = new Set(['dashboard', 'dtc', 'live', 'info', 'tests', 'checks', 'gauges', 'monitors', 'terminal']);

// store is the shared state of the interface.
export const store = {
  status: { state: 'idle' },
  settings: {},
  info: {},
  ecu: '',
  live: {},            // parameter id -> {v, t, ok, text, l, time, min, max}
  liveError: '',
  liveRate: 0,         // polled answers a second
  liveStopped: false,  // the user stopped polling by hand in this connection
  update: null,        // the answer of the update check
  on: { status: new Set(), live: new Set(), can: new Set(), journal: new Set() },
};

export function subscribe(kind, fn) {
  store.on[kind].add(fn);
  return () => store.on[kind].delete(fn);
}

function emit(kind, data) {
  for (const fn of store.on[kind]) {
    try { fn(data); } catch (e) { console.error(e); }
  }
}

export function connected() { return store.status.state === 'connected'; }

export function currentECU() {
  const ecus = store.status.ecus || [];
  return ecus.find(e => e.id === store.ecu) || ecus[0] || null;
}

export async function saveSettings(patch) {
  Object.assign(store.settings, patch);
  store.settings = await api.post('/api/settings', store.settings);
  applyTheme();
}

// autoPoll starts polling ids when nothing is polled yet, so that a page
// shows numbers as soon as it opens. Once the user has stopped polling by
// hand in this connection, polling waits for the Start button.
export async function autoPoll(ids) {
  const e = currentECU();
  if (!e || !ids.length || store.status.live || store.liveStopped) return false;
  try {
    await api.post('/api/live/start', { ecu: e.id, params: ids, intervalMs: 250 });
    return true;
  } catch (err) {
    toast(errText(err), 'bad');
    return false;
  }
}

// stopPolling stops polling at the user's request.
export async function stopPolling() {
  store.liveStopped = true;
  await api.post('/api/live/stop');
}

// startLive polls the values last chosen for this control unit (the key
// values the first time), as the Start button of the live data page.
export async function startLive() {
  const e = currentECU();
  if (!e) return;
  const r = await api.get(`/api/ecu/${e.id}/params`);
  const ids = r.selected && r.selected.length ? r.selected : keyParams(r.params || []).map(p => p.id);
  store.liveStopped = false;
  await api.post('/api/live/start', { ecu: e.id, params: ids, intervalMs: (store.status.live && store.status.live.intervalMs) || 250 });
}

// trySimulator connects the simulated modern car without replacing the
// user's own last connection.
export async function trySimulator() {
  await api.post('/api/connect?try=1', { adapter: 'sim', profile: 'auto', simVariant: 'can' });
}

export function disconnect() {
  return api.post('/api/disconnect').catch(e => toast(errText(e), 'bad'));
}

function applyTheme() {
  const th = store.settings.theme;
  if (th === 'dark' || th === 'light') document.documentElement.dataset.theme = th;
  else delete document.documentElement.dataset.theme;
}

// ---- header ----

function updateHeader() {
  const s = store.status;
  const pill = document.getElementById('conn-pill');
  const text = document.getElementById('conn-text');
  pill.className = 'pill';
  if (s.state === 'connected') {
    pill.classList.add('ok');
    const e = currentECU();
    text.textContent = [s.adapter, e ? e.desc.name : '', e ? e.desc.protocol : ''].filter(Boolean).join(' · ');
  } else if (s.state === 'connecting') {
    pill.classList.add('busy');
    text.textContent = t('conn.connecting') + ' ' + ((s.progress || []).slice(-1)[0] || '');
  } else if (s.state === 'error') {
    pill.classList.add('bad');
    text.textContent = t('conn.error');
  } else {
    text.textContent = t('conn.idle');
  }
  pill.title = text.textContent;
  document.querySelectorAll('#nav a').forEach(a => {
    a.classList.toggle('disabled', needsConnection.has(a.dataset.page) && s.state !== 'connected');
    if (a.dataset.page === 'bus') a.classList.toggle('disabled', !s.canBus);
  });
  const rec = document.getElementById('rec-pill');
  rec.classList.toggle('hidden', !s.recording);
  rec.title = s.recording ? t('live.recording') + ' ' + s.recording : '';
  paintLivePill();
}

// The polling mark in the top bar: how many values are polled and how
// often. Its text leads to the live data page, its × stops polling. The
// buttons are made once, so keyboard focus stays on them.
const livePill = { text: el('span'), go: null, stop: null };

function setupLivePill() {
  const pill = document.getElementById('live-pill');
  livePill.go = el('button.pill-go', { type: 'button', onclick: () => { location.hash = '#/live'; } }, el('span.dot'), livePill.text);
  livePill.stop = el('button.x', { type: 'button', onclick: () => stopPolling().catch(err => toast(errText(err), 'bad')) }, icon('x', 'sm'));
  pill.append(livePill.go, livePill.stop);
}

function paintLivePill() {
  const pill = document.getElementById('live-pill');
  const l = store.status.live;
  if (!l || !connected()) { pill.classList.add('hidden'); return; }
  pill.classList.remove('hidden');
  const rate = store.liveRate >= 10 ? Math.round(store.liveRate) : store.liveRate.toFixed(1);
  livePill.text.textContent = count((l.params || []).length, 'live.values') + ' · ' + fill(t('live.rate'), { n: rate });
  livePill.go.title = t('live.pillHint');
  livePill.stop.title = t('live.stop');
  livePill.stop.setAttribute('aria-label', t('live.stop'));
}

// The rate of answers is counted over the last three seconds.
const liveTimes = [];
function countLive() {
  const now = Date.now();
  liveTimes.push(now);
  while (liveTimes.length && now - liveTimes[0] > 3000) liveTimes.shift();
}
setInterval(() => {
  const now = Date.now();
  while (liveTimes.length && now - liveTimes[0] > 3000) liveTimes.shift();
  store.liveRate = liveTimes.length / 3;
  paintLivePill();
}, 1000);

function setupHeader() {
  const app = document.getElementById('app');
  const navBtn = document.getElementById('nav-btn');
  const setNav = open => { app.classList.toggle('nav-open', open); navBtn.setAttribute('aria-expanded', String(open)); };
  navBtn.onclick = () => setNav(!app.classList.contains('nav-open'));
  document.getElementById('side-backdrop').onclick = () => setNav(false);
  window.addEventListener('hashchange', () => setNav(false));
  setupLivePill();
  document.getElementById('cmd-btn').onclick = () => openPalette();
  document.getElementById('lang-btn').onclick = () => switchLanguage();
  document.getElementById('theme-btn').onclick = async () => {
    const order = ['auto', 'dark', 'light'];
    const next = order[(order.indexOf(store.settings.theme || 'auto') + 1) % order.length];
    await saveSettings({ theme: next }).catch(() => {});
    toast(t('set.theme') + ': ' + t('set.theme' + next[0].toUpperCase() + next.slice(1)));
    paintChrome();
  };
  document.getElementById('version-chip').onclick = () => showNews(false);
  document.getElementById('update-chip').onclick = () => showUpdate();
  document.addEventListener('keydown', e => {
    if ((e.ctrlKey || e.metaKey) && !e.altKey && (e.key === 'k' || e.key === 'K' || e.code === 'KeyK')) {
      e.preventDefault();
      openPalette();
    }
  });
}

// paintChrome puts the texts of the fixed parts in the current language.
function paintChrome() {
  document.getElementById('lang-btn').textContent = lang() === 'ru' ? 'EN' : 'RU';
  document.getElementById('lang-btn').title = t('common.language');
  document.getElementById('theme-btn').title = t('set.theme') + ': ' + t('set.theme' + cap(store.settings.theme || 'auto'));
  document.getElementById('cmd-label').textContent = t('cmd.button');
  document.getElementById('cmd-btn').title = t('cmd.title') + ' (Ctrl+K)';
  document.getElementById('nav-btn').setAttribute('aria-label', t('common.menu'));
  const v = store.status.version || document.querySelector('meta[name="udt-version"]').content;
  clear(document.getElementById('version-chip')).append(icon('sparkle', 'sm'), el('span', 'v' + v + ' · ' + t('news.title')));
  paintUpdateChip();
}

function cap(s) { return s[0].toUpperCase() + s.slice(1); }

export async function switchLanguage(next) {
  next = next || (lang() === 'ru' ? 'en' : 'ru');
  setLang(next);
  await saveSettings({ language: next }).catch(() => {});
  paintChrome();
  updateHeader();
  route();
}

// ---- pages ----

let cleanup = null;
let currentPage = '';

function parseHash() {
  const raw = location.hash.replace(/^#\/?/, '');
  const [name, q] = raw.split('?');
  return { name, query: new URLSearchParams(q || '') };
}

function route() {
  const { name: hashName, query } = parseHash();
  const name = hashName || (connected() ? 'dashboard' : 'connect');
  const page = pages[name] ? name : 'connect';
  if (cleanup) { try { cleanup(); } catch (e) { console.error(e); } cleanup = null; }
  currentPage = page;
  document.querySelectorAll('#nav a').forEach(a => {
    const on = a.dataset.page === page;
    a.classList.toggle('active', on);
    if (on) a.setAttribute('aria-current', 'page'); else a.removeAttribute('aria-current');
  });
  const view = clear(document.getElementById('view'));
  document.title = t('nav.' + page) + ' — Universal Diagnostic Tool';
  // The program's own window shows the title in its title bar.
  if (typeof window.udtTitle === 'function') window.udtTitle(document.title).catch(() => {});
  if (needsConnection.has(page) && !connected()) {
    view.append(notConnected());
    return;
  }
  try {
    cleanup = pages[page].render(view, { store, query, go: h => { location.hash = h; } }) || null;
  } catch (e) {
    console.error(e);
    view.appendChild(el('div.card', el('p', String(e))));
  }
  view.focus({ preventScroll: true });
}

// notConnected explains a page that needs a vehicle and offers the two
// ways on: connecting, or trying the program on the simulator.
function notConnected() {
  const busyNow = store.status.state === 'connecting';
  const sim = button('flask', t('common.trySim'), null);
  sim.onclick = () => busy(sim, trySimulator);
  sim.disabled = busyNow;
  return emptyState('plug', t('common.notConnected'), t('common.notConnectedLead'),
    button('plug', t('common.goConnect'), () => { location.hash = '#/connect'; }, 'primary'), sim);
}

// ---- voltage ----

async function voltageLoop() {
  const v = document.getElementById('volt');
  if (connected()) {
    try {
      const r = await api.get('/api/voltage');
      clear(v).append(icon('zap', 'sm'), r.voltage.toFixed(1) + ' ' + unitName('V'));
      v.title = t('common.voltage');
      v.classList.remove('hidden');
    } catch (e) {
      v.classList.add('hidden');
    }
  } else {
    v.classList.add('hidden');
  }
  setTimeout(voltageLoop, 5000);
}

// ---- what's new and updates ----

const numeric = v => (v || '').replace(/^v/, '').split(/[-+ ]/)[0];

// showNews lists the changes of each release; auto marks the window that
// opens by itself after an update.
export function showNews(auto) {
  const cur = numeric(store.status.version || store.info.version);
  const list = el('div', releases.map(r => el('div.news-rel',
    el('div.news-head', el('b', 'v' + r.version),
      r.version === cur ? el('span.badge.info', t('news.current')) : null,
      r.date ? el('span.muted.small', new Date(r.date + 'T12:00:00').toLocaleDateString(lang())) : el('span.badge', t('news.upcoming'))),
    el('ul', (r[lang()] || r.en).map(x => el('li', x))))));
  return showDialog({
    title: auto ? fill(t('news.updated'), { v: cur }) : t('news.title'),
    body: list, wide: true,
    actions: [{ label: t('common.close'), kind: 'primary' }],
  });
}

// maybeNews opens "What's new" once after the program was updated. A new
// installation only records the version; builds between releases stay
// quiet.
async function maybeNews() {
  const v = store.info.version || '';
  if (!v || /-/.test(v)) return;
  if (store.settings.seenVersion === v) return;
  const first = !!store.info.firstRun;
  await saveSettings({ seenVersion: v }).catch(() => {});
  if (!first && releases.some(r => r.version === numeric(v))) showNews(true);
}

function paintUpdateChip() {
  const chip = document.getElementById('update-chip');
  const u = store.update;
  if (!u || !u.available || store.settings.skipVersion === u.latest) { chip.classList.add('hidden'); return; }
  chip.classList.remove('hidden');
  clear(chip).append(icon('download', 'sm'), el('span', fill(t('upd.chip'), { v: u.latest })));
  chip.title = t('upd.open');
}

// checkUpdates asks once a start whether a newer release is out.
export async function checkUpdates(force) {
  try {
    const u = await api.get('/api/update' + (force ? '?force=1' : ''));
    store.update = u;
    paintUpdateChip();
    return u;
  } catch (e) {
    return null;
  }
}

function showUpdate() {
  const u = store.update;
  if (!u || !u.available) return;
  showDialog({
    title: fill(t('upd.title'), { v: u.latest }),
    body: el('div', el('p', fill(t('upd.text'), { cur: u.current, v: u.latest })), el('p.muted.small', t('upd.how'))),
    actions: [
      { label: t('upd.skip'), kind: 'ghost', onclick: async () => { await saveSettings({ skipVersion: u.latest }).catch(() => {}); paintUpdateChip(); } },
      { label: t('upd.later') },
      { label: t('upd.download'), kind: 'primary', icon: 'external', onclick: () => openLink(u.url) },
    ],
  });
}

// openLink opens a page in the system browser: the program's own window
// does not open web pages by itself.
export function openLink(url) {
  if (window.chrome && window.chrome.webview) {
    api.post('/api/open-url', { url }).catch(err => toast(errText(err), 'bad'));
  } else {
    window.open(url, '_blank', 'noopener');
  }
}

// ---- start ----

async function main() {
  try {
    store.settings = await api.get('/api/settings');
    store.info = await api.get('/api/info');
  } catch (e) {
    document.getElementById('view').appendChild(el('div.card', el('h2', 'Access denied / Нет доступа'), el('p', e.message)));
    return;
  }
  let l = store.settings.language || store.info.systemLanguage || navigator.language || 'en';
  l = /^(ru|uk|be|kk)/.test(l) ? 'ru' : 'en';
  setLang(l);
  applyTheme();
  setupHeader();
  window.addEventListener('hashchange', route);
  if (window.chrome && window.chrome.webview) {
    document.addEventListener('click', e => {
      const a = e.target.closest('a[href^="http"]');
      if (!a) return;
      e.preventDefault();
      openLink(a.href);
    });
  }
  // Render at once from the current status; the event stream follows.
  try { store.status = await api.get('/api/status'); } catch (e) { /* stream will retry */ }
  if (store.status.ecus && store.status.ecus.length) store.ecu = store.status.ecus[0].id;
  paintChrome();
  updateHeader();
  route();
  startAlerts(store, subscribe);
  voltageLoop();
  setTimeout(() => startEvents(), 400);
  setTimeout(() => maybeNews(), 1200);
  if (store.settings.updates !== false) setTimeout(() => checkUpdates(false), 4000);
}

function startEvents() {
  events({
    status: s => {
      const was = store.status.state;
      const wasUnits = (store.status.ecus || []).map(e => e.id).join();
      if (s.state !== 'connected') store.liveStopped = false;
      store.status = s;
      if (s.ecus && !s.ecus.find(e => e.id === store.ecu)) store.ecu = s.ecus[0].id;
      updateHeader();
      emit('status', s);
      if (was !== s.state && (s.state === 'connected' || was === 'connected')) {
        if (s.state === 'connected' && currentPage === 'connect') location.hash = '#/dashboard';
        else route();
      } else if (s.state === 'connected' && wasUnits !== (s.ecus || []).map(e => e.id).join()) {
        route();
      } else if (!connected() && needsConnection.has(currentPage)) {
        // The "try the simulator" button follows the connection state.
        route();
      }
    },
    live: ev => {
      if (ev.text && !ev.values) { store.liveError = errText({ message: ev.text, code: ev.code }); emit('live', ev); return; }
      store.liveError = '';
      countLive();
      const now = Date.now();
      for (const v of ev.values || []) {
        const prev = store.live[v.id] || {};
        const entry = { ...v, time: now, min: prev.min, max: prev.max, hist: prev.hist || [] };
        if (v.ok && !v.t) {
          entry.min = prev.min === undefined ? v.v : Math.min(prev.min, v.v);
          entry.max = prev.max === undefined ? v.v : Math.max(prev.max, v.v);
          entry.hist.push(v.v);
          if (entry.hist.length > 40) entry.hist.shift();
        }
        store.live[v.id] = entry;
      }
      emit('live', ev);
    },
    can: ev => emit('can', ev.data || []),
    journal: entries => emit('journal', entries),
    onerror: () => {
      document.getElementById('conn-pill').className = 'pill bad';
      document.getElementById('conn-text').textContent = t('conn.offline');
    },
  });
}

export { tr };
main();
