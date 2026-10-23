import { api } from '../api.js';
import { t, tr, unitName, fill } from '../i18n.js';
import { el, clear, toast, busy, fmt, convert, pageHeader, errText } from '../ui.js';
import { store, subscribe, currentECU, saveSettings } from '../app.js';
import { ecuSelect, keyParams, limitsFor, groupName } from './common.js';

// The gauges page: dials for chosen values, a trip computer and
// acceleration timing, all computed from the live data the program polls.

const NS = 'http://www.w3.org/2000/svg';
const A0 = 150 * Math.PI / 180;   // a dial starts bottom left…
const SPAN = 240 * Math.PI / 180; // …and sweeps 240° clockwise
const CX = 100, CY = 100, R = 76;
const maxGauges = 12;

function svg(tag, attrs, ...children) {
  const n = document.createElementNS(NS, tag);
  for (const [k, v] of Object.entries(attrs || {})) n.setAttribute(k, v);
  for (const c of children) n.append(c);
  return n;
}

function polar(a, r) { return [CX + r * Math.cos(a), CY + r * Math.sin(a)]; }

function arc(a0, a1, r) {
  const [x0, y0] = polar(a0, r), [x1, y1] = polar(a1, r);
  return `M${x0.toFixed(2)} ${y0.toFixed(2)} A${r} ${r} 0 ${a1 - a0 > Math.PI ? 1 : 0} 1 ${x1.toFixed(2)} ${y1.toFixed(2)}`;
}

// ticks gives round values (1, 2, 2.5 or 5 × 10^k apart) between lo and hi.
function ticks(lo, hi, n = 5) {
  const raw = (hi - lo) / n;
  const mag = Math.pow(10, Math.floor(Math.log10(raw)));
  const step = [1, 2, 2.5, 5, 10].map(m => m * mag).find(s => s >= raw * 0.999) || raw;
  const out = [];
  for (let v = Math.ceil(lo / step - 1e-9) * step; v <= hi + step * 1e-6; v += step) out.push(+v.toFixed(10));
  return out;
}

// Dial ranges for well-known values; the description's range is often
// the protocol's (an engine speed up to 16383 rpm).
const dialRange = { '0C': [0, 8000], '0D': [0, 220], '05': [-40, 130], '0F': [-40, 80], '11': [0, 100], '04': [0, 100], '0E': [-20, 60] };

function rangeOf(p) {
  if (dialRange[p.obd]) return dialRange[p.obd];
  if (p.obd === '42') return p.max > 20 ? [16, 32] : [8, 16];
  const lo = p.min || 0, hi = p.max || 0;
  return hi > lo ? [lo, hi] : [lo, lo + 100];
}

// Gauge is one dial: the value as an arc and a needle, warning and danger
// zones on the rim, round ticks and the digits.
class Gauge {
  constructor(p, units) {
    this.p = p;
    this.conv = v => convert(v, p.unit, units).v;
    const [a, b] = rangeOf(p).map(this.conv);
    this.lo = Math.min(a, b);
    this.hi = Math.max(a, b);
    this.unit = unitName(convert(0, p.unit, units).unit);
    const lim = limitsFor(p);
    const c = v => (v === undefined ? undefined : this.conv(v));
    this.lim = { warn: c(lim.warn), bad: c(lim.bad), warnBelow: c(lim.warnBelow), badBelow: c(lim.badBelow) };

    const thousands = this.hi >= 2000;
    const g = svg('svg', { viewBox: '0 0 200 172', class: 'dial', role: 'img', 'aria-label': tr(p.name) });
    g.append(svg('path', { d: arc(A0, A0 + SPAN, R), class: 'g-track' }));
    for (const [from, to, cls] of this.zones()) {
      g.append(svg('path', { d: arc(this.angle(from), this.angle(to), R + 12), class: 'g-zone ' + cls }));
    }
    for (const v of ticks(this.lo, this.hi)) {
      const a = this.angle(v);
      const [x0, y0] = polar(a, R - 13), [x1, y1] = polar(a, R - 7), [tx, ty] = polar(a, R - 24);
      g.append(svg('line', { x1: x0, y1: y0, x2: x1, y2: y1, class: 'g-tick' }));
      const label = thousands ? fmt(v / 1000, 1) : fmt(v, Math.abs(this.hi - this.lo) < 20 ? 1 : 0);
      const tl = svg('text', { x: tx, y: ty, class: 'g-tl', 'text-anchor': 'middle', 'dominant-baseline': 'middle' });
      tl.textContent = label;
      g.append(tl);
    }
    this.valArc = svg('path', { d: arc(A0, A0 + SPAN, R), class: 'g-val', pathLength: 100 });
    g.append(this.valArc);
    this.needle = svg('g', { class: 'g-needle' },
      svg('line', { x1: CX, y1: CY, x2: polar(A0, R - 16)[0], y2: polar(A0, R - 16)[1] }));
    g.append(this.needle, svg('circle', { cx: CX, cy: CY, r: 6, class: 'g-hub' }));
    this.num = svg('text', { x: CX, y: 146, class: 'g-num', 'text-anchor': 'middle' });
    this.num.textContent = '—';
    const unit = svg('text', { x: CX, y: 164, class: 'g-unit', 'text-anchor': 'middle' });
    unit.textContent = (thousands ? '×1000 ' : '') + this.unit;
    g.append(this.num, unit);

    this.label = el('span', tr(p.name));
    this.mark = el('span.g-mark');
    this.range = el('div.g-range.small.muted', '');
    this.root = el('div.card.gauge', g, el('div.g-label', this.mark, this.label), this.range);
    this.set(null);
  }

