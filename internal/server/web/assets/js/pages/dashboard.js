import { api, token } from '../api.js';
import { t, tr, unitName, lang } from '../i18n.js';
import { el, clear, toast, busy, fmt, convert, statusBadge, pageHeader } from '../ui.js';
import { sparkline, meter } from '../charts.js';
import { store, subscribe, currentECU } from '../app.js';
import { ecuSelect, ecuPath, hasCap, keyParams, limitsFor, repairHints, sevBadge } from './common.js';

export function render(view) {
  const unsubs = [];
  const ecuCard = el('div.card');
  const codesCard = el('div.card');
  const tilesBox = el('div.grid.tiles');
  const liveCard = el('div.card', el('h2', t('dash.live')), tilesBox);
  const extra = el('div');
  let params = [];
  let tiles = {};

  const go = (hash, label) => el('button.small', { onclick: () => { location.hash = hash; } }, label);
  const report = el('button.small', { onclick: () => window.open(`/api/report?lang=${lang()}&t=${encodeURIComponent(token)}`, '_blank') }, t('dtc.report'));
  view.append(
    pageHeader(t('dash.title'), null, go('#/dtc', t('nav.dtc')), go('#/live', t('nav.live')), go('#/tests', t('nav.tests')), go('#/checks', t('nav.checks')), report),
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

  function paintCodes(codes) {
    clear(codesCard);
    codesCard.append(el('h2', t('dash.codes')));
    if (codes === undefined) {
      const b = el('button.primary', t('dash.readCodes'));
      b.onclick = () => busy(b, async () => {
        const r = await api.get(ecuPath('dtc'));
        paintCodes(r.codes);
      });
      codesCard.append(el('p.muted', '—'), b);
      return;
    }
    const n = codes.length;
    const shown = codes.slice(0, 4).map(d => ({ d, sev: el('span') }));
    codesCard.append(
      el('div.row', el('span.hero', String(n)), el('span.secondary', n ? '' : t('dtc.none'))),
      el('ul.codes', shown.map(x => el('li', el('span.code', x.d.code), ' ', tr(x.d.desc), ' ', x.sev))),
      el('button', { onclick: () => { location.hash = '#/dtc'; } }, (n ? t('dtc.howFix') : t('nav.dtc')) + ' →'));
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
    // Offer to poll the tiles' values when nothing or only part of them is polled.
    const e = currentECU() || {};
    const running = store.status.live;
    const polled = running && running.ecu === e.id ? new Set(running.params || []) : null;
    const missing = kp.filter(p => !polled || !polled.has(p.id));
    if (missing.length) {
      const b = el('button.primary', t(polled ? 'dash.addLive' : 'dash.startLive'));
      const tile = el('div.card.tile', el('p.muted', t(polled ? 'dash.addLiveHint' : 'dash.liveHint')), b);
      b.onclick = () => busy(b, async () => {
        const ids = [...(polled || []), ...missing.map(p => p.id)];
        await api.post('/api/live/start', { ecu: e.id, params: ids, intervalMs: (running && running.intervalMs) || 250 });
        tile.remove();
      });
      tilesBox.append(tile);
    }
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
          extra.append(el('div.card', el('h2', t('dash.readiness')),
            el('p', el('span.status-line' + (r.mil ? '.bad' : '.good'), r.mil ? '⚠ MIL' : '✓ MIL off'), ' · ', `${r.count} DTC`),
            el('div.grid.tiles', (r.monitors || []).map(m => el('div.row',
              el('span.status-line' + (m.complete ? '.good' : '.bad'), m.complete ? '✓' : '✕'), tr(m.name))))));
        }
      } catch (e) { /* not essential */ }
    }
    if (store.status.simulator) {
      const act = a => api.post('/api/sim/control', { action: a }).catch(e => toast(e.message, 'bad'));
      extra.append(el('div.card', el('h2', t('dash.sim')), el('div.row',
        el('button', { onclick: () => act('rev') }, '⏫ ' + t('dash.rev')),
        el('button', { onclick: () => act('stop') }, t('dash.engineStop')),
        el('button', { onclick: () => act('start') }, t('dash.engineStart')),
        el('button', { onclick: () => act('drive') }, t('dash.drive')),
        el('button', { onclick: () => act('park') }, t('dash.park')),
        el('button', { onclick: () => act('fault') }, t('dash.injectFault')))));
    }
  }

  async function paint() {
    paintECU();
    paintCodes(undefined);
    try {
      params = (await api.get(ecuPath('params'))).params || [];
    } catch (e) {
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
