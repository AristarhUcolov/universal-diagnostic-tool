import { api, token } from '../api.js';
import { t, tr, unitName, lang, fill } from '../i18n.js';
import { el, clear, toast, busy, fmt, convert, statusBadge, pageHeader, icon, button, errText } from '../ui.js';
import { sparkline, meter } from '../charts.js';
import { store, subscribe, currentECU, autoPoll } from '../app.js';
import { ecuSelect, ecuPath, hasCap, keyParams, limitsFor, repairHints, sevBadge } from './common.js';

// Codes read on the overview are kept for a minute, so going back and
// forth between pages does not ask the control unit every time. The key
// holds the start of the connection: another car never shows old codes.
const codesCache = {};
const cacheKey = id => (store.status.started || '') + '|' + id;

// rememberCodes keeps codes read elsewhere (the trouble codes page), so
// the overview shows the same list, also right after clearing.
export function rememberCodes(id, codes) {
  codesCache[cacheKey(id)] = { codes, time: Date.now() };
}

export function render(view) {
  const unsubs = [];
  const ecuCard = el('div.card');
  const codesCard = el('div.card');
  const tilesBox = el('div.grid.tiles');
  const liveCard = el('div.card', el('h2', t('dash.live')), tilesBox);
  const extra = el('div');
  let params = [];
  let tiles = {};

  const go = (hash, ic, label) => button(ic, label, () => { location.hash = hash; }, 'small');
  const report = button('printer', t('dtc.report'), () => window.open(`/api/report?lang=${lang()}&t=${encodeURIComponent(token)}`, '_blank'), 'small');
  view.append(
    pageHeader(t('dash.title'), null, go('#/dtc', 'engine', t('nav.dtc')), go('#/live', 'chart', t('nav.live')),
      go('#/gauges', 'gauge', t('nav.gauges')), go('#/tests', 'wrench', t('nav.tests')), go('#/checks', 'checks', t('nav.checks')), report),
    ecuSelect(() => paint()), el('div.grid.two', ecuCard, codesCard), liveCard, extra);

  function paintECU() {
    const e = currentECU();
    clear(ecuCard);
    if (!e) return;
    ecuCard.append(
      el('h2', t('dash.unit')),
      el('div.kv',
        el('div', t('common.title')), el('div', e.desc.name),
        el('div', t('common.protocol')), el('div', e.desc.protocol),
        e.desc.address && e.desc.address !== '-' ? [el('div', t('common.address')), el('div.mono', e.desc.address)] : null,
        el('div', t('dash.status')), el('div', statusBadge(e.desc.status)),
        el('div', t('connect.adapter')), el('div', store.status.adapter || '')),
      e.desc.notes ? el('p.muted.small', tr(e.desc.notes)) : null);
  }

  async function readCodes() {
    const e = currentECU();
    if (!e) return;
    clear(codesCard).append(el('h2', t('dash.codes')), el('p.muted', t('dash.reading')));
    try {
      const r = await api.get(ecuPath('dtc'));
      rememberCodes(e.id, r.codes || []);
      paintCodes(r.codes || []);
    } catch (err) {
      clear(codesCard).append(el('h2', t('dash.codes')), el('p', { style: { color: 'var(--critical-ink)' } }, errText(err)), readBtn());
    }
  }

  function readBtn() {
    const b = button('refresh', t('dash.readCodes'), null, 'small');
    b.onclick = () => busy(b, readCodes);
    return b;
  }

  function paintCodes(codes) {
    clear(codesCard);
    const n = codes.length;
    const shown = codes.slice(0, 4).map(d => ({ d, sev: el('span') }));
    codesCard.append(
      el('div.row', el('h2', { style: { margin: 0 } }, t('dash.codes')), el('span.grow'), readBtn()),
      n ? el('div.row', el('span.hero', String(n)))
        : el('div.row', { style: { margin: '10px 0' } }, el('span.status-line.good', icon('check-circle'), t('dtc.none'))),
      n ? el('ul.codes', shown.map(x => el('li', el('span.code', x.d.code), ' ', tr(x.d.desc), ' ', x.sev))) : null,
      button(n ? 'wrench' : 'engine', (n ? t('dtc.howFix') : t('nav.dtc')), () => { location.hash = '#/dtc'; }));
    repairHints().then(h => {
      for (const x of shown) {
        const hint = h[x.d.hint] || h.generic;
        if (hint) x.sev.append(sevBadge(hint.severity));
      }
    }).catch(() => {});
  }

  function buildTiles() {
    clear(tilesBox);
    tiles = {};
    const kp = keyParams(params);
    if (!kp.length) {
      tilesBox.append(el('p.muted', t('live.noParams')));
      return;
    }
    for (const p of kp) {
      const val = el('span.value', '—');
      const unit = el('span.unit');
      const spark = el('canvas', { style: { width: '100%', height: '34px' } });
      const meterBox = el('div');
      tilesBox.append(el('div.card.tile', el('span.label', tr(p.name)), el('div', val, unit), spark, meterBox));
      tiles[p.id] = { p, val, unit, spark, meterBox };
    }
    const e = currentECU() || {};
    const running = store.status.live;
    const polled = running && running.ecu === e.id ? new Set(running.params || []) : null;
    const missing = kp.filter(p => !polled || !polled.has(p.id));
    if (!missing.length) return;
    // Nothing polled yet: the values come by themselves.
    if (!running && !store.liveStopped) {
      autoPoll(kp.map(p => p.id));
      return;
    }
    const b = button('play', t(polled ? 'dash.addLive' : 'dash.startLive'), null, 'primary');
    const tile = el('div.card.tile', el('p.muted', t(polled ? 'dash.addLiveHint' : 'dash.liveHint')), b);
    b.onclick = () => busy(b, async () => {
      const ids = [...(polled || []), ...missing.map(p => p.id)];
      store.liveStopped = false;
      await api.post('/api/live/start', { ecu: e.id, params: ids, intervalMs: (running && running.intervalMs) || 250 });
      tile.remove();
    });
    tilesBox.append(tile);
  }

  function paintTiles() {
    for (const [id, tile] of Object.entries(tiles)) {
      const v = store.live[id];
      if (!v || !v.ok) continue;
      const c = convert(v.v, tile.p.unit, store.settings.units);
      tile.val.textContent = v.t ? v.t : fmt(c.v, tile.p.digits);
      tile.unit.textContent = unitName(c.unit);
      if (v.hist && v.hist.length > 1) sparkline(tile.spark, v.hist);
      clear(tile.meterBox).append(meter(v.v, tile.p.min, tile.p.max, limitsFor(tile.p)));
    }
  }

  async function paintExtra() {
    clear(extra);
    if (hasCap('readiness')) {
      try {
        const r = await api.get(ecuPath('readiness'));
        if (r.supported) {
          const open = (r.monitors || []).filter(m => !m.complete).length;
          extra.append(el('div.card',
            el('div.row', el('h2', { style: { margin: 0 } }, t('dash.readiness')), el('span.grow'), button('shield', t('nav.monitors'), () => { location.hash = '#/monitors'; }, 'small')),
            el('p', el('span.status-line' + (r.mil ? '.bad' : '.good'), icon(r.mil ? 'alert' : 'check-circle'), r.mil ? t('mon.milOn') : t('mon.milOff')),
              ' · ', `${r.count} DTC`, ' · ', open ? fill(t('mon.notDone'), { n: open }) : t('mon.allDone')),
            el('div.grid.tiles', (r.monitors || []).map(m => el('div.row',
              el('span.status-line' + (m.complete ? '.good' : '.bad'), m.complete ? '✓' : '✕'), tr(m.name))))));
        }
      } catch (e) { /* not essential */ }
    }
    if (store.status.simulator) {
      const act = a => api.post('/api/sim/control', { action: a }).catch(e => toast(e.message, 'bad'));
      extra.append(el('div.card', el('h2', t('dash.sim')), el('p.muted.small', t('dash.simLead')), el('div.row',
        el('button', { type: 'button', onclick: () => act('rev') }, '⏫ ' + t('dash.rev')),
        el('button', { type: 'button', onclick: () => act('stop') }, t('dash.engineStop')),
        el('button', { type: 'button', onclick: () => act('start') }, t('dash.engineStart')),
        el('button', { type: 'button', onclick: () => act('drive') }, t('dash.drive')),
        el('button', { type: 'button', onclick: () => act('launch') }, t('dash.launch')),
        el('button', { type: 'button', onclick: () => act('overheat') }, t('dash.overheat')),
        el('button', { type: 'button', onclick: () => act('cool') }, t('dash.cool')),
        el('button', { type: 'button', onclick: () => act('park') }, t('dash.park')),
        el('button', { type: 'button', onclick: () => act('fault') }, t('dash.injectFault')))));
    }
  }

  async function paint() {
    paintECU();
    const e = currentECU();
    const cached = e && codesCache[cacheKey(e.id)];
    if (cached && Date.now() - cached.time < 60000) paintCodes(cached.codes);
    else readCodes();
    try {
      params = (await api.get(ecuPath('params'))).params || [];
    } catch (err) {
      params = [];
    }
    buildTiles();
    paintTiles();
    paintExtra();
  }

  paint();
  unsubs.push(subscribe('live', paintTiles));
  unsubs.push(subscribe('status', () => paintECU()));
  return () => unsubs.forEach(u => u());
}
