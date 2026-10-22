import { api } from '../api.js';
import { t } from '../i18n.js';
import { el, clear, toast, busy } from '../ui.js';
import { store, subscribe } from '../app.js';

export function render(view) {
  view.append(el('h1', t('bus.title')), el('p.secondary', t('bus.lead')));
  if (!store.status.canBus) {
    view.append(el('div.card.empty', t('bus.noCan')));
    return null;
  }
  const rows = new Map();
  const tbody = el('tbody');
  const filter = el('input', { placeholder: t('bus.filterPh'), size: 14 });
  const startBtn = el('button.primary');
  let on = false;
  const sendBox = el('div.row');
  if (store.settings.expert) {
    const inp = el('input', { placeholder: t('bus.sendPh'), size: 28, style: { fontFamily: 'var(--mono)' } });
    const b = el('button', t('common.send'));
    b.onclick = () => busy(b, async () => { await api.post('/api/monitor/send', { frame: inp.value.trim() }); toast('→ ' + inp.value, 'good'); });
    sendBox.append(el('span.muted.small', t('bus.send') + ':'), inp, b);
  }
  view.append(el('div.card', el('div.row', startBtn, filter, sendBox),
    el('div.table-wrap', { style: { marginTop: '10px' } }, el('table.bus-table',
      el('thead', el('tr', el('th', t('bus.id')), el('th', t('bus.len')), el('th', t('bus.data')), el('th', 'ASCII'),
        el('th', t('bus.count')), el('th', t('bus.period')), el('th', t('bus.pgn')))), tbody))));

  function paintBtn() {
    startBtn.textContent = on ? '■ ' + t('common.stop') : '▶ ' + t('common.start');
    startBtn.className = on ? 'danger' : 'primary';
  }
  startBtn.onclick = () => busy(startBtn, async () => {
    await api.post(on ? '/api/monitor/stop' : '/api/monitor/start');
    on = !on;
    paintBtn();
  });
  paintBtn();

  function dataCells(e) {
    const bytes = e.data.split(' ').filter(Boolean);
    return bytes.map((b, i) => el((e.changed / Math.pow(2, i)) % 2 >= 1 ? 'span.ch' : 'span', b + ' '));
  }

  const unsub = subscribe('can', entries => {
    const f = filter.value.trim().toUpperCase();
    for (const e of entries) {
      let r = rows.get(e.id);
      if (!r) {
        r = { tr: el('tr'), id: e.id };
        rows.set(e.id, r);
        const sorted = [...rows.keys()].sort();
        const idx = sorted.indexOf(e.id);
        const next = rows.get(sorted[idx + 1]);
        tbody.insertBefore(r.tr, next ? next.tr : null);
      }
      r.tr.classList.toggle('hidden', !!f && !e.id.includes(f));
      clear(r.tr).append(
        el('td.mono', e.id), el('td.num', String(e.len)), el('td.data', dataCells(e)), el('td.mono.muted', e.ascii || ''),
        el('td.num', String(e.count)), el('td.num', e.periodMs ? e.periodMs.toFixed(0) : ''),
        el('td.small', e.ext && e.pgn ? `${e.pgn} · ${e.source || ''}` : ''));
    }
  });
  return () => { unsub(); if (on) api.post('/api/monitor/stop').catch(() => {}); };
}
