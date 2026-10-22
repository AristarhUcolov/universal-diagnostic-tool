import { api } from '../api.js';
import { t, tr } from '../i18n.js';
import { el, clear, toast, statusBadge, copyText, errText } from '../ui.js';
import { store, currentECU } from '../app.js';
import { ecuSelect, ecuPath } from './common.js';

export function render(view) {
  const identBox = el('div.card');
  const defBox = el('div');
  view.append(el('h1', t('info.title')), ecuSelect(() => load()), identBox, defBox, allUnits());

  async function load() {
    clear(identBox).append(el('h2', t('info.ident')), el('p.muted', '…'));
    clear(defBox);
    try {
      const r = await api.get(ecuPath('info'));
      clear(identBox).append(el('h2', t('info.ident')),
        el('div.kv', (r.items || []).map(it => [el('div', tr(it.name)), el('div', it.value)])),
        el('div.row', { style: { marginTop: '10px' } },
          el('button.small', { onclick: () => copyText((r.items || []).map(i => tr(i.name) + ': ' + i.value).join('\n')) }, t('common.copy'))));
      const d = r.desc;
      if (d.definition) {
        const defs = (await api.get('/api/definitions')).definitions || [];
        const def = defs.find(x => x.id === d.definition);
        if (def) {
          defBox.append(el('div.card', el('h2', t('info.definition')),
            el('div.kv',
              el('div', t('common.title')), el('div', def.name),
              el('div', 'ID'), el('div.mono', def.id),
              el('div', t('dash.status')), el('div', statusBadge(def.status)),
              def.vehicles ? [el('div', t('info.vehicles')), el('div', def.vehicles.join('; '))] : null,
              el('div', t('info.capabilities')), el('div', (d.capabilities || []).join(', '))),
            def.notes ? el('p.secondary', tr(def.notes)) : null,
            def.sources ? [el('h3', t('info.sources')), el('ul.small', def.sources.map(s => el('li', s)))] : null));
        }
      }
    } catch (e) {
      clear(identBox).append(el('h2', t('info.ident')), el('p', { style: { color: 'var(--critical-ink)' } }, errText(e)));
    }
  }
  load();
}

function allUnits() {
  const ecus = store.status.ecus || [];
  return el('div.card', el('h2', t('info.all')), el('table', el('tbody', ecus.map(e => el('tr',
    el('td.mono', e.id), el('td', el('b', e.desc.name)), el('td', e.desc.protocol), el('td.mono', e.desc.address || ''),
    el('td', statusBadge(e.desc.status)),
    el('td', el('button.small', { onclick: () => { store.ecu = e.id; location.hash = '#/dashboard'; } }, '→')))))));
}
