import { api } from '../api.js';
import { t, tr, unitName } from '../i18n.js';
import { el, clear, toast, busy, confirmBox, fmt, pageHeader, errText, icon } from '../ui.js';
import { currentECU } from '../app.js';
import { ecuSelect, ecuPath, hasCap } from './common.js';

let confirmed = false;
let confirming = null;

// confirmOnce asks once per program run whether output tests may be
// used; requests made while the question is open share its answer.
function confirmOnce() {
  if (confirmed) return Promise.resolve(true);
  if (!confirming) {
    confirming = confirmBox(t('svc.title'), t('tests.confirm'), t('common.yes'), true).then(ok => {
      confirmed = ok;
      confirming = null;
      return ok;
    });
  }
  return confirming;
}

function css(name) {
  return getComputedStyle(document.documentElement).getPropertyValue(name).trim();
}

const chartWindow = 60000; // ms shown on an adjustment's chart

// The service page: output tests, idle adjustments and the control unit
// restart, with the values that show their effect next to them.
export function render(view) {
  const liveBox = el('div.grid.svc-live');
  const resetBox = el('div');
  const actsBox = el('div');
  const held = {}; // output id -> time it returns to the control unit (ms)
  let params = [];
  let watch = [];
  let tiles = {};
  let cards = {};
  let acts = [];
  let alive = true;
  let timer = 0;
  let syncAt = 0;

  view.append(
    pageHeader(t('svc.title'), t('svc.lead')),
    ecuSelect(() => load()),
    el('div.card.safety',
      el('div.safety-head', icon('alert'), t('svc.safetyTitle')),
      el('ul', ['svc.safety1', 'svc.safety2', 'svc.safety3'].map(k => el('li', t(k))))),
    liveBox, resetBox, actsBox);

  // Values shown on the page: engine speed, coolant, voltage and the
  // idle control values, whatever the unit calls them.
  function pickWatch() {
    const byOBD = o => params.find(p => p.obd === o && p.kind === 'num');
    const out = [byOBD('0C'), byOBD('05'), byOBD('42')];
    for (const p of params) if (p.group === 'idle' && p.kind === 'num' && out.length < 6) out.push(p);
    return out.filter(Boolean);
  }

  function paintLive() {
    clear(liveBox);
    tiles = {};
    for (const p of watch) {
      const val = el('span.value', '—');
      liveBox.append(el('div.card.tile.mini', el('span.label', tr(p.name)), el('div', val, el('span.unit', unitName(p.unit)))));
      tiles[p.id] = { p, val, v: null };
    }
  }

  // measured returns the value an adjustment acts on: the engine speed
  // for an idle speed, the current position for an idle air valve.
  function measured(a) {
    const p = a.unit === 'rpm' ? watch.find(x => x.obd === '0C')
      : watch.find(x => x.unit === a.unit && /current|текущ/i.test(tr(x.name))) || watch.find(x => x.unit === a.unit);
    return p && tiles[p.id] && tiles[p.id].v !== null ? { p, v: tiles[p.id].v } : null;
  }

  async function poll() {
    if (!alive) return;
    if (watch.length) {
      try {
        const r = await api.post(ecuPath('read'), { ids: watch.map(p => p.id) });
        for (const v of r.values || []) {
          const tile = tiles[v.id];
          if (!tile || !v.ok) continue;
          tile.v = v.v;
          tile.val.textContent = v.t ? v.t : fmt(v.v, tile.p.digits);
        }
      } catch (e) { /* the next action reports problems */ }
    }
    if (Date.now() > syncAt) {
      syncAt = Date.now() + 5000;
      try { mergeActive((await api.get(ecuPath('actuators'))).actuators || []); } catch (e) { /* ignore */ }
    }
    const now = Date.now();
    for (const c of Object.values(cards)) {
      paintState(c.a.id);
      if (c.canvas) record(c, now);
    }
    // While an adjustment is held, follow the engine more closely.
    if (alive) timer = setTimeout(poll, Object.keys(held).length ? 400 : 800);
  }

  // mergeActive follows the server: outputs it handed back on time or
  // that a restart dropped are no longer shown as held.
  function mergeActive(list) {
    for (const a of list) {
      if (!a.active) delete held[a.id];
      else if (!held[a.id]) held[a.id] = Date.now() + (a.timeoutS || 60) * 1000;
    }
  }

  function paintReset() {
    clear(resetBox);
    if (!hasCap('reset')) return;
    const b = el('button', { type: 'button' }, icon('refresh'), t('svc.reset'));
    b.onclick = async () => {
      if (!await confirmBox(t('svc.resetTitle'), t('svc.resetConfirm'), t('svc.reset'), false)) return;
      await busy(b, async () => {
        const t0 = performance.now();
        await api.post(ecuPath('reset'));
        for (const k of Object.keys(held)) delete held[k];
        toast(`${t('svc.resetDone')} (${((performance.now() - t0) / 1000).toFixed(1)} ${t('chart.sec')})`, 'good');
      });
    };
    resetBox.append(el('div.card.svc-reset',
      el('div', el('h2', t('svc.resetTitle')), el('p.secondary', t('svc.resetLead'))), b));
  }

  async function act(a, action, value, btn) {
    if (!await confirmOnce()) return;
    await busy(btn, async () => {
      await api.post(ecuPath('actuate'), { id: a.id, action, value });
      if (action === 'on' || action === 'set') held[a.id] = Date.now() + (a.timeoutS || 60) * 1000;
      if (action === 'off') delete held[a.id];
      toast(`${tr(a.name)}: ${t('svc.done.' + action)}`, 'good');
      paintState(a.id);
    });
  }

  function paintState(id) {
    const c = cards[id];
    if (!c) return;
    const until = held[id];
    c.card.classList.toggle('on', !!until);
    if (until) {
      const left = Math.max(0, Math.round((until - Date.now()) / 1000));
      if (left === 0) delete held[id];
      c.state.textContent = `● ${t('svc.held')} · ${t('svc.returns')} ${left} ${t('chart.sec')}`;
    } else {
      c.state.textContent = '';
      c.target = null;
    }
    if (c.now) {
      const m = measured(c.a);
      const u = ' ' + unitName(c.a.unit);
      c.now.textContent = [
        c.target !== null ? t('svc.target') + ': ' + fmt(c.target, 0) + u : '',
        m ? t('svc.now') + ': ' + fmt(m.v, m.p.digits) + ' ' + unitName(m.p.unit) : '',
      ].filter(Boolean).join(' · ');
    }
  }

  // valueControls builds an adjustment that follows the hand in real
  // time: a slider, steps and a number, applied at once when asked, with
  // the target and the measured value on a chart.
  function valueControls(a, c) {
    // A zero minimum is left out of the server's answer.
    const lo = a.min || 0, hi = a.max || 255;
    const step = a.unit === 'rpm' ? 10 : 1;
    const clamp = v => Math.min(hi, Math.max(lo, Math.round(v / step) * step));
    const start = clamp((lo + hi) / 2);
    const num = el('input', { type: 'number', min: lo, max: hi, step, value: start, style: { width: '96px' } });
    const range = el('input.svc-range', { type: 'range', min: lo, max: hi, step, value: start });
    const live = el('input', { type: 'checkbox' });
    const show = v => { num.value = v; range.value = v; };
    c.show = v => { if (!c.touched) show(clamp(v)); };

    // apply sends the newest value; values given while a request is on
    // its way replace each other, so the unit is never flooded.
    c.apply = async (v, btn) => {
      c.touched = true;
      if (!await confirmOnce()) return;
      c.pending = v;
      if (c.sending) return;
      c.sending = true;
      if (btn) btn.disabled = true;
      try {
        while (c.pending !== null) {
          const x = c.pending;
          c.pending = null;
          await api.post(ecuPath('actuate'), { id: a.id, action: 'set', value: x });
          c.target = x;
          held[a.id] = Date.now() + (a.timeoutS || 60) * 1000;
          paintState(a.id);
        }
      } catch (e) {
        c.pending = null;
        toast(`${tr(a.name)}: ${errText(e)}`, 'bad');
      } finally {
        c.sending = false;
        if (btn) btn.disabled = false;
      }
    };

    range.oninput = () => { c.touched = true; num.value = range.value; if (live.checked) c.apply(+range.value); };
    num.onchange = () => { c.touched = true; const v = clamp(+num.value); show(v); if (live.checked) c.apply(v); };
    const nudges = (a.unit === 'rpm' ? [-50, -10, 10, 50] : [-5, -1, 1, 5]).map(d => el('button.small.nudge', {
      onclick: () => { const v = clamp(+num.value + d); show(v); c.apply(v); },
    }, (d > 0 ? '+' : '−') + Math.abs(d)));
    const set = el('button.primary', t('tests.set'));
    const rel = el('button', t('tests.release'));
    set.onclick = () => c.apply(clamp(+num.value), set);
    rel.onclick = () => act(a, 'off', 0, rel).then(() => { c.target = null; c.touched = false; });

    c.canvas = el('canvas.svc-chart', { role: 'img', 'aria-label': tr(a.name) });
    c.hist = [];
    return [
      el('div.row.svc-controls', range, num, el('span.muted', unitName(a.unit))),
      el('div.row.svc-controls', nudges, el('label.check.small', live, t('svc.liveApply'))),
      el('div.row.svc-controls', set, rel),
      c.canvas,
      el('div.svc-legend.small', el('span.k.actual', t('svc.actual')), el('span.k.target', t('svc.target'))),
    ];
  }

  // record adds the measured value and the target to an adjustment's
  // chart and redraws it.
  function record(c, now) {
    const m = measured(c.a);
    if (m && !c.touched) c.show(m.v);
    c.hist.push([now, m ? m.v : null, held[c.a.id] ? c.target : null]);
    while (c.hist.length && c.hist[0][0] < now - chartWindow - 2000) c.hist.shift();
    drawChart(c);
  }

  // drawChart plots the measured value (solid) and the target (dashed)
  // on one axis: both are in the same unit.
  function drawChart(c) {
    const cv = c.canvas;
    const w = cv.clientWidth, h = cv.clientHeight;
    if (!w || !h) return;
    const dpr = window.devicePixelRatio || 1;
    cv.width = Math.round(w * dpr);
    cv.height = Math.round(h * dpr);
    const g = cv.getContext('2d');
    g.setTransform(dpr, 0, 0, dpr, 0, 0);
    g.clearRect(0, 0, w, h);
    const pts = c.hist;
    const vals = pts.flatMap(p => [p[1], p[2]]).filter(v => v !== null);
    if (pts.length < 2 || !vals.length) return;
    let lo = Math.min(...vals), hi = Math.max(...vals);
    const pad = Math.max((hi - lo) * 0.15, c.a.unit === 'rpm' ? 50 : 2);
    lo -= pad; hi += pad;
    const t1 = pts[pts.length - 1][0], t0 = t1 - chartWindow;
    const x = tm => 34 + (tm - t0) / chartWindow * (w - 38);
    const y = v => 6 + (h - 12) * (1 - (v - lo) / (hi - lo));
    g.font = '11px system-ui, sans-serif';
    g.fillStyle = css('--text-muted');
    g.textBaseline = 'middle';
    g.fillText(fmt(hi, 0), 2, y(hi));
    g.fillText(fmt(lo, 0), 2, y(lo));
    g.strokeStyle = css('--border');
    g.lineWidth = 1;
    g.beginPath(); g.moveTo(34, y(hi)); g.lineTo(w - 4, y(hi)); g.moveTo(34, y(lo)); g.lineTo(w - 4, y(lo)); g.stroke();
    const line = (pick, color, dash) => {
      g.strokeStyle = color;
      g.lineWidth = 2;
      g.setLineDash(dash);
      g.beginPath();
      let on = false;
      for (const p of pts) {
        const v = pick(p);
        if (v === null || p[0] < t0) { on = false; continue; }
        if (on) g.lineTo(x(p[0]), y(v)); else g.moveTo(x(p[0]), y(v));
        on = true;
      }
      g.stroke();
      g.setLineDash([]);
    };
    line(p => p[2], css('--text-muted'), [6, 4]);
    line(p => p[1], css('--series-1'), []);
  }

  function actCard(a) {
    const c = { a, target: null, pending: null, sending: false, touched: false };
    const state = el('div.svc-state.small');
    let body;
    if (a.kind === 'toggle') {
      const on = el('button.primary', t('tests.on'));
      const off = el('button', t('tests.off'));
      on.onclick = () => act(a, 'on', 0, on);
      off.onclick = () => act(a, 'off', 0, off);
      body = el('div.row.svc-controls', on, off);
    } else if (a.kind === 'pulse') {
      const p = el('button.primary', '⚡ ' + t('tests.pulse'));
      p.onclick = () => act(a, 'pulse', 0, p);
      body = el('div.row.svc-controls', p);
    } else {
      c.now = el('div.svc-now.small');
      body = [c.now, valueControls(a, c)];
    }
    const cond = a.condition
      ? el('span.badge' + (a.condition === 'engine_off' ? '.info' : '.warn'), a.condition === 'engine_off' ? t('svc.condOff') : t('svc.condRun'))
      : null;
    c.card = el('div.card.svc-act' + (a.kind === 'value' ? '.svc-value' : ''),
      el('div.svc-head', el('h3', tr(a.name)), cond),
      a.warning ? el('p.small.secondary', tr(a.warning)) : null,
      body, state);
    c.state = state;
    cards[a.id] = c;
    return c.card;
  }

  function paintActs() {
    clear(actsBox);
    cards = {};
    if (!acts.length) {
      const e = currentECU();
      const legacy = e && /legacy|микас/i.test(e.desc.protocol + ' ' + e.desc.name);
      actsBox.append(el('div.card.empty', el('p', t('svc.none')), legacy ? el('p.muted.small', t('svc.noneLegacy')) : null));
      return;
    }
    const groups = [
      ['value', 'svc.gValue', 'svc.gValueLead'],
      ['pulse', 'svc.gPulse', 'svc.gPulseLead'],
      ['toggle', 'svc.gToggle', 'svc.gToggleLead'],
    ];
    for (const [kind, title, lead] of groups) {
      const list = acts.filter(a => a.kind === kind);
      if (!list.length) continue;
      actsBox.append(el('div.svc-group', el('h2', t(title)), el('p.secondary.small', t(lead)),
        el('div.grid.svc-grid' + (kind === 'value' ? '.wide' : ''), list.map(actCard))));
    }
    for (const id of Object.keys(cards)) paintState(id);
  }

  async function load() {
    clear(actsBox).append(el('p.muted', '…'));
    try {
      params = (await api.get(ecuPath('params'))).params || [];
    } catch (e) {
      params = [];
    }
    watch = pickWatch();
    paintLive();
    paintReset();
    try {
      acts = (await api.get(ecuPath('actuators'))).actuators || [];
    } catch (e) {
      acts = [];
    }
    mergeActive(acts);
    paintActs();
  }

  load().then(() => { if (alive) poll(); });
  return () => { alive = false; clearTimeout(timer); };
}
