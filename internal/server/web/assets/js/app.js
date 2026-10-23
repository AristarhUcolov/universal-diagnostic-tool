import { api, events } from './api.js';
import { setLang, lang, t, tr } from './i18n.js';
import { el, clear, toast, errText } from './ui.js';
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

function applyTheme() {
  const th = store.settings.theme;
  if (th === 'dark' || th === 'light') document.documentElement.dataset.theme = th;
  else delete document.documentElement.dataset.theme;
}

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
  document.querySelectorAll('#nav a').forEach(a => {
    a.classList.toggle('disabled', needsConnection.has(a.dataset.page) && s.state !== 'connected');
    if (a.dataset.page === 'bus') a.classList.toggle('disabled', !s.canBus);
  });
  const foot = document.getElementById('side-foot');
  foot.textContent = 'v' + (s.version || document.querySelector('meta[name="udt-version"]').content);
}

let cleanup = null;
let currentPage = '';

function route() {
  const name = (location.hash.replace(/^#\/?/, '').split('?')[0]) || (connected() ? 'dashboard' : 'connect');
  const page = pages[name] ? name : 'connect';
  if (cleanup) { try { cleanup(); } catch (e) { console.error(e); } cleanup = null; }
  currentPage = page;
  document.querySelectorAll('#nav a').forEach(a => a.classList.toggle('active', a.dataset.page === page));
  const view = clear(document.getElementById('view'));
  if (needsConnection.has(page) && !connected()) {
    view.appendChild(el('div.card.empty',
      el('p', t('common.notConnected')),
      el('button.primary', { onclick: () => { location.hash = '#/connect'; } }, t('common.goConnect'))));
    return;
  }
  try {
    cleanup = pages[page].render(view, { store, go: h => { location.hash = h; } }) || null;
  } catch (e) {
    console.error(e);
    view.appendChild(el('div.card', el('p', String(e))));
  }
  view.focus({ preventScroll: true });
}

function rerender() { route(); }

async function voltageLoop() {
  const v = document.getElementById('volt');
  if (connected()) {
    try {
      const r = await api.get('/api/voltage');
      v.textContent = '⚡ ' + r.voltage.toFixed(1) + ' V';
      v.classList.remove('hidden');
    } catch (e) {
      v.classList.add('hidden');
    }
  } else {
    v.classList.add('hidden');
  }
  setTimeout(voltageLoop, 5000);
}

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
  document.getElementById('lang-btn').textContent = lang() === 'ru' ? 'EN' : 'RU';
  document.getElementById('lang-btn').onclick = async () => {
    const next = lang() === 'ru' ? 'en' : 'ru';
    setLang(next);
    document.getElementById('lang-btn').textContent = next === 'ru' ? 'EN' : 'RU';
    await saveSettings({ language: next }).catch(() => {});
    updateHeader();
    rerender();
  };
  document.getElementById('theme-btn').onclick = async () => {
    const order = ['auto', 'dark', 'light'];
    const next = order[(order.indexOf(store.settings.theme || 'auto') + 1) % order.length];
    await saveSettings({ theme: next }).catch(() => {});
    toast(t('set.theme') + ': ' + t('set.theme' + next[0].toUpperCase() + next.slice(1)));
  };
  window.addEventListener('hashchange', route);
  // The program's own window (WebView2) does not open web pages by itself,
  // so external links are handed to the system browser.
  if (window.chrome && window.chrome.webview) {
    document.addEventListener('click', e => {
      const a = e.target.closest('a[href^="http"]');
      if (!a) return;
      e.preventDefault();
      api.post('/api/open-url', { url: a.href }).catch(err => toast(err.message, 'bad'));
    });
  }
  // Render at once from the current status; the event stream follows.
  try { store.status = await api.get('/api/status'); } catch (e) { /* stream will retry */ }
  if (store.status.ecus && store.status.ecus.length) store.ecu = store.status.ecus[0].id;
  updateHeader();
  route();
  startAlerts(store, subscribe);
  voltageLoop();
  setTimeout(() => startEvents(), 400);
}

function startEvents() {
  events({
    status: s => {
      const was = store.status.state;
      const wasUnits = (store.status.ecus || []).map(e => e.id).join();
      store.status = s;
      if (s.ecus && !s.ecus.find(e => e.id === store.ecu)) store.ecu = s.ecus[0].id;
      updateHeader();
      emit('status', s);
      if (was !== s.state && (s.state === 'connected' || was === 'connected')) {
        if (s.state === 'connected' && currentPage === 'connect') location.hash = '#/dashboard';
        else rerender();
      } else if (s.state === 'connected' && wasUnits !== (s.ecus || []).map(e => e.id).join()) {
        rerender();
      }
    },
    live: ev => {
      if (ev.text && !ev.values) { store.liveError = errText({ message: ev.text, code: ev.code }); emit('live', ev); return; }
      store.liveError = '';
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
      document.getElementById('conn-text').textContent = 'offline';
    },
  });
}

export { tr };
main();
