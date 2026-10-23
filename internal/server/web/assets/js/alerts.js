import { api } from './api.js';
import { t, tr, unitName, fill } from './i18n.js';
import { fmt, convert, toast } from './ui.js';
import { limitsFor } from './pages/common.js';

// Alerts warn when a polled value leaves its safe range — an overheating
// engine, a battery that is not charged, an engine speed past the red
// line — on whatever page is open: in the top bar, as a message and,
// unless switched off, with a short sound. An alert ends only when the
// value is back in its normal range, so a value hovering at the limit
// does not raise it again and again.

const meta = {};   // control unit id -> Promise of {param id: param}
const active = {}; // param id -> {p, v, text}
let store;

export function startAlerts(s, subscribe) {
  store = s;
  subscribe('live', check);
  subscribe('status', st => {
    if (st.state !== 'connected') {
      for (const k of Object.keys(meta)) delete meta[k];
      for (const k of Object.keys(active)) delete active[k];
      paint();
    }
  });
  document.getElementById('alert-pill').onclick = () => { location.hash = '#/gauges'; };
}

function paramsOf(ecu) {
  if (!meta[ecu]) {
    meta[ecu] = api.get(`/api/ecu/${ecu}/params`)
      .then(r => Object.fromEntries((r.params || []).map(p => [p.id, p])))
      .catch(() => { delete meta[ecu]; return {}; });
  }
  return meta[ecu];
}

function level(v, l) {
  if ((l.bad !== undefined && v >= l.bad) || (l.badBelow !== undefined && v <= l.badBelow)) return 'bad';
  if ((l.warn !== undefined && v >= l.warn) || (l.warnBelow !== undefined && v <= l.warnBelow)) return 'warn';
  return '';
}

async function check(ev) {
  if (!ev || !ev.values || !ev.ecu || store.settings.alerts === false) return;
  const ps = await paramsOf(ev.ecu);
  let changed = false;
  for (const v of ev.values) {
    const p = ps[v.id];
    if (!p || !v.ok || v.t) continue;
    const lim = limitsFor(p);
    const lvl = level(v.v, lim);
    const cur = active[v.id];
    if (lvl === 'bad') {
      if (!cur) {
        active[v.id] = { p, v: v.v, text: message(p, v.v, lim) };
        toast(active[v.id].text, 'bad');
        if (store.settings.alertSound !== false) beep();
      } else {
        cur.v = v.v;
      }
      changed = true;
    } else if (cur && lvl === '') {
      delete active[v.id];
      changed = true;
    }
  }
  if (changed) paint();
}

function message(p, v, lim) {
  const c = convert(v, p.unit, store.settings.units);
  const value = fmt(c.v, p.digits) + ' ' + unitName(c.unit);
  const name = tr(p.name);
  if (lim.bad !== undefined && v >= lim.bad && (p.obd === '05' || /coolant|охлажд/i.test(name))) return fill(t('alert.hot'), { v: value });
  if (lim.badBelow !== undefined && v <= lim.badBelow) return fill(t('alert.lowV'), { v: value });
  if (lim.bad !== undefined && v >= lim.bad && p.unit === 'V') return fill(t('alert.highV'), { v: value });
  if (p.obd === '0C' || p.id === 'j1939.spn190') return fill(t('alert.rpm'), { v: value });
  return fill(t('alert.generic'), { name, v: value });
}

function paint() {
  const pill = document.getElementById('alert-pill');
  const list = Object.values(active);
  pill.classList.toggle('hidden', !list.length);
  if (!list.length) return;
  const first = list[0];
  const c = convert(first.v, first.p.unit, store.settings.units);
  pill.textContent = `⚠ ${tr(first.p.name)}: ${fmt(c.v, first.p.digits)} ${unitName(c.unit)}` + (list.length > 1 ? ` (+${list.length - 1})` : '');
  pill.title = list.map(a => a.text).join('\n');
}

// beep plays two short falling tones.
let audio = null;
function beep() {
  try {
    audio = audio || new (window.AudioContext || window.webkitAudioContext)();
    const now = audio.currentTime;
    [[0, 880], [0.25, 660]].forEach(([at, f]) => {
      const o = audio.createOscillator(), g = audio.createGain();
      o.type = 'sine';
      o.frequency.value = f;
      g.gain.setValueAtTime(0.0001, now + at);
      g.gain.exponentialRampToValueAtTime(0.25, now + at + 0.02);
      g.gain.exponentialRampToValueAtTime(0.0001, now + at + 0.22);
      o.connect(g).connect(audio.destination);
      o.start(now + at);
      o.stop(now + at + 0.24);
    });
  } catch (e) { /* no sound available */ }
}
