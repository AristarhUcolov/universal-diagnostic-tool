import { api } from '../api.js';
import { t, tr, unitName } from '../i18n.js';
import { el, clear, fmt, pageHeader, errText } from '../ui.js';
import { ecuPath } from './common.js';

// Guided checks: a few steps with instructions, the live values they
// watch and a verdict computed from those values. Values are found by
// their OBD-II PID, which works for OBD cars and for manufacturer
// descriptions that map their values to PIDs.
//
// A verdict is { state: 'wait' | 'ok' | 'warn' | 'bad', text }.

const RPM = '0C', COOLANT = '05', VOLTS = '42', O2 = '14', TPS = '11';

function fill(s, vars) {
  return s.replace(/\{(\w+)\}/g, (_, k) => (vars[k] !== undefined ? vars[k] : ''));
}

// Samples kept per value during a step.
class Samples {
  constructor() { this.s = {}; this.t0 = Date.now(); }
  add(id, v) { (this.s[id] = this.s[id] || []).push([Date.now(), v]); }
  since(id, sec) {
    const from = Date.now() - sec * 1000;
    return (this.s[id] || []).filter(x => x[0] >= from).map(x => x[1]);
  }
  all(id) { return (this.s[id] || []).map(x => x[1]); }
  last(id) { const a = this.s[id]; return a && a.length ? a[a.length - 1][1] : null; }
  first(id) { const a = this.s[id]; return a && a.length ? a[0][1] : null; }
  elapsed() { return (Date.now() - this.t0) / 1000; }
}

const avg = a => a.reduce((x, y) => x + y, 0) / a.length;
const wait = (text) => ({ state: 'wait', text: text || t('chk.waiting') });

// engineGate checks the engine state a step needs.
function engineGate(s, need) {
  const rpm = s.last(RPM);
  if (rpm === null || !need) return null;
  if (need === 'running' && rpm < 400) return wait(t('chk.needRunning'));
  if (need === 'stopped' && rpm > 50) return wait(t('chk.needStopped'));
  if (need === 'warm') {
    if (rpm < 400) return wait(t('chk.needRunning'));
    const c = s.last(COOLANT);
    if (c !== null && c < 70) return wait(t('chk.needWarm'));
  }
  return null;
}

