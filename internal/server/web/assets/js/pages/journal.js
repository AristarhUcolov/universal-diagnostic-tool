import { api, token } from '../api.js';
import { t } from '../i18n.js';
import { el, clear, toast, icon, button } from '../ui.js';
import { subscribe } from '../app.js';

const kindClass = { tx: 'k-tx', rx: 'k-rx', warn: 'k-warn', error: 'k-error', info: '' };

export function render(view) {
  const tbody = el('tbody');
  const kind = el('select', ['', 'tx', 'rx', 'info', 'warn', 'error'].map(k => el('option', { value: k }, k || t('journal.all'))));
  const search = el('input', { type: 'search', placeholder: t('common.search'), size: 18 });
  const pauseBtn = el('button', { type: 'button' });
  const fileInfo = el('span.mono.small');
  let paused = false;
  let entries = [];
  const max = 3000;

  view.append(el('h1', t('journal.title')), el('p.secondary', t('journal.lead'), ' ', fileInfo),
    el('div.card', el('div.row', kind, search, pauseBtn,
      button('download', t('common.export'), () => window.open('/api/journal/export.csv?t=' + encodeURIComponent(token), '_blank')),
      button('trash', t('journal.clear'), async () => { await api.post('/api/journal/clear'); entries = []; paint(); }),
      button('folder', t('journal.folder'), () => api.post('/api/open-folder', { kind: 'logs' }).catch(e => toast(e.message, 'bad')))),
    el('div.table-wrap', { style: { maxHeight: '68vh', overflow: 'auto', marginTop: '10px' } },
      el('table.journal-table', el('thead', el('tr', el('th', t('journal.time')), el('th', t('journal.kind')), el('th', t('journal.layer')),
        el('th', t('journal.addr')), el('th', t('journal.data')), el('th', t('journal.text')))), tbody))));

  const paintPause = () => pauseBtn.replaceChildren(icon(paused ? 'play' : 'stop'), paused ? t('journal.resume') : t('journal.pause'));
  paintPause();
  pauseBtn.onclick = () => { paused = !paused; paintPause(); if (!paused) paint(); };
  kind.onchange = paint;
  search.oninput = paint;

  function row(e) {
    const time = new Date(e.time);
    return el('tr', el('td.mono.small', time.toLocaleTimeString() + '.' + String(time.getMilliseconds()).padStart(3, '0')),
      el('td', el('span' + (kindClass[e.kind] ? '.' + kindClass[e.kind] : ''), e.kind)), el('td', e.layer), el('td.mono', e.addr || ''),
      el('td.hex', e.hex || ''), el('td', e.text || ''));
  }

  function visible(e) {
    if (kind.value && e.kind !== kind.value) return false;
    const q = search.value.trim().toLowerCase();
    return !q || (e.text || '').toLowerCase().includes(q) || (e.hex || '').toLowerCase().includes(q) || (e.layer || '').includes(q);
  }

  function paint() {
    clear(tbody);
    const show = entries.filter(visible).slice(-800);
    for (let i = show.length - 1; i >= 0; i--) tbody.append(row(show[i]));
  }

  api.get('/api/journal?limit=2000').then(r => {
    entries = r.entries || [];
    fileInfo.textContent = r.file || '';
    paint();
  });
  return subscribe('journal', list => {
    entries.push(...list);
    if (entries.length > max) entries = entries.slice(-max);
    if (paused) return;
    for (const e of list) if (visible(e)) tbody.insertBefore(row(e), tbody.firstChild);
    while (tbody.children.length > 800) tbody.removeChild(tbody.lastChild);
  });
}