  angle(v) {
    const f = Math.max(0, Math.min(1, (v - this.lo) / (this.hi - this.lo || 1)));
    return A0 + f * SPAN;
  }

  zones() {
    const l = this.lim, z = [];
    if (l.bad !== undefined) z.push([l.bad, this.hi, 'bad']);
    if (l.warn !== undefined) z.push([l.warn, l.bad !== undefined ? l.bad : this.hi, 'warn']);
    if (l.badBelow !== undefined) z.push([this.lo, l.badBelow, 'bad']);
    if (l.warnBelow !== undefined) z.push([l.badBelow !== undefined ? l.badBelow : this.lo, l.warnBelow, 'warn']);
    return z.filter(([a, b]) => b > a && b > this.lo && a < this.hi)
      .map(([a, b, cls]) => [Math.max(a, this.lo), Math.min(b, this.hi), cls]);
  }

  level(v) {
    const l = this.lim;
    if ((l.bad !== undefined && v >= l.bad) || (l.badBelow !== undefined && v <= l.badBelow)) return 'bad';
    if ((l.warn !== undefined && v >= l.warn) || (l.warnBelow !== undefined && v <= l.warnBelow)) return 'warn';
    return '';
  }

  // set shows a value in the description's unit (null: no value).
  set(raw, entry) {
    if (raw === null || raw === undefined) {
      this.num.textContent = '—';
      this.valArc.style.strokeDashoffset = 100;
      this.needle.style.transform = 'rotate(0rad)';
      return;
    }
    const v = this.conv(raw);
    const f = Math.max(0, Math.min(1, (v - this.lo) / (this.hi - this.lo || 1)));
    this.num.textContent = fmt(v, this.p.digits ?? (Math.abs(v) < 10 ? 1 : 0));
    this.valArc.style.strokeDasharray = '100 100';
    this.valArc.style.strokeDashoffset = 100 - f * 100;
    this.needle.style.transform = `rotate(${(f * SPAN).toFixed(4)}rad)`;
    const lvl = this.level(v);
    this.root.classList.toggle('warn', lvl === 'warn');
    this.root.classList.toggle('bad', lvl === 'bad');
    this.mark.textContent = lvl ? '⚠ ' : '';
    if (entry && entry.min !== undefined) {
      this.range.textContent = `${t('common.min')} ${fmt(this.conv(entry.min), this.p.digits)} · ${t('common.max')} ${fmt(this.conv(entry.max), this.p.digits)}`;
    }
  }
}

// ---- Trip computer -------------------------------------------------------

// The trip computer keeps counting from the first visit of the page until
// the program is closed, on whatever live data arrives.
const trip = { last: 0, dist: 0, fuel: 0, time: 0, moving: 0, vmax: 0, est: false, speed: null, rate: null };
let tripSrc = null; // {ecu, speed, rate, maf} chosen from the unit's values
let tripWatching = false;

