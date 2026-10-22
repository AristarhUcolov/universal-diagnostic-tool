import { api } from '../api.js';
import { t, tr, unitName } from '../i18n.js';
import { el, clear, toast, busy, fmt, convert } from '../ui.js';
import { StripChart } from '../charts.js';
import { store, subscribe, currentECU } from '../app.js';
import { ecuSelect, ecuPath, hasCap, keyParams, groupName } from './common.js';

export function render(view) {
  const unsubs = [];
  let params = [];
  let selected = new Set();
  let mode = 'table';
  let charts = {};
  let rawTimer = null;

  const listBox = el('div.param-list');
  const filter = el('input', { placeholder: t('live.filter'), style: { width: '100%' } });
  const startBtn = el('button.primary');
  const interval = el('select',
    el('option', { value: 0 }, t('live.fast')),
    el('option', { value: 250, selected: true }, t('live.normal')),
    el('option', { value: 1000 }, t('live.slow')));
  const recBtn = el('button');
  const recInfo = el('span.muted.small');
  const errBox = el('div.muted.small');
  const tabs = el('div.row');
  const content = el('div');

  view.append(
    el('h1', t('live.title')),
    ecuSelect(() => { stop().then(load); }),
    el('div.live-layout',
      el('div.card', el('h2', t('live.params')), filter, listBox),
      el('div',
        el('div.card', el('div.row', startBtn, el('label.field', t('live.interval'), interval), recBtn, recInfo), errBox),
        el('div', { style: { margin: '12px 0' } }, tabs),
        content)));

  function running() {
    const l = store.status.live;
    return l && l.ecu === (currentECU() || {}).id ? l : (l && !l.ecu ? l : null);
  }

  function paintControls() {
    const r = running();
    startBtn.textContent = r ? '■ ' + t('live.stop') : '▶ ' + t('live.start');
    startBtn.className = r ? 'danger' : 'primary';
    recBtn.textContent = store.recording ? '■ ' + t('live.recordStop') : '● ' + t('live.record');
  }

  startBtn.onclick = () => busy(startBtn, async () => {
    if (running()) { await stop(); return; }
    const ids = params.filter(p => selected.has(p.id)).map(p => p.id);
    if (!ids.length) { toast(t('live.chooseParams')); return; }
    await api.post('/api/live/start', { ecu: currentECU().id, params: ids, intervalMs: +interval.value });
  });

  recBtn.onclick = () => busy(recBtn, async () => {
    if (store.recording) {
      const r = await api.post('/api/record/stop');
      store.recording = false;
      recInfo.textContent = r.file ? t('common.saved') + ': ' + r.file : '';
    } else {
      const r = await api.post('/api/record/start');
      store.recording = true;
      recInfo.textContent = t('live.recording') + ' ' + r.file;
    }
    paintControls();
  });

  async function stop() {
    if (running()) await api.post('/api/live/stop');
  }

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
    for (const [g, ps] of Object.entries(groups)) {
      const setAll = on => { ps.forEach(p => (on ? selected.add(p.id) : selected.delete(p.id))); paintList(); paintContent(); };
      listBox.append(el('div.param-group',
        el('b', groupName(g), ' ', el('a', { href: '#', onclick: e => { e.preventDefault(); setAll(true); } }, t('live.selectAll')), ' / ',
          el('a', { href: '#', onclick: e => { e.preventDefault(); setAll(false); } }, t('live.selectNone'))),
        ps.map(p => el('label.param-item',
          el('input', { type: 'checkbox', checked: selected.has(p.id), onchange: e => { e.target.checked ? selected.add(p.id) : selected.delete(p.id); paintContent(); } }),
          el('span', tr(p.name), p.unit ? el('small', ' ' + unitName(p.unit)) : null,
            p.verified ? null : el('span.unverified', { title: tr(p.note) || t('common.unverified') }, ' ?'))))));
    }
    if (params.some(p => !p.verified)) listBox.append(el('p.muted.small', t('live.unverifiedLegend')));
  }
  filter.oninput = paintList;

  function paintTabs() {
    clear(tabs);
    const tab = (id, label) => el('button.small' + (mode === id ? '.primary' : ''), { onclick: () => { mode = id; paintTabs(); paintContent(); } }, label);
    tabs.append(tab('table', t('live.table')), tab('charts', t('live.charts')));
    if (hasCap('blocks')) tabs.append(tab('raw', t('live.raw')));
  }

  const rows = {};
  function paintContent() {
    clearInterval(rawTimer);
    clear(content);
    charts = {};
    if (mode === 'raw') { rawView(); return; }
    const sel = params.filter(p => selected.has(p.id));
    if (!sel.length) { content.append(el('div.card.empty', t('live.chooseParams'))); return; }
    if (mode === 'table') {
      const tbody = el('tbody');
      for (const p of sel) {
        const v = el('td.num'), mn = el('td.num.muted'), mx = el('td.num.muted'), u = el('td.muted');
        rows[p.id] = { p, v, mn, mx, u };
        tbody.append(el('tr', el('td', tr(p.name), p.verified ? null : el('span.unverified', { title: tr(p.note) || t('common.unverified') }, ' ?')), v, u, mn, mx));
      }
      content.append(el('div.card.table-wrap', el('table',
        el('thead', el('tr', el('th', t('common.name')), el('th', t('common.value')), el('th', t('common.unit')), el('th', t('common.min')), el('th', t('common.max')))),
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
    for (const [id, r] of Object.entries(rows)) {
      const v = store.live[id];
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
        el('button', { onclick: () => readRaw() }, t('common.read'))), info, grid));
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
        info.textContent = e.message;
      }
    }
    rawTimer = setInterval(() => { if (auto.checked && content.isConnected) readRaw(); }, 700);
  }

  async function load() {
    try {
      const r = await api.get(ecuPath('params'));
      params = r.params || [];
      selected = new Set(r.selected && r.selected.length ? r.selected : keyParams(params).map(p => p.id));
    } catch (e) {
      params = [];
      toast(e.message, 'bad');
    }
    paintList();
    paintTabs();
    paintContent();
    paintControls();
  }

  load();
  unsubs.push(subscribe('live', paintValues));
  unsubs.push(subscribe('status', paintControls));
  return () => { clearInterval(rawTimer); unsubs.forEach(u => u()); };
}
