import { api } from '../api.js';
import { t, tr, unitName, fill } from '../i18n.js';
import { el, clear, busy, fmt, pageHeader, errText, icon } from '../ui.js';
import { currentECU } from '../app.js';
import { ecuSelect, ecuPath, hasCap } from './common.js';

// The monitors page: readiness of the emission monitors and the results
// of the tests the engine controller runs on itself (OBD-II service $06),
// with the misfire counters per cylinder drawn as bars.

const cache = {}; // control unit id -> {results, at}: the last reading stays

export function render(view) {
  const readyBox = el('div');
  const misBox = el('div');
  const resBox = el('div');
  const readBtn = el('button.primary', { type: 'button' }, icon('refresh'), t('mon.read'));
  readBtn.onclick = () => busy(readBtn, readTests);
  view.append(
    pageHeader(t('mon.title'), t('mon.lead'), readBtn),
    ecuSelect(() => load()),
    readyBox, misBox, resBox);

  async function loadReadiness() {
    clear(readyBox);
    if (!hasCap('readiness')) return;
    readyBox.append(el('div.card', el('h2', t('mon.readiness')), el('p.muted', '…')));
    try {
      const r = await api.get(ecuPath('readiness'));
      if (!r.supported) { clear(readyBox); return; }
      const mons = r.monitors || [];
      const open = mons.filter(m => !m.complete);
      clear(readyBox).append(el('div.card',
        el('div.row.mon-head', el('h2', t('mon.readiness')), el('span.grow'),
          el('span.badge' + (r.mil ? '.bad' : '.good'), (r.mil ? '⚠ ' : '✓ ') + t(r.mil ? 'mon.milOn' : 'mon.milOff')),
          el('span.badge' + (open.length ? '.warn' : '.good'), open.length ? fill(t('mon.notDone'), { n: open.length }) : '✓ ' + t('mon.allDone'))),
        el('div.mon-list', mons.map(m => el('div.mon-item' + (m.complete ? '.done' : '.open'),
          el('span.mon-mark', m.complete ? '✓' : '…'), tr(m.name),
          el('span.muted.small', ' — ' + t(m.complete ? 'mon.complete' : 'mon.incomplete'))))),
        open.length ? el('p.secondary.small', t('mon.readyHint')) : null,
        el('p.muted.small', t('mon.inspect'))));
    } catch (e) {
      clear(readyBox).append(el('div.card', el('h2', t('mon.readiness')), el('p.secondary', errText(e))));
    }
  }

  async function readTests() {
    const e = currentECU();
    if (!e) return;
    try {
      const r = await api.get(ecuPath('tests'));
      cache[e.id] = { results: r.results || [], at: new Date(), supported: r.supported };
    } catch (err) {
      cache[e.id] = { error: errText(err), at: new Date() };
    }
    paintTests();
  }

  function paintTests() {
    clear(misBox);
    clear(resBox);
    const e = currentECU();
    if (!hasCap('tests')) {
      readBtn.disabled = true;
      resBox.append(el('div.card', el('h2', t('mon.results')), el('p.secondary', t('mon.unsupported'))));
      return;
    }
    readBtn.disabled = false;
    const c = e && cache[e.id];
    if (!c) {
      resBox.append(el('div.card', el('h2', t('mon.results')), el('p.muted', t('mon.none'))));
      return;
    }
    if (c.error || c.supported === false) {
      resBox.append(el('div.card', el('h2', t('mon.results')), el('p.secondary', c.error || t('mon.unsupported'))));
      return;
    }
    misBox.append(misfireCard(c.results));
    resBox.append(resultsCard(c.results, c.at));
  }

  function load() {
    loadReadiness();
    paintTests();
  }

  load();
}