const CHECKS = [
  {
    id: 'battery', needs: [VOLTS],
    steps: [
      {
        text: 'chk.battery.s1', watch: [VOLTS, RPM], need: 'stopped',
        judge: s => {
          const v = s.since(VOLTS, 3);
          if (v.length < 3) return wait();
          const u = avg(v), at = ` (${fmt(u, 2)} ${unitName('V')})`;
          if (u > 13.0) return { state: 'warn', text: t('chk.battery.high1') + at };
          if (u >= 12.3) return { state: 'ok', text: t('chk.battery.ok1') + at };
          if (u >= 12.0) return { state: 'warn', text: t('chk.battery.warn1') + at };
          return { state: 'bad', text: t('chk.battery.bad1') + at };
        },
      },
      {
        text: 'chk.battery.s2', watch: [VOLTS, RPM], need: 'running',
        judge: s => {
          const v = s.since(VOLTS, 5);
          if (v.length < 5) return wait();
          const u = avg(v), at = ` (${fmt(u, 2)} ${unitName('V')})`;
          if (u > 14.8) return { state: 'bad', text: t('chk.battery.over') + at };
          if (u >= 13.5) return { state: 'ok', text: t('chk.battery.ok2') + at };
          if (u >= 13.2) return { state: 'warn', text: t('chk.battery.warn2') + at };
          return { state: 'bad', text: t('chk.battery.bad2') + at };
        },
      },
      {
        text: 'chk.battery.s3', watch: [VOLTS, RPM], need: 'running',
        judge: s => {
          if (s.elapsed() < 10) return wait();
          const v = s.since(VOLTS, 10);
          if (!v.length) return wait();
          const u = Math.min(...v), at = ` (${t('chk.min')} ${fmt(u, 2)} ${unitName('V')})`;
          if (u >= 13.0) return { state: 'ok', text: t('chk.battery.ok3') + at };
          if (u >= 12.8) return { state: 'warn', text: t('chk.battery.warn3') + at };
          return { state: 'bad', text: t('chk.battery.bad3') + at };
        },
      },
    ],
  },
  {
    id: 'warmup', needs: [COOLANT],
    steps: [
      {
        text: 'chk.warmup.s1', watch: [COOLANT, RPM], need: 'running', note: 'chk.warmup.note',
        judge: s => {
          const c = s.last(COOLANT);
          if (c === null) return wait();
          const prog = fill(t('chk.warmup.progress'), { t: fmt(c, 0), s: fmt(s.first(COOLANT), 0), m: fmt(s.elapsed() / 60, 1) });
          if (c > 105) return { state: 'bad', text: t('chk.warmup.hot') + ' — ' + prog };
          if (c >= 80) return { state: 'ok', text: t('chk.warmup.ok') + ' — ' + prog };
          if (s.elapsed() > 15 * 60 && c < 70) return { state: 'bad', text: t('chk.warmup.bad') + ' — ' + prog };
          return wait(prog);
        },
      },
    ],
  },
  {
    id: 'idle', needs: [RPM],
    steps: [
      {
        text: 'chk.idle.s1', watch: [RPM, COOLANT], need: 'warm',
        judge: s => {
          if (s.elapsed() < 20) return wait(fill(t('chk.secondsLeft'), { n: Math.ceil(20 - s.elapsed()) }));
          const r = s.since(RPM, 20);
          if (r.length < 10) return wait();
          const a = avg(r), d = Math.max(...r) - Math.min(...r);
          const stats = fill(t('chk.idle.stats'), { a: fmt(a, 0), d: fmt(d, 0) });
          if (d > 150) return { state: 'bad', text: t('chk.idle.bad') + ': ' + stats };
          if (a < 650 || a > 1100) return { state: 'warn', text: t('chk.idle.odd') + ': ' + stats };
          if (d > 80) return { state: 'warn', text: t('chk.idle.warn') + ': ' + stats };
          return { state: 'ok', text: t('chk.idle.ok') + ': ' + stats };
        },
      },
    ],
  },
  {
    id: 'o2', needs: [O2],
    steps: [
      {
        text: 'chk.o2.s1', watch: [O2, RPM, COOLANT], need: 'warm',
        judge: s => {
          const rpm = s.last(RPM);
          if (rpm !== null && (rpm < 1700 || rpm > 3000)) return wait(t('chk.o2.rpm'));
          const v = s.since(O2, 10);
          if (s.elapsed() < 10 || v.length < 8) return wait(fill(t('chk.secondsLeft'), { n: Math.max(1, Math.ceil(10 - s.elapsed())) }));
          let n = 0;
          for (let i = 1; i < v.length; i++) if ((v[i - 1] < 0.45) !== (v[i] < 0.45)) n++;
          const lo = Math.min(...v), hi = Math.max(...v);
          const stats = ' — ' + fill(t('chk.o2.stats'), { n, lo: fmt(lo, 2), hi: fmt(hi, 2) });
          if (hi - lo < 0.1) return { state: 'bad', text: t('chk.o2.flat') + stats };
          if (hi < 0.35) return { state: 'bad', text: t('chk.o2.lean') + stats };
          if (lo > 0.55) return { state: 'bad', text: t('chk.o2.rich') + stats };
          if (n >= 6 && lo < 0.3 && hi > 0.6) return { state: 'ok', text: t('chk.o2.ok') + stats };
          return { state: 'warn', text: t('chk.o2.slow') + stats };
        },
      },
    ],
  },
  {
    id: 'tps', needs: [TPS],
    steps: [
      {
        text: 'chk.tps.s1', watch: [TPS, RPM], need: 'stopped',
        judge: s => {
          const v = s.all(TPS);
          if (v.length < 3) return wait();
          const lo = Math.min(...v), hi = Math.max(...v), vars = { lo: fmt(lo, 0), hi: fmt(hi, 0) };
          const last = v[v.length - 1];
          if (hi >= 85 && lo <= 5 && last <= 5) return { state: 'ok', text: fill(t('chk.tps.ok'), vars) };
          if (s.elapsed() < 25) return wait(fill(t('chk.tps.progress'), vars));
          if (hi < 85) return { state: 'bad', text: fill(t('chk.tps.noMax'), vars) };
          return { state: 'bad', text: fill(t('chk.tps.noMin'), vars) };
        },
      },
    ],
  },
];

const verdictIcon = { wait: '…', ok: '✓', warn: '!', bad: '✕' };

