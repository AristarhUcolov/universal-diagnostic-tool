import { api, token } from '../api.js';
import { t, tr, lang, count, unitName } from '../i18n.js';
import { el, clear, toast, busy, confirmBox, fmt, errText, icon, button, pageHeader } from '../ui.js';
import { ecuSelect, ecuPath, hasCap, repairHints, hintCard, sevBadge } from './common.js';
import { rememberCodes } from './dashboard.js';
import { currentECU } from '../app.js';

const flagStyle = {
  active: 'bad', mil: 'bad', testFailed: 'bad', pending: 'warn', intermittent: 'warn',
  testFailedThisCycle: 'warn', permanent: 'info', confirmed: '', history: '',
};
const flagIcon = { active: '●', mil: '⚠', pending: '◐', intermittent: '◐', permanent: '■', history: '○', confirmed: '✓' };

export function render(view, { query } = {}) {
  const result = el('div');
  const freeze = el('div');
  let flagNames = {};
  const readBtn = button('refresh', t('dtc.read'), null, 'primary');
  const clearBtn = button('trash', t('dtc.clear'), null, 'danger');
  const reportBtn = button('printer', t('dtc.report'), null);

  view.append(
    pageHeader(t('dtc.title'), t('dtc.lead')),
    ecuSelect(() => read()),
    el('div.card', el('div.row', readBtn, clearBtn, reportBtn), result),
    freeze,
    lookupCard(query && query.get('q')));

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
    if (currentECU()) rememberCodes(currentECU().id, r.codes || []);
    flagNames = r.flagNames || {};
    await paint(r.codes || []);
    if (hasCap('freeze') && r.codes && r.codes.length) loadFreeze();
  }

  function flagBadge(f) {
    const cls = flagStyle[f] ? '.' + flagStyle[f] : '';
    const icon = flagIcon[f] ? flagIcon[f] + ' ' : '';
    return el('span.badge' + cls, icon + (tr(flagNames[f]) || f));
  }

  async function paint(codes) {
    clear(result);
    if (!codes.length) {
      result.append(el('div.dtc-none', icon('check-circle', 'xl'), el('b.status-line.good', t('dtc.none')), el('span.muted.small', t('dtc.noneHint'))));
      return;
    }
    let hints = {};
    try { hints = await repairHints(); } catch (e) { /* advice is optional */ }
    const bySev = { stop: 0, soon: 0, info: 0 };
    const tbody = el('tbody');
    for (const d of codes) {
      const h = hints[d.hint] || hints.generic;
      if (h) bySev[h.severity] = (bySev[h.severity] || 0) + 1;
      const detail = el('tr.hint-row.hidden', el('td', { colSpan: 5 }, h ? hintCard(h) : null));
      const toggle = el('button.small', { type: 'button', 'aria-expanded': 'false' }, t('dtc.howFix') + ' ▾');
      toggle.onclick = () => {
        const open = !detail.classList.toggle('hidden');
        toggle.textContent = t('dtc.howFix') + (open ? ' ▴' : ' ▾');
        toggle.setAttribute('aria-expanded', String(open));
      };
      tbody.append(el('tr',
        el('td', el('span.code', d.code), d.extra && d.extra.sae ? el('div.muted.small', d.extra.sae) : null),
        el('td', tr(d.desc) || '—',
          d.detail ? el('div.secondary.small', tr(d.detail)) : null,
          !d.known ? el('div.muted.small', t('dtc.unknown')) : null,
          d.raw ? el('div.muted.small', t('dtc.bytes') + ': ', el('span.mono', d.raw)) : null),
        el('td', (d.flags || []).map(flagBadge)),
        el('td.small', d.source || '', d.protocol ? el('div.muted', d.protocol) : null),
        el('td.dtc-fix', h ? sevBadge(h.severity) : null, h ? toggle : null)), detail);
    }
    const summary = ['stop', 'soon', 'info'].filter(k => bySev[k]).map(k => `${bySev[k]} — ${t('sev.' + k).toLowerCase()}`).join(' · ');
    result.append(el('p.secondary', count(codes.length, 'dtc.count'), summary ? ' · ' + summary : ''),
      el('div.table-wrap', el('table.codes',
        el('thead', el('tr', el('th', t('dtc.code')), el('th', t('dtc.desc')), el('th', t('dtc.status')), el('th', t('dtc.source')), el('th', t('dtc.fix')))),
        tbody)));
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

  read().catch(e => { clear(result).append(el('p', { style: { color: 'var(--critical-ink)' } }, errText(e))); });
}

// lookupCard searches the offline database of trouble codes; initial is
// a code or a word to look up at once (from the command palette).
export function lookupCard(initial) {
  const out = el('div');
  const inp = el('input', { type: 'search', placeholder: t('dtc.lookupPh'), size: 32, value: initial || '' });
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
  if (initial) setTimeout(() => { inp.oninput(); inp.focus(); }, 0);
  return el('div.card', el('h2', t('dtc.lookup')), el('div.row', icon('search'), inp), out);
}