// misfireCard draws the misfire counts of each cylinder: a bar for the
// current or last drive and a mark for the average of the last ten.
function misfireCard(results) {
  const cyl = {};
  for (const r of results) {
    if (r.mid < 0xA2 || r.mid > 0xAD) continue;
    const n = r.mid - 0xA1;
    const c = cyl[n] = cyl[n] || { n, now: null, avg: null };
    if (r.tid === 0x0C) c.now = r.value;
    if (r.tid === 0x0B) c.avg = r.value;
  }
  const rows = Object.values(cyl).sort((a, b) => a.n - b.n);
  if (!rows.length) return null;
  const top = Math.max(1, ...rows.flatMap(c => [c.now || 0, c.avg || 0]));
  const scale = v => Math.min(100, (v || 0) / top * 100);
  const worst = rows.reduce((w, c) => ((c.now || 0) > (w ? w.now || 0 : 0) ? c : w), null);
  return el('div.card',
    el('h2', t('mon.misfire')),
    el('div.mis-chart', rows.map(c => el('div.mis-row', {
      title: `${t('mon.misNow')}: ${c.now ?? '—'} · ${t('mon.misAvg')}: ${c.avg ?? '—'}`,
    },
    el('span.mis-label', fill(t('mon.cyl'), { n: c.n })),
    el('div.mis-track',
      el('div.mis-bar', { style: { width: scale(c.now) + '%' } }),
      c.avg !== null ? el('div.mis-avg', { style: { left: scale(c.avg) + '%' } }) : null),
    el('span.mis-val', c.now === null ? '—' : fmt(c.now, 0))))),
    el('div.mis-legend.small', el('span.k.now', t('mon.misNow')), el('span.k.avg', t('mon.misAvg'))),
    el('p.secondary', worst && worst.now > 0 ? fill(t('mon.misWorst'), { n: worst.n }) : t('mon.misNone')));
}

// resultsCard lists the tests grouped by monitor, each value against its
// limits.
function resultsCard(results, at) {
  const sources = new Set(results.map(r => r.source));
  const groups = new Map();
  for (const r of results) {
    const key = r.source + ':' + r.mid;
    if (!groups.has(key)) groups.set(key, []);
    groups.get(key).push(r);
  }
  const failed = results.filter(r => r.limits && !r.pass).length;
  const rows = [];
  for (const list of groups.values()) {
    const head = list[0];
    rows.push(el('tr.mon-group', el('td', { colSpan: 4 }, tr(head.monitor), sources.size > 1 ? el('span.muted.small', ' · ' + head.source) : null)));
    for (const r of list) {
      const unit = r.unit ? ' ' + unitName(r.unit) : '';
      const name = r.test && tr(r.test) ? tr(r.test) : fill(t('mon.mfr'), { tid: r.tid.toString(16).toUpperCase().padStart(2, '0') });
      const verdict = !r.limits ? el('span.muted', t('mon.noLimits'))
        : r.pass ? el('span.badge.good', '✓ ' + t('mon.pass')) : el('span.badge.bad', '✕ ' + t('mon.fail'));
      rows.push(el('tr' + (r.limits && !r.pass ? '.mon-fail' : ''),
        el('td', name),
        el('td.mono', fmt(r.value, r.digits) + unit, r.limits && r.max > r.min ? rangeBar(r) : null),
        el('td.mono.secondary', r.limits ? `${fmt(r.min, r.digits)} … ${fmt(r.max, r.digits)}${unit}` : '—'),
        el('td', verdict)));
    }
  }
  return el('div.card',
    el('div.row.mon-head', el('h2', t('mon.results')), el('span.grow'),
      el('span.secondary.small', fill(t('mon.summary'), { n: results.length, f: failed })),
      el('span.muted.small', fill(t('mon.readAt'), { time: at.toLocaleTimeString() }))),
    results.length ? el('table.mon-table', el('thead', el('tr',
      el('th', t('mon.colTest')), el('th', t('mon.colValue')), el('th', t('mon.colLimits')), el('th', t('mon.colResult')))),
    el('tbody', rows)) : el('p.muted', t('mon.empty')));
}

// rangeBar shows where a value lies between its limits.
function rangeBar(r) {
  const f = Math.max(0, Math.min(1, (r.value - r.min) / (r.max - r.min)));
  return el('div.mon-range', el('div.mon-dot' + (r.pass ? '' : '.bad'), { style: { left: (f * 100) + '%' } }));
}
