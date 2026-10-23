import { api } from '../api.js';
import { t, tr, unitName, count, fill } from '../i18n.js';
import { el, clear, toast, busy, fmt, convert, errText, icon, pageHeader } from '../ui.js';
import { StripChart } from '../charts.js';
import { store, subscribe, currentECU, autoPoll, stopPolling } from '../app.js';
import { ecuSelect, ecuPath, hasCap, keyParams, groupName } from './common.js';

export function render(view, { query }) {
  const unsubs = [];
  let params = [];
  let selected = new Set();
  let mode = 'table';
  let charts = {};
  let rawTimer = null;
  let applyTimer = null;

  const listBox = el('div.param-list');
  const presets = el('div.row.presets');
  const filter = el('input', { type: 'search', placeholder: t('live.filter'), style: { width: '100%' }, value: (query && query.get('q')) || '' });
  const startBtn = el('button.primary', { type: 'button' });
  const interval = el('select',
    el('option', { value: 0 }, t('live.fast')),
    el('option', { value: 250, selected: true }, t('live.normal')),
    el('option', { value: 1000 }, t('live.slow')));
  const recBtn = el('button', { type: 'button' });
  const recInfo = el('span.muted.small');
  const errBox = el('div.small', { style: { color: 'var(--critical-ink)' } });
  const countBox = el('span.muted.small.live-count');
  const tabs = el('div.tabs', { role: 'tablist' });
  const content = el('div');

  view.append(
    pageHeader(t('live.title'), t('live.lead')),
    ecuSelect(() => load(true)),
    el('div.live-layout',
      el('div.card.live-side', el('h2', t('live.params')), presets, filter, listBox),
      el('div',
        el('div.card', el('div.row.live-bar', startBtn, el('label.field', t('live.interval'), interval), recBtn, countBox), recInfo, errBox),
        tabs,
        content)));

  function running() {
    const l = store.status.live;
    return l && l.ecu === (currentECU() || {}).id ? l : (l && !l.ecu ? l : null);
  }

  // chosen lists the ticked values in the order of the list.
  function chosen() {
    return params.filter(p => selected.has(p.id)).map(p => p.id);
  }

  function paintControls() {
    const r = running();
    startBtn.replaceChildren(icon(r ? 'stop' : 'play'), r ? t('live.stop') : t('live.start'));
    startBtn.classList.toggle('danger', !!r);
    startBtn.classList.toggle('primary', !r);
    const rec = store.status.recording;
    recBtn.replaceChildren(icon(rec ? 'stop' : 'rec', rec ? '' : 'fill rec-dot'), rec ? t('live.recordStop') : t('live.record'));
    recInfo.textContent = rec ? t('live.recording') + ' ' + rec : recInfo.dataset.saved || '';
    paintCount();
  }

  function paintCount() {
    const r = running();
    const n = selected.size;
    countBox.textContent = fill(t('live.selected'), { n }) + (r ? ' · ' + fill(t('live.rate'), { n: store.liveRate.toFixed(1) }) : '');
  }

  startBtn.onclick = () => busy(startBtn, async () => {
    if (running()) { await stopPolling(); return; }
    const ids = chosen();
    if (!ids.length) { toast(t('live.chooseParams')); return; }
    store.liveStopped = false;
    await api.post('/api/live/start', { ecu: currentECU().id, params: ids, intervalMs: +interval.value });
  });

  recBtn.onclick = () => busy(recBtn, async () => {
    if (store.status.recording) {
      const r = await api.post('/api/record/stop');
      recInfo.dataset.saved = r.file ? t('common.saved') + ': ' + r.file : '';
    } else {
      recInfo.dataset.saved = '';
      await api.post('/api/record/start');
      if (!running() && chosen().length) {
        store.liveStopped = false;
        await api.post('/api/live/start', { ecu: currentECU().id, params: chosen(), intervalMs: +interval.value });
      }
    }
  });

  // While values are polled, a change of the selection or of the update
  // rate takes effect at once: the polled set is replaced after a short
  // pause, so ticking several boxes in a row restarts polling only once.
  function selectionChanged() {
    paintContent();
    paintCount();
    if (!running()) return;
    clearTimeout(applyTimer);
    applyTimer = setTimeout(apply, 350);
  }

  async function apply() {
    clearTimeout(applyTimer);
    applyTimer = null;
    if (!running()) return;
    const ids = chosen();
    try {
      if (!ids.length) await stopPolling();
      else await api.post('/api/live/start', { ecu: currentECU().id, params: ids, intervalMs: +interval.value });
    } catch (e) {
      toast(errText(e), 'bad');
    }
  }

  interval.onchange = () => { if (running()) apply(); };

  function paintList() {
    clear(listBox);
    if (!params.length) {
      listBox.append(el('p.muted', t('live.noParams')));
      return;
    }
    const q = filter.value.trim().toLowerCase();
    const groups = {};
    for (const p of params) {
      const name = tr(p.name);
      if (q && !name.toLowerCase().includes(q) && !p.id.toLowerCase().includes(q)) continue;
      (groups[p.group || '—'] = groups[p.group || '—'] || []).push(p);
    }
    if (!Object.keys(groups).length) listBox.append(el('p.muted', t('cmd.nothing')));
    for (const [g, ps] of Object.entries(groups)) {
      const setAll = on => { ps.forEach(p => (on ? selected.add(p.id) : selected.delete(p.id))); paintList(); selectionChanged(); };
      listBox.append(el('div.param-group',
        el('b', groupName(g), el('a', { href: '#', onclick: e => { e.preventDefault(); setAll(true); } }, t('live.selectAll')), '/',
          el('a', { href: '#', onclick: e => { e.preventDefault(); setAll(false); } }, t('live.selectNone'))),
        ps.map(p => el('label.param-item',
          el('input', { type: 'checkbox', checked: selected.has(p.id), onchange: e => { e.target.checked ? selected.add(p.id) : selected.delete(p.id); selectionChanged(); } }),
          el('span', tr(p.name), p.unit ? el('small', ' ' + unitName(p.unit)) : null,
            p.verified ? null : el('span.unverified', { title: tr(p.note) || t('common.unverified') }, ' ?'))))));
    }
    if (params.some(p => !p.verified)) listBox.append(el('p.muted.small', t('live.unverifiedLegend')));
  }
  filter.oninput = paintList;

  function paintTabs() {
    clear(tabs);
    const tab = (id, ic, label) => el('button' + (mode === id ? '.on' : ''), {
      type: 'button', role: 'tab', 'aria-selected': String(mode === id),
      onclick: () => { mode = id; paintTabs(); paintContent(); },
    }, icon(ic, 'sm'), label);
    tabs.append(tab('table', 'journal', t('live.table')), tab('charts', 'chart', t('live.charts')));
    if (hasCap('blocks')) tabs.append(tab('raw', 'cpu', t('live.raw')));
  }

  const rows = {};
  function paintContent() {
    clearInterval(rawTimer);
    clear(content);
    charts = {};
    for (const k of Object.keys(rows)) delete rows[k];
    if (mode === 'raw') { rawView(); return; }
    const sel = params.filter(p => selected.has(p.id));
    if (!sel.length) { content.append(el('div.card.empty', t('live.chooseParams'))); return; }
    if (mode === 'table') {
      const tbody = el('tbody');
      for (const p of sel) {
        const v = el('td.num', '—'), mn = el('td.num.muted'), mx = el('td.num.muted'), u = el('td.muted', unitName(convert(0, p.unit, store.settings.units).unit));
        rows[p.id] = { p, v, mn, mx, u };
        tbody.append(el('tr', el('td', tr(p.name), p.verified ? null : el('span.unverified', { title: tr(p.note) || t('common.unverified') }, ' ?')), v, u, mn, mx));
      }
      content.append(el('div.card.table-wrap', el('table.live-table',
        el('thead', el('tr', el('th', t('common.name')), el('th', { style: { textAlign: 'right' } }, t('common.value')), el('th', t('common.unit')),
          el('th', { style: { textAlign: 'right' } }, t('common.min')), el('th', { style: { textAlign: 'right' } }, t('common.max')))),
        tbody)));
    } else {
      const box = el('div.charts');
      for (const p of sel.filter(p => p.kind !== 'text').slice(0, 16)) {
        const c = convert(p.max, p.unit, store.settings.units);
        const cmin = convert(p.min, p.unit, store.settings.units);
        const ch = new StripChart({ title: tr(p.name), unit: unitName(c.unit), min: cmin.v, max: c.v, digits: p.digits });
        ch.p = p;
        charts[p.id] = ch;
        box.append(ch.root);
      }
      content.append(box);
    }
    paintValues();
  }

  function paintValues() {
    errBox.textContent = store.liveError ? t('live.err') + ': ' + store.liveError : '';
    const units = store.settings.units;
    for (const r of Object.values(rows)) {
      const v = store.live[r.p.id];
      if (!v || !r.v.isConnected) continue;
      if (!v.ok) { r.v.textContent = '—'; continue; }
      const text = v.l ? tr(v.l) : v.t;
      const c = convert(v.v, r.p.unit, units);
      r.v.textContent = text || fmt(c.v, r.p.digits);
      r.u.textContent = text ? '' : unitName(c.unit);
      r.v.title = v.raw ? 'raw: ' + v.raw : '';
      if (v.min !== undefined && !text) {
        r.mn.textContent = fmt(convert(v.min, r.p.unit, units).v, r.p.digits);
        r.mx.textContent = fmt(convert(v.max, r.p.unit, units).v, r.p.digits);
      }
    }
    for (const [id, ch] of Object.entries(charts)) {
      const v = store.live[id];
      if (v && v.ok && !v.t) ch.push(v.time, convert(v.v, ch.p.unit, units).v);
    }
  }

  function rawView() {
    const sel = el('select');
    const auto = el('input', { type: 'checkbox', checked: true });
    const grid = el('div.hexgrid');
    const info = el('div.muted.small');
    let prev = [];
    content.append(el('div.card', el('p.secondary', t('live.rawHint')),
      el('div.row', el('label.field', t('live.rawRecord'), sel), el('label.check', auto, t('live.rawAuto')),
        el('button', { type: 'button', onclick: () => readRaw() }, icon('refresh'), t('common.read'))), info, grid));
    api.get(ecuPath('blocks')).then(r => {
      for (const b of r.blocks || []) sel.append(el('option', { value: b }, b));
      readRaw();
    });
    async function readRaw() {
      if (!sel.value) return;
      try {
        const r = await api.post(ecuPath('block'), { id: sel.value });
        clear(grid);
        r.bytes.forEach((b, i) => {
          grid.append(el('div.hexcell' + (prev.length === r.bytes.length && prev[i] !== b ? '.changed' : ''),
            el('span.i', '#' + i), b.toString(16).toUpperCase().padStart(2, '0'), el('span.d', String(b))));
        });
        info.textContent = `${r.bytes.length} bytes · ${new Date(r.time).toLocaleTimeString()}`;
        prev = r.bytes;
      } catch (e) {
        info.textContent = errText(e);
      }
    }
    rawTimer = setInterval(() => { if (auto.checked && content.isConnected) readRaw(); }, 700);
  }

  // load reads the values of the current control unit; switched means
  // the user picked another unit, and polling moves over to it.
  async function load(switched) {
    let saved = [];
    try {
      const r = await api.get(ecuPath('params'));
      params = r.params || [];
      saved = r.selected || [];
    } catch (e) {
      params = [];
      toast(errText(e), 'bad');
    }
    // The ticks show what is polled now; otherwise the last choice for
    // this control unit, or the key values the first time.
    const known = new Set(params.map(p => p.id));
    const r = running();
    const now = r ? (r.params || []).filter(id => known.has(id)) : [];
    selected = new Set(now.length ? now : saved.length ? saved : keyParams(params).map(p => p.id));
    if (r && r.intervalMs !== undefined) interval.value = String(r.intervalMs);
    paintPresets();
    paintList();
    paintTabs();
    paintContent();
    paintControls();
    if (r) return;
    if (switched && store.status.live && !store.liveStopped && chosen().length) {
      api.post('/api/live/start', { ecu: currentECU().id, params: chosen(), intervalMs: +interval.value })
        .catch(e => toast(errText(e), 'bad'));
    } else {
      autoPoll(chosen());
    }
  }

  // paintPresets offers ready-made selections for common jobs.
  function paintPresets() {
    clear(presets);
    const sets = [
      ['live.pKey', keyParams(params).map(p => p.id)],
      ['live.pIdle', params.filter(p => p.group === 'idle' || ['0C', '05'].includes(p.obd)).map(p => p.id)],
      ['live.pMix', params.filter(p => ['fuel', 'o2'].includes(p.group) || ['0C', '10'].includes(p.obd)).map(p => p.id)],
    ];
    for (const [key, ids] of sets) {
      if (ids.length < 2) continue;
      presets.append(el('button.small', { type: 'button', onclick: () => { selected = new Set(ids); paintList(); selectionChanged(); } }, t(key)));
    }
    if (presets.childNodes.length) {
      presets.append(el('button.small.ghost', { type: 'button', onclick: () => { selected = new Set(); paintList(); selectionChanged(); } }, t('live.pNone')));
    }
  }

  load();
  unsubs.push(subscribe('live', () => { paintValues(); paintCount(); }));
  unsubs.push(subscribe('status', paintControls));
  return () => {
    clearInterval(rawTimer);
    // A change made just before leaving the page is still applied.
    if (applyTimer) apply();
    unsubs.forEach(u => u());
  };
}
