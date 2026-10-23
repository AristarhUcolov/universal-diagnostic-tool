import { api } from '../api.js';
import { t, tr, lang } from '../i18n.js';
import { el, clear, toast, statusBadge } from '../ui.js';
import { lookupCard } from './dtc.js';
import { repairHints, hintCard, sevBadge } from './common.js';

const obdPins = {
  en: [
    ['1', 'manufacturer (GM single-wire CAN)'], ['2', 'SAE J1850 bus + (PWM, VPW)'],
    ['3', 'manufacturer (Ford MS-CAN; Ethernet: DoIP, BMW ENET)'], ['4', 'chassis ground'], ['5', 'signal ground'],
    ['6', 'CAN high (ISO 15765-4)'], ['7', 'K-Line (ISO 9141-2, ISO 14230-4)'], ['8', 'manufacturer (Ethernet activation: DoIP, BMW ENET)'],
    ['9', 'manufacturer'], ['10', 'SAE J1850 bus − (PWM)'], ['11', 'manufacturer (Ford MS-CAN; Ethernet: DoIP, BMW ENET)'],
    ['12', 'manufacturer (Ethernet: DoIP, BMW ENET)'], ['13', 'manufacturer (Ethernet: DoIP, BMW ENET)'], ['14', 'CAN low (ISO 15765-4)'],
    ['15', 'L-Line (ISO 9141-2, ISO 14230-4)'], ['16', 'battery +12 V (permanent)'],
  ],
  ru: [
    ['1', 'на усмотрение производителя (GM: однопроводной CAN)'], ['2', 'шина SAE J1850 + (PWM, VPW)'],
    ['3', 'производителя (Ford MS-CAN; Ethernet: DoIP, BMW ENET)'], ['4', 'масса кузова'], ['5', 'сигнальная масса'],
    ['6', 'CAN high (ISO 15765-4)'], ['7', 'K-Line (ISO 9141-2, ISO 14230-4)'], ['8', 'производителя (активация Ethernet: DoIP, BMW ENET)'],
    ['9', 'производителя'], ['10', 'шина SAE J1850 − (PWM)'], ['11', 'производителя (Ford MS-CAN; Ethernet: DoIP, BMW ENET)'],
    ['12', 'производителя (Ethernet: DoIP, BMW ENET)'], ['13', 'производителя (Ethernet: DoIP, BMW ENET)'], ['14', 'CAN low (ISO 15765-4)'],
    ['15', 'L-Line (ISO 9141-2, ISO 14230-4)'], ['16', '+12 В АКБ (постоянно)'],
  ],
};

const truckPins = {
  en: '9-pin Deutsch (J1939, trucks/buses)\n  A  battery −        B  battery + (12/24 V)\n  C  J1939 CAN high   D  J1939 CAN low\n  E  CAN shield       F  J1708 +   G  J1708 −\n  H/J  manufacturer CAN (often second J1939 network)\n  Black socket: 250 kbit/s, green: 500 kbit/s\n\n6-pin Deutsch (J1708 / J1587, older trucks)\n  A  J1708 +   B  J1708 −   C  battery +   E  battery −',
  ru: '9-контактный Deutsch (J1939, грузовики/автобусы)\n  A  масса АКБ        B  + АКБ (12/24 В)\n  C  J1939 CAN high   D  J1939 CAN low\n  E  экран CAN        F  J1708 +   G  J1708 −\n  H/J  CAN производителя (часто вторая сеть J1939)\n  Чёрный разъём: 250 кбит/с, зелёный: 500 кбит/с\n\n6-контактный Deutsch (J1708 / J1587, старые грузовики)\n  A  J1708 +   B  J1708 −   C  + АКБ   E  масса',
};

export function render(view, { query } = {}) {
  const l = lang();
  const adapters = el('div', el('p.muted', '…'));
  const defs = el('div', el('p.muted', '…'));
  const advice = el('div', el('p.muted', '…'));
  view.append(
    el('h1', t('ref.title')),
    lookupCard(query && query.get('q')),
    el('div.card', el('h2', t('ref.repair')), el('p.secondary', t('ref.repairLead')), advice),
    el('div.card', el('h2', t('ref.adapters')), adapters),
    el('div.card', el('h2', t('ref.connectors')),
      el('div.grid.two',
        el('div', el('h3', 'OBD-II / EOBD (SAE J1962), 16 pin'),
          el('table.small', el('tbody', obdPins[l].map(([p, d]) => el('tr', el('td.num', p), el('td', d)))))),
        el('div', el('h3', l === 'ru' ? 'Грузовики и спецтехника' : 'Trucks and machinery'), el('div.pinout', truckPins[l])))),
    el('div.card', el('h2', t('ref.defs')), defs),
    el('div.card', el('h2', t('ref.notSupported')), el('p', t('ref.notSupportedText'))));

  repairHints().then(h => {
    clear(advice).append(...Object.values(h).map(x => el('details.advice',
      el('summary', el('b', tr(x.title)), ' ', sevBadge(x.severity)), hintCard(x))));
  }).catch(e => toast(e.message, 'bad'));

  api.get('/api/adapters').then(r => {
    clear(adapters).append(el('table', el('tbody', r.adapters.map(a => el('tr',
      el('td', el('b', tr(a.name)), a.note ? el('div.muted.small', tr(a.note)) : null),
      el('td.small', a.examples),
      el('td', a.available ? el('span.badge.good', '✓') : el('span.badge', '—')))))));
  }).catch(e => toast(e.message, 'bad'));

  api.get('/api/definitions').then(r => {
    clear(defs).append(
      el('p.secondary', t('ref.addDefs'), ' ', el('code', r.folder || '')),
      el('table', el('tbody', (r.definitions || []).map(d => el('tr',
        el('td', el('b', d.name), el('div.muted.small.mono', d.id)),
        el('td.small', (d.vehicles || []).join('; ')),
        el('td', statusBadge(d.status)),
        el('td.num', String(d.params)))))),
      (r.errors || []).length ? el('div.warnbox', r.errors.join('\n')) : null);
  }).catch(e => toast(e.message, 'bad'));
}
