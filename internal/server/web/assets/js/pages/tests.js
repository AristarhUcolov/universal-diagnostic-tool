import { api } from '../api.js';
import { t, tr, unitName } from '../i18n.js';
import { el, clear, toast, busy, confirmBox, fmt, pageHeader } from '../ui.js';
import { currentECU } from '../app.js';
import { ecuSelect, ecuPath, hasCap } from './common.js';

let confirmed = false;

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
      el('div.safety-head', '⚠ ', t('svc.safetyTitle')),
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

  // current returns the measured value an adjustment acts on.
  function current(a) {
    const p = a.unit === 'rpm' ? watch.find(x => x.obd === '0C')
      : watch.find(x => x.unit === a.unit && /current|текущ/i.test(tr(x.name))) || watch.find(x => x.unit === a.unit);
    return p && tiles[p.id] && tiles[p.id].v !== null ? fmt(tiles[p.id].v, p.digits) + ' ' + unitName(p.unit) : '';
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
    for (const id of Object.keys(cards)) paintState(id);
    if (alive) timer = setTimeout(poll, 800);
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
    const b = el('button', '⟳ ' + t('svc.reset'));
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
    if (!confirmed) {
      if (!await confirmBox(t('svc.title'), t('tests.confirm'), t('common.yes'), true)) return;
      confirmed = true;
    }
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
    }
    if (c.now) c.now.textContent = current(c.a) ? t('svc.now') + ': ' + current(c.a) : '';
  }

  function actCard(a) {
    const controls = el('div.row.svc-controls');
    const state = el('div.svc-state.small');
    let now = null;
    if (a.kind === 'toggle') {
      const on = el('button.primary', t('tests.on'));
      const off = el('button', t('tests.off'));
      on.onclick = () => act(a, 'on', 0, on);
      off.onclick = () => act(a, 'off', 0, off);
      controls.append(on, off);
    } else if (a.kind === 'pulse') {
      const p = el('button.primary', '⚡ ' + t('tests.pulse'));
      p.onclick = () => act(a, 'pulse', 0, p);
      controls.append(p);
    } else {
      // A zero minimum is left out of the server's answer.
      const lo = a.min || 0, hi = a.max || 255;
      const start = Math.round((lo + hi) / 2 / 10) * 10;
      const num = el('input', { type: 'number', min: lo, max: hi, value: start, style: { width: '100px' } });
      const range = el('input.svc-range', { type: 'range', min: lo, max: hi, step: a.unit === 'rpm' ? 10 : 1, value: start });
      range.oninput = () => { num.value = range.value; };
      num.oninput = () => { range.value = num.value; };
      const set = el('button.primary', t('tests.set'));
      const rel = el('button', t('tests.release'));
      set.onclick = () => act(a, 'set', +num.value, set);
      rel.onclick = () => act(a, 'off', 0, rel);
      now = el('span.muted.small');
      controls.append(range, num, el('span.muted', unitName(a.unit)), set, rel);
    }
    const cond = a.condition
      ? el('span.badge' + (a.condition === 'engine_off' ? '.info' : '.warn'), a.condition === 'engine_off' ? t('svc.condOff') : t('svc.condRun'))
      : null;
    const card = el('div.card.svc-act',
      el('div.svc-head', el('h3', tr(a.name)), cond),
      a.warning ? el('p.small.secondary', tr(a.warning)) : null,
      controls, now, state);
    cards[a.id] = { a, card, state, now };
    return card;
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
      ['pulse', 'svc.gPulse', 'svc.gPulseLead'],
      ['toggle', 'svc.gToggle', 'svc.gToggleLead'],
      ['value', 'svc.gValue', 'svc.gValueLead'],
    ];
    for (const [kind, title, lead] of groups) {
      const list = acts.filter(a => a.kind === kind);
      if (!list.length) continue;
      actsBox.append(el('div.svc-group', el('h2', t(title)), el('p.secondary.small', t(lead)),
        el('div.grid.svc-grid', list.map(actCard))));
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
