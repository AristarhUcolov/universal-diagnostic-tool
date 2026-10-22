import { t, tr } from '../i18n.js';
import { el } from '../ui.js';
import { store, currentECU } from '../app.js';

// ecuSelect shows a selector when the session has several control units.
export function ecuSelect(onChange) {
  const ecus = store.status.ecus || [];
  if (ecus.length < 2) return null;
  return el('label.field', t('dash.unit'),
    el('select', {
      onchange: e => { store.ecu = e.target.value; onChange(); },
    }, ecus.map(e => el('option', { value: e.id, selected: (currentECU() || {}).id === e.id }, e.desc.name))));
}

export function ecuPath(suffix) {
  const e = currentECU();
  return `/api/ecu/${e ? e.id : 'e1'}/${suffix}`;
}

// groupName translates a parameter group id such as "fuel".
export function groupName(g) {
  const key = 'group.' + g;
  const s = t(key);
  return s === key ? g : s;
}

export function hasCap(cap) {
  const e = currentECU();
  return !!(e && (e.desc.capabilities || []).includes(cap));
}

// keyParams picks the parameters shown as tiles on the overview.
export function keyParams(params) {
  const byOBD = {};
  for (const p of params) if (p.obd && !byOBD[p.obd] && p.kind === 'num') byOBD[p.obd] = p;
  const order = ['0C', '05', '0D', '42', '11', '0E', '10', '14', '5E', '0F'];
  let out = order.map(o => byOBD[o]).filter(Boolean);
  if (!out.length) {
    const ids = ['j1939.spn190', 'j1939.spn110', 'j1939.spn84', 'j1939.spn168', 'j1939.spn100', 'j1939.spn183', 'j1939.spn96', 'j1939.spn247'];
    out = ids.map(id => params.find(p => p.id === id)).filter(Boolean);
  }
  if (!out.length) out = params.filter(p => p.kind === 'num').slice(0, 8);
  return out.slice(0, 8);
}

// limits returns warning thresholds for well-known quantities.
export function limitsFor(p) {
  const name = tr(p.name).toLowerCase();
  if (p.obd === '05' || p.id === 'j1939.spn110' || /coolant|охлажд/.test(name)) return { warn: 105, bad: 115 };
  if (p.obd === '42' || p.id === 'j1939.spn168' || /voltage|напряж/.test(name)) {
    return p.max > 20 ? { warnBelow: 24, badBelow: 23, warn: 29.5, bad: 30.5 } : { warnBelow: 12.0, badBelow: 11.5, warn: 14.8, bad: 15.2 };
  }
  if (p.obd === '0C' || p.id === 'j1939.spn190') return { warn: 5500, bad: 6500 };
  return {};
}