export function render(view) {
  const box = el('div');
  let params = [];
  let alive = true;
  let timer = 0;
  view.append(pageHeader(t('chk.title'), t('chk.lead')), box);

  const paramFor = obd => params.find(p => p.obd === obd && p.kind === 'num' && (obd !== O2 || p.unit === 'V'));

  function list() {
    clearTimeout(timer);
    clear(box);
    const grid = el('div.grid.chk-grid');
    for (const c of CHECKS) {
      const ok = c.needs.every(paramFor);
      const start = el('button.primary', t('chk.start'));
      start.disabled = !ok;
      start.onclick = () => run(c);
      grid.append(el('div.card.chk-card' + (ok ? '' : '.off'),
        el('h2', t('chk.' + c.id)),
        el('p.secondary', t('chk.' + c.id + 'Lead')),
        el('p.muted.small', fill(t('chk.steps'), { n: c.steps.length })),
        ok ? start : el('p.muted.small', t('chk.unavailable'))));
    }
    box.append(grid);
  }

  function run(c) {
    const results = [];
    let i = 0;
    let samples = new Samples();
    let verdict = wait();
    const stepTitle = el('div.chk-step');
    const text = el('p.chk-text');
    const note = el('p.muted.small');
    const tiles = el('div.grid.svc-live');
    const verdictBox = el('div.chk-verdict');
    const back = el('button', t('chk.back'));
    const next = el('button.primary', t('chk.next'));
    const abort = el('button.ghost', t('chk.abort'));
    clear(box).append(el('div.card.chk-run',
      el('div.chk-runhead', el('h2', t('chk.' + c.id)), abort),
      stepTitle, text, note, tiles, verdictBox,
      el('div.row', back, next)));
    let tileMap = {};

    const showStep = () => {
      const st = c.steps[i];
      samples = new Samples();
      verdict = wait();
      stepTitle.textContent = `${t('chk.step')} ${i + 1} ${t('chk.of')} ${c.steps.length}`;
      text.textContent = t(st.text);
      note.textContent = st.note ? t(st.note) : '';
      back.disabled = i === 0;
      next.textContent = i === c.steps.length - 1 ? t('chk.finish') : t('chk.next');
      clear(tiles);
      tileMap = {};
      for (const obd of st.watch) {
        const p = paramFor(obd);
        if (!p) continue;
        const val = el('span.value', '—');
        tiles.append(el('div.card.tile.mini', el('span.label', tr(p.name)), el('div', val, el('span.unit', unitName(p.unit)))));
        tileMap[obd] = { p, val };
      }
      paintVerdict();
    };

    const paintVerdict = () => {
      clear(verdictBox).append(el('div.verdict.' + verdict.state,
        el('span.verdict-icon', verdictIcon[verdict.state]), el('span', verdict.text)));
    };

    const poll = async () => {
      if (!alive) return;
      const st = c.steps[i];
      const ids = Object.values(tileMap).map(x => x.p.id);
      if (ids.length) {
        try {
          const r = await api.post(ecuPath('read'), { ids });
          for (const v of r.values || []) {
            const entry = Object.entries(tileMap).find(([, x]) => x.p.id === v.id);
            if (!entry || !v.ok) continue;
            const [obd, x] = entry;
            samples.add(obd, v.v);
            x.val.textContent = fmt(v.v, x.p.digits);
          }
        } catch (e) {
          verdict = { state: 'warn', text: errText(e) };
        }
      }
      const gate = engineGate(samples, st.need);
      if (gate) {
        verdict = gate;
        samples = Object.assign(new Samples(), { s: samples.s, t0: Date.now() });
      } else if (!['ok', 'bad'].includes(verdict.state)) {
        verdict = st.judge(samples);
      }
      paintVerdict();
      timer = setTimeout(poll, 500);
    };

    back.onclick = () => { if (i > 0) { i--; results.length = i; showStep(); } };
    next.onclick = () => {
      results[i] = verdict;
      if (i < c.steps.length - 1) { i++; showStep(); return; }
      finish();
    };
    abort.onclick = () => list();

    const finish = () => {
      clearTimeout(timer);
      const worst = results.some(r => r.state === 'bad') ? 'bad' : results.some(r => r.state !== 'ok') ? 'warn' : 'ok';
      clear(box).append(el('div.card.chk-run',
        el('h2', t('chk.' + c.id) + ' — ' + t('chk.result')),
        el('div.verdict.' + worst, el('span.verdict-icon', verdictIcon[worst]), el('span', t('chk.summary.' + worst))),
        el('ol.chk-results', c.steps.map((st, k) => el('li',
          el('span.status-line.' + ({ ok: 'good', bad: 'bad', warn: 'warn' }[results[k].state] || ''), verdictIcon[results[k].state] + ' '),
          t(st.text), el('div.muted.small', results[k].text)))),
        el('div.row', el('button.primary', { onclick: () => run(c) }, t('chk.again')), el('button', { onclick: () => list() }, t('chk.toList')))));
    };

    showStep();
    poll();
  }

  (async () => {
    clear(box).append(el('p.muted', '…'));
    try {
      params = (await api.get(ecuPath('params'))).params || [];
    } catch (e) {
      params = [];
    }
    if (alive) list();
  })();
  return () => { alive = false; clearTimeout(timer); };
}