// mafToRate estimates fuel use (l/h) of a petrol engine at λ = 1 from the
// air mass flow: 14.7 kg of air per kg of fuel, 0.74 kg per litre.
function mafToRate(v, unit) {
  const gs = unit === 'kg/h' ? v / 3.6 : v;
  return gs * 3600 / 14.7 / 740;
}

function tripSources(ecuId, params) {
  const by = o => params.find(p => p.obd === o && p.kind === 'num');
  const speed = by('0D') || params.find(p => p.id === 'j1939.spn84');
  const rate = by('5E') || params.find(p => p.id === 'j1939.spn183');
  const maf = by('10');
  return { ecu: ecuId, speed, rate: rate && rate.unit === 'l/h' ? rate : null, maf };
}

function tripTick(ev) {
  if (!tripSrc || !ev || !ev.values || (ev.ecu && ev.ecu !== tripSrc.ecu)) return;
  const tm = ev.time ? Date.parse(ev.time) : Date.now();
  const val = p => {
    if (!p) return null;
    const v = ev.values.find(x => x.id === p.id);
    return v && v.ok && !v.t ? v.v : null;
  };
  const speed = val(tripSrc.speed);
  let rate = val(tripSrc.rate), est = false;
  if (rate === null && tripSrc.maf) {
    const m = val(tripSrc.maf);
    if (m !== null) { rate = mafToRate(m, tripSrc.maf.unit); est = true; }
  }
  if (speed === null && rate === null) return;
  if (trip.last && tm > trip.last) {
    const dh = (tm - trip.last) / 3600000;
    if (dh < 5 / 3600) { // a pause longer than 5 s is not counted
      trip.time += dh;
      if (speed !== null) {
        trip.dist += speed * dh;
        if (speed > 1) trip.moving += dh;
        trip.vmax = Math.max(trip.vmax, speed);
      }
      if (rate !== null) {
        trip.fuel += rate * dh;
        trip.est = trip.est || est;
      }
    }
  }
  trip.last = tm;
  trip.speed = speed;
  trip.rate = rate;
}

function watchTrip() {
  if (tripWatching) return;
  tripWatching = true;
  subscribe('live', tripTick);
}

function resetTrip() {
  Object.assign(trip, { last: 0, dist: 0, fuel: 0, time: 0, moving: 0, vmax: 0, est: false });
}

function hms(hours) {
  const s = Math.round(hours * 3600);
  return [Math.floor(s / 3600), Math.floor(s / 60) % 60, s % 60].map((x, i) => (i ? String(x).padStart(2, '0') : x)).join(':');
}

// ---- Acceleration timing -------------------------------------------------

const presetsMetric = [
  { id: '0-60', from: 0, to: 60 }, { id: '0-100', from: 0, to: 100 }, { id: '60-100', from: 60, to: 100 },
  { id: '80-120', from: 80, to: 120 }, { id: '402m', from: 0, dist: 402.336, label: '402 m' },
];
const presetsImperial = [
  { id: '0-30mph', from: 0, to: 30 / 0.621371, label: '0–30 mph' }, { id: '0-60mph', from: 0, to: 60 / 0.621371, label: '0–60 mph' },
  { id: '30-70mph', from: 30 / 0.621371, to: 70 / 0.621371, label: '30–70 mph' }, { id: '402m', from: 0, dist: 402.336, label: '¼ mile' },
];

function presetLabel(p) {
  return p.label || `${p.from}–${p.to} ${unitName('km/h')}`;
}

// resultLabel names a stored run in the current language.
function resultLabel(r) {
  const p = [...presetsMetric, ...presetsImperial].find(x => x.id === r.preset);
  return p ? presetLabel(p) : r.label;
}

function loadResults() {
  try { return JSON.parse(localStorage.getItem('udt-accel') || '[]'); } catch (e) { return []; }
}

function saveResults(list) {
  try { localStorage.setItem('udt-accel', JSON.stringify(list.slice(0, 20))); } catch (e) { /* private mode */ }
}

// crossing returns the moment a value passed level between two samples,
// assuming it changed evenly in between.
function crossing(a, b, level) {
  if (b.v === a.v) return b.t;
  return a.t + (level - a.v) / (b.v - a.v) * (b.t - a.t);
}

// ---- Page ----------------------------------------------------------------

