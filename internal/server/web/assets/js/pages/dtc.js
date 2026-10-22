import { api, token } from '../api.js';
import { t, tr, lang, count, unitName } from '../i18n.js';
import { el, clear, toast, busy, confirmBox, fmt } from '../ui.js';
import { ecuSelect, ecuPath, hasCap } from './common.js';

const flagStyle = {
  active: 'bad', mil: 'bad', testFailed: 'bad', pending: 'warn', intermittent: 'warn',
  testFailedThisCycle: 'warn', permanent: 'info', confirmed: '', history: '',
};
const flagIcon = { active: '●', mil: '⚠', pending: '◐', intermittent: '◐', permanent: '■', history: '○', confirmed: '✓' };

export function render(view) {
  const result = el('div');
  const freeze = el('div');
  let flagNames = {};
  const readBtn = el('button.primary', t('dtc.read'));
  const clearBtn = el('button.danger', t('dtc.clear'));
  const reportBtn = el('button', t('dtc.report'));

  view.append(
    el('h1', t('dtc.title')),
    ecuSelect(() => read()),
    el('div.card', el('div.row', readBtn, clearBtn, reportBtn), result),
    freeze,
    lookupCard());

  readBtn.onclick = () => busy(readBtn, read);
  clearBtn.onclick = async () => {
    if (!await confirmBox(t('dtc.clear'), t('dtc.clearConfirm'), t('dtc.clear'), true)) return;
    await busy(clearBtn, async () => {
      await api.post(ecuPath('dtc/clear'));
      toast(t('dtc.cleared'), 'good');
      await read();
    });
  };
  reportBtn.onclick = () => window.open(`/api/report?lang=${lang()}&t=${encodeURIComponent(token)}`, '_blank');

  async function read() {
    clear(result).append(el('p.muted', '…'));
    const r = await api.get(ecuPath('dtc'));
    flagNames = r.flagNames || {};
    paint(r.codes || []);
    if (hasCap('freeze') && r.codes && r.codes.length) loadFreeze();
  }

  function flagBadge(f) {
    const cls = flagStyle[f] ? '.' + flagStyle[f] : '';
    const icon = flagIcon[f] ? flagIcon[f] + ' ' : '';
    return el('span.badge' + cls, icon + (tr(flagNames[f]) || f));
  }

  function paint(codes) {
    clear(result);
    if (!codes.length) {
      result.append(el('div.empty', el('span.status-line.good', '✓ ' + t('dtc.none'))));
      return;
    }
    result.append(el('p.secondary', count(codes.length, 'dtc.count')),
      el('div.table-wrap', el('table.codes',
        el('thead', el('tr', el('th', t('dtc.code')), el('th', t('dtc.desc')), el('th', t('dtc.status')), el('th', t('dtc.source')))),
        el('tbody', codes.map(d => el('tr',
          el('td', el('span.code', d.code), d.extra && d.extra.sae ? el('div.muted.small', d.extra.sae) : null),
          el('td', tr(d.desc) || '—',
            d.detail ? el('div.secondary.small', tr(d.detail)) : null,
            !d.known ? el('div.muted.small', t('dtc.unknown')) : null,
            d.raw ? el('div.muted.small', t('dtc.bytes') + ': ', el('span.mono', d.raw)) : null),
          el('td', (d.flags || []).map(flagBadge)),
          el('td.small', d.source || '', d.protocol ? el('div.muted', d.protocol) : null)))))));
  }

  async function loadFreeze() {
    clear(freeze);
    try {
      const r = await api.get(ecuPath('freeze'));
      if (!r.supported || !r.values || !r.values.length) return;
      const params = r.params || {};
      freeze.append(el('div.card', el('h2', t('dtc.freeze')), el('p.muted.small', t('dtc.freezeHint')),
        el('table', el('tbody', r.values.map(v => {
          const p = params[v.id];
          const name = v.id === 'freeze.dtc' ? t('dtc.code') : (p ? tr(p.name) : v.id);
          const val = v.t ? v.t : (v.l ? tr(v.l) : fmt(v.v, p ? p.digits : 2));
          return el('tr', el('td', name), el('td.num', val, ' ', p ? unitName(p.unit) : ''));
        })))));
    } catch (e) { /* optional */ }
  }

  read().catch(e => { clear(result).append(el('p', { style: { color: 'var(--critical-ink)' } }, e.message)); });
}

export function lookupCard() {
  const out = el('div');
  const inp = el('input', { placeholder: t('dtc.lookupPh'), size: 32 });
  let timer;
  inp.oninput = () => {
    clearTimeout(timer);
    timer = setTimeout(async () => {
      const q = inp.value.trim();
      clear(out);
      if (q.length < 2) return;
      try {
        const r = await api.get('/api/dtc/search?q=' + encodeURIComponent(q));
        const res = r.results || [];
        if (!res.length) { out.append(el('p.muted', '—')); return; }
        out.append(el('table', el('tbody', res.slice(0, 60).map(m => el('tr',
          el('td', el('span.code.mono', m.code)), el('td', tr(m.desc)), el('td', el('span.badge', m.family)))))));
      } catch (e) { toast(e.message, 'bad'); }
    }, 250);
  };
  return el('div.card', el('h2', t('dtc.lookup')), inp, out);
}