export function render(view) {
  const units = store.settings.units;
  const grid = el('div.gauges');
  const pickBox = el('div');
  const tripBox = el('div');
  const accelBox = el('div');
  let params = [];
  let gauges = {};
  let selection = [];
  let key = '';
  let alive = true;
  const unsubs = [];

  const fsBtn = el('button.small', { onclick: () => fullscreen(grid) }, '⛶ ' + t('gauges.fullscreen'));
  const hudBtn = el('button.small', { onclick: () => openHUD() }, t('gauges.hud'));
  const editBtn = el('button.small', { onclick: () => togglePicker() }, '⚙ ' + t('gauges.edit'));
  view.append(
    pageHeader(t('gauges.title'), t('gauges.lead'), editBtn, hudBtn, fsBtn),
    ecuSelect(() => load()),
    pickBox, grid,
    el('div.grid.gauges-tools', tripBox, accelBox));

  function settingsKey() {
    const e = currentECU();
    return e ? (e.desc.definition || e.desc.id || e.id) : '';
  }

  function numeric() {
    return params.filter(p => p.kind === 'num');
  }

  function defaultSelection() {
    return keyParams(params).map(p => p.id).slice(0, 8);
  }

  function buildGauges() {
    clear(grid);
    gauges = {};
    const list = selection.map(id => params.find(p => p.id === id)).filter(Boolean);
    if (!list.length) {
      grid.append(el('p.muted', t('live.noParams')));
      return;
    }
    for (const p of list) {
      const g = new Gauge(p, units);
      gauges[p.id] = g;
      grid.append(g.root);
    }
    paint();
  }

  function paint() {
    for (const [id, g] of Object.entries(gauges)) {
      const v = store.live[id];
      g.set(v && v.ok && !v.t ? v.v : null, v);
    }
    paintTrip();
  }

  // ensurePolling adds the values this page needs to the polled set,
  // keeping what other pages already poll.
  async function ensurePolling(extra = []) {
    const e = currentECU();
    if (!e) return;
    const running = store.status.live;
    const have = running && running.ecu === e.id ? (running.params || []) : [];
    const src = tripSrc && tripSrc.ecu === e.id ? [tripSrc.speed, tripSrc.rate, tripSrc.maf].filter(Boolean).map(p => p.id) : [];
    const need = [...new Set([...selection, ...src, ...extra])];
    const missing = need.filter(id => !have.includes(id));
    if (!missing.length) return;
    try {
      await api.post('/api/live/start', { ecu: e.id, params: [...have, ...missing], intervalMs: (running && running.intervalMs) || 250 });
    } catch (err) {
      toast(errText(err), 'bad');
    }
  }

  // ---- picker ----
  let picking = false;
  function togglePicker() {
    picking = !picking;
    editBtn.classList.toggle('active', picking);
    clear(pickBox);
    if (!picking) return;
    const chosen = new Set(selection);
    const count = el('span.muted.small');
    const update = () => { count.textContent = fill(t('gauges.chosen'), { n: chosen.size, max: maxGauges }); };
    const groups = {};
    for (const p of numeric()) (groups[p.group || 'other'] = groups[p.group || 'other'] || []).push(p);
    const lists = Object.entries(groups).map(([gname, ps]) => el('div.pick-group',
      el('div.pick-head', groupName(gname)),
      ps.map(p => el('label.check', el('input', {
        type: 'checkbox', checked: chosen.has(p.id),
        onchange: ev => {
          if (ev.target.checked && chosen.size >= maxGauges) { ev.target.checked = false; toast(fill(t('gauges.tooMany'), { max: maxGauges }), 'bad'); return; }
          if (ev.target.checked) chosen.add(p.id); else chosen.delete(p.id);
          update();
        },
      }), tr(p.name), p.unit ? el('span.muted.small', ' ' + unitName(p.unit)) : null))));
    const save = el('button.primary', t('common.save'));
    save.onclick = () => busy(save, async () => {
      // Keep the order of the list, so the dials appear as in it.
      selection = numeric().map(p => p.id).filter(id => chosen.has(id));
      store.settings.gauges = { ...(store.settings.gauges || {}), [key]: selection };
      await saveSettings({ gauges: store.settings.gauges });
      togglePicker();
      buildGauges();
      await ensurePolling();
    });
    const reset = el('button', { onclick: () => { chosen.clear(); defaultSelection().forEach(id => chosen.add(id)); togglePicker(); togglePicker(); } }, t('gauges.defaults'));
    update();
    pickBox.append(el('div.card.picker', el('div.row.picker-head', el('h2', t('gauges.edit')), count, el('span.grow'), reset, save), el('div.pick-groups', lists)));
  }

  // ---- trip computer ----
  function paintTrip() {
    clear(tripBox);
    if (!tripSrc || (!tripSrc.speed && !tripSrc.rate && !tripSrc.maf)) {
      tripBox.append(el('div.card', el('h2', t('trip.title')), el('p.muted', t('trip.unavailable'))));
      return;
    }
    const d = convert(trip.dist, 'km', units), vmax = convert(trip.vmax, 'km/h', units);
    const avgSpeed = trip.moving > 0 ? convert(trip.dist / trip.moving, 'km/h', units) : null;
    const avg = trip.dist > 0.2 && trip.fuel > 0 ? convert(trip.fuel / trip.dist * 100, 'l/100km', units) : null;
    let inst = null;
    if (trip.rate !== null && trip.rate !== undefined) {
      inst = trip.speed !== null && trip.speed > 5 ? convert(trip.rate / trip.speed * 100, 'l/100km', units) : convert(trip.rate, 'l/h', units);
    }
    const fuel = units === 'imperial' ? { v: trip.fuel * 0.264172, unit: 'gal' } : { v: trip.fuel, unit: 'l' };
    const cell = (label, v, unit, digits = 1) => el('div.trip-cell', el('span.label', label),
      el('div', el('span.value', v === null || v === undefined ? '—' : fmt(v, digits)), ' ', el('span.unit', unitName(unit))));
    const cells = [
      inst ? cell(t('trip.now'), inst.v, inst.unit) : cell(t('trip.now'), null, ''),
      cell(t('trip.avg'), avg && avg.v, avg ? avg.unit : (units === 'imperial' ? 'mpg' : 'l/100km')),
      cell(t('trip.fuel'), tripSrc.rate || tripSrc.maf ? fuel.v : null, fuel.unit, 2),
      cell(t('trip.dist'), tripSrc.speed ? d.v : null, d.unit, 2),
      el('div.trip-cell', el('span.label', t('trip.time')), el('div', el('span.value', hms(trip.time)))),
      cell(t('trip.avgSpeed'), avgSpeed && avgSpeed.v, avgSpeed ? avgSpeed.unit : 'km/h', 0),
      cell(t('trip.vmax'), tripSrc.speed ? vmax.v : null, vmax.unit, 0),
    ];
    tripBox.append(el('div.card.trip',
      el('div.row.trip-head', el('h2', t('trip.title')), el('span.grow'),
        el('button.small', { onclick: () => { resetTrip(); paintTrip(); } }, t('trip.reset'))),
      el('div.trip-grid', cells),
      el('p.muted.small', [
        !tripSrc.speed ? t('trip.noSpeed') : '',
        trip.est || (!tripSrc.rate && tripSrc.maf) ? t('trip.estimated') : '',
      ].filter(Boolean).join(' '))));
  }

  // ---- acceleration timing ----
  const presets = units === 'imperial' ? presetsImperial : presetsMetric;
  const run = { preset: presets[1], state: 'idle', prev: null, zero: null, t0: 0, dist: 0, hz: [], saved: null, results: loadResults() };

  function speedParam() {
    return tripSrc && tripSrc.speed;
  }

  async function arm() {
    const sp = speedParam();
    if (!sp) return;
    run.state = 'wait';
    run.prev = null; run.zero = null; run.dist = 0; run.hz = [];
    // Poll only the speed (and engine speed), as fast as the line allows.
    const e = currentECU();
    if (!store.recording && e) {
      run.saved = store.status.live ? { ...store.status.live } : null;
      const rpm = params.find(p => p.obd === '0C' && p.kind === 'num');
      try {
        await api.post('/api/live/start', { ecu: e.id, params: [sp.id, ...(rpm ? [rpm.id] : [])], intervalMs: 0 });
      } catch (err) { toast(errText(err), 'bad'); }
    }
    paintAccel();
  }

  async function disarm(state) {
    run.state = state;
    const saved = run.saved;
    run.saved = null;
    if (saved !== null && saved !== undefined) {
      try { await api.post('/api/live/start', saved); } catch (e) { /* the connection may be gone */ }
    } else if (!store.recording && state !== 'done-keep') {
      await ensurePolling();
    }
    paintAccel();
  }

  function accelTick(ev) {
    const sp = speedParam();
    if (!sp || !ev || !ev.values || !['wait', 'ready', 'run'].includes(run.state)) return;
    const v0 = ev.values.find(x => x.id === sp.id);
    if (!v0 || !v0.ok) return;
    const s = { t: ev.time ? Date.parse(ev.time) : Date.now(), v: v0.v };
    const p = run.preset;
    const prev = run.prev;
    run.prev = s;
    run.hz.push(s.t);
    if (run.hz.length > 20) run.hz.shift();
    if (run.state === 'wait') {
      if ((p.from === 0 && s.v <= 0.5) || (p.from > 0 && s.v < p.from - 3)) run.state = 'ready';
      if (p.from === 0 && s.v <= 0.5) run.zero = s;
    } else if (run.state === 'ready') {
      if (p.from === 0) {
        if (s.v <= 0.5) run.zero = s;
        else if (run.zero) { run.t0 = crossing(run.zero, s, 0.5); run.dist = (s.v / 3.6) * (s.t - run.t0) / 2000; run.state = 'run'; }
      } else if (prev && prev.v < p.from && s.v >= p.from) {
        run.t0 = crossing(prev, s, p.from);
        run.dist = 0;
        run.state = 'run';
      }
    } else if (run.state === 'run' && prev) {
      run.dist += (prev.v + s.v) / 2 / 3.6 * (s.t - prev.t) / 1000;
      const elapsed = (s.t - run.t0) / 1000;
      if (s.v < prev.v - 8 || (p.from > 0 && s.v < p.from - 2) || elapsed > 90) {
        disarm('aborted');
        return;
      }
      let finish = null;
      if (p.to && s.v >= p.to) finish = crossing(prev, s, p.to);
      if (p.dist && run.dist >= p.dist) {
        const segment = (prev.v + s.v) / 2 / 3.6 * (s.t - prev.t) / 1000;
        const before = run.dist - segment;
        finish = prev.t + (p.dist - before) / (segment || 1) * (s.t - prev.t);
      }
      if (finish !== null) {
        const res = { preset: p.id, label: presetLabel(p), time: (finish - run.t0) / 1000, trap: p.dist ? s.v : null, at: Date.now(), hz: rate() };
        run.results.unshift(res);
        saveResults(run.results);
        run.last = res;
        disarm('done');
        return;
      }
    }
    paintAccel();
  }

  // rate returns how often the speed arrived, which limits the accuracy.
  function rate() {
    const h = run.hz;
    return h.length > 2 ? (h.length - 1) / ((h[h.length - 1] - h[0]) / 1000) : 0;
  }

  function paintAccel() {
    clear(accelBox);
    const sp = speedParam();
    if (!sp) {
      accelBox.append(el('div.card', el('h2', t('accel.title')), el('p.muted', t('accel.unavailable'))));
      return;
    }
    const select = el('select', {
      disabled: ['wait', 'ready', 'run'].includes(run.state),
      onchange: e => { run.preset = presets.find(p => p.id === e.target.value) || presets[0]; paintAccel(); },
    }, presets.map(p => el('option', { value: p.id, selected: p === run.preset }, presetLabel(p))));
    const sv = run.prev ? convert(run.prev.v, 'km/h', units) : null;
    let status = '', big = '—';
    switch (run.state) {
      case 'wait': status = run.preset.from === 0 ? t('accel.stop') : fill(t('accel.below'), { v: presetLabel(run.preset).split('–')[0] }); break;
      case 'ready': status = t('accel.go'); big = '0.00'; break;
      case 'run': status = t('accel.running'); big = run.prev ? ((run.prev.t - run.t0) / 1000).toFixed(2) : '0.00'; break;
      case 'done': status = t('accel.done'); big = run.last ? run.last.time.toFixed(2) : '—'; break;
      case 'aborted': status = t('accel.aborted'); break;
      default: status = t('accel.idle');
    }
    const active = ['wait', 'ready', 'run'].includes(run.state);
    const btn = active
      ? el('button', { onclick: () => disarm('idle') }, t('accel.cancel'))
      : el('button.primary', { onclick: () => arm() }, t('accel.start'));
    const best = {};
    for (const r of run.results) if (!best[r.preset] || r.time < best[r.preset].time) best[r.preset] = r;
    accelBox.append(el('div.card.accel',
      el('div.row.trip-head', el('h2', t('accel.title')), el('span.grow'), select, btn),
      el('div.accel-main', el('div.accel-time' + (run.state === 'run' ? '.running' : ''), big, el('span.unit', ' ' + t('chart.sec'))),
        el('div.accel-side', el('div.accel-status', status),
          el('div.small.secondary', t('accel.speed') + ': ' + (sv ? fmt(sv.v, 0) + ' ' + unitName(sv.unit) : '—')),
          run.state === 'done' && run.last ? el('div.small.secondary',
            [run.last.trap ? t('accel.trap') + ' ' + fmt(convert(run.last.trap, 'km/h', units).v, 0) + ' ' + unitName(convert(0, 'km/h', units).unit) : '',
              run.last.hz ? fill(t('accel.rate'), { hz: fmt(run.last.hz, 1) }) : ''].filter(Boolean).join(' · ')) : null)),
      run.results.length ? el('table.small.accel-results', el('tbody', run.results.slice(0, 6).map(r => el('tr',
        el('td', resultLabel(r)),
        el('td.mono', r.time.toFixed(2) + ' ' + t('chart.sec') + (best[r.preset] === r ? ' ★' : '')),
        el('td.muted', new Date(r.at).toLocaleString()))))) : null,
      el('p.muted.small', t('accel.note'))));
  }

  // ---- HUD and full screen ----
  function fullscreen(node) {
    if (document.fullscreenElement) document.exitFullscreen();
    else if (node.requestFullscreen) node.requestFullscreen().catch(() => toast(t('gauges.noFullscreen'), 'bad'));
  }

  function openHUD() {
    const byObd = o => params.find(p => p.obd === o && p.kind === 'num');
    const main = (tripSrc && tripSrc.speed) || byObd('0C') || params.find(p => p.id === selection[0]);
    const second = main && main.obd === '0D' ? byObd('0C') : null;
    if (!main) return;
    const big = el('div.hud-big', '—'), bigUnit = el('div.hud-unit');
    const small = el('div.hud-small', '');
    const vals = el('div.hud-vals', big, bigUnit, second ? small : null);
    const close = () => { stop(); hud.remove(); document.removeEventListener('keydown', key); if (document.fullscreenElement) document.exitFullscreen(); };
    const key = e => { if (e.key === 'Escape') close(); };
    const hud = el('div.hud',
      vals,
      el('div.hud-bar',
        el('button', { onclick: () => vals.classList.toggle('mirror') }, t('gauges.mirror')),
        el('button', { onclick: () => fullscreen(hud) }, '⛶'),
        el('button', { onclick: close }, '✕ ' + t('common.close'))));
    const show = () => {
      const v = store.live[main.id];
      if (v && v.ok) {
        const c = convert(v.v, main.unit, units);
        big.textContent = fmt(c.v, 0);
        bigUnit.textContent = unitName(c.unit);
      }
      if (second) {
        const r = store.live[second.id];
        small.textContent = r && r.ok ? fmt(r.v, 0) + ' ' + unitName(second.unit) : '';
      }
    };
    const stop = subscribe('live', show);
    document.addEventListener('keydown', key);
    document.body.append(hud);
    show();
  }

  async function load() {
    clear(grid).append(el('p.muted', '…'));
    const e = currentECU();
    if (!e) return;
    try {
      params = (await api.get(`/api/ecu/${e.id}/params`)).params || [];
    } catch (err) {
      params = [];
    }
    if (!alive) return;
    key = settingsKey();
    const saved = (store.settings.gauges || {})[key];
    selection = (saved && saved.length ? saved : defaultSelection()).filter(id => params.some(p => p.id === id));
    tripSrc = tripSources(e.id, params);
    watchTrip();
    buildGauges();
    paintAccel();
    await ensurePolling();
  }

  unsubs.push(subscribe('live', ev => { paint(); accelTick(ev); }));
  load();
  return () => {
    alive = false;
    unsubs.forEach(u => u());
    if (['wait', 'ready', 'run'].includes(run.state)) disarm('idle');
  };
}
