import { api } from '../api.js';
import { t } from '../i18n.js';
import { el, busy } from '../ui.js';
import { currentECU } from '../app.js';
import { ecuSelect, ecuPath } from './common.js';

const history = [];

const quick = {
  kwp: [['1A 80', 'ECU identification'], ['21 01', 'data record 01'], ['18 00 FF 00', 'read DTC'], ['3E 01', 'tester present'], ['10 81', 'default session']],
  uds: [['22 F1 90', 'VIN'], ['22 F1 87', 'part number'], ['19 02 FF', 'read DTC'], ['10 03', 'extended session'], ['3E 00', 'tester present']],
  obd: [['01 00', 'supported PIDs'], ['01 0C', 'engine speed'], ['03', 'stored DTC'], ['09 02', 'VIN'], ['01 01', 'monitor status']],
  legacy: [['01', 'version'], ['61 1A 29', 'coolant + rpm'], ['02', 'faults'], ['51', 'program passport']],
  kw1281: [['29 01', 'group 1'], ['29 02', 'group 2'], ['07', 'read faults'], ['09', 'ACK']],
  j1939: [['00 FE CA', 'DM1 (PGN 65226)'], ['00 FE EC', 'VIN (PGN 65260)'], ['00 FE E5', 'engine hours'], ['00 FE EB', 'component id']],
};

function kindOf(desc) {
  const p = (desc.protocol || '').toLowerCase();
  if (desc.id === 'obd') return 'obd';
  if (desc.id === 'j1939') return 'j1939';
  if (p.includes('legacy')) return 'legacy';
  if (p.includes('kw1281')) return 'kw1281';
  if (p.includes('uds') || p.includes('15765') || p.includes('doip') || p.includes('hsfz')) return 'uds';
  return 'kwp';
}

export function render(view) {
  const log = el('div.term-log');
  const input = el('input', { placeholder: t('term.placeholder'), style: { flex: '1', fontFamily: 'var(--mono)' } });
  const sendBtn = el('button.primary', t('common.send'));
  const e = currentECU();
  const quickBox = el('div.row', (quick[kindOf(e.desc)] || []).map(([req, label]) =>
    el('button.small', { title: label, onclick: () => { input.value = req; send(); } }, req)));
  let hpos = history.length;

  view.append(el('h1', t('term.title')), el('p.secondary', t('term.lead')), ecuSelect(() => {}),
    el('div.card', el('div.row', input, sendBtn), el('div', { style: { margin: '10px 0' } }, el('span.muted.small', t('term.quick') + ': '), quickBox), log));

  function line(cls, text) {
    log.append(el('div' + (cls ? '.' + cls : ''), text));
    log.scrollTop = log.scrollHeight;
  }

  async function send() {
    const hex = input.value.trim();
    if (!hex) return;
    history.push(hex);
    hpos = history.length;
    await busy(sendBtn, async () => {
      line('req', '> ' + hex);
      try {
        const r = await api.post(ecuPath('raw'), { hex });
        for (const a of r.answers || []) {
          line('', '< ' + (a.from ? '[' + a.from + '] ' : '') + a.hex);
          line('note', '  ' + a.text + (a.ascii && /[A-Za-z0-9]{3}/.test(a.ascii) ? '  "' + a.ascii + '"' : ''));
        }
        if (r.error) line('err', '! ' + r.error);
        if (!r.answers && !r.error) line('note', '  (no answer)');
      } catch (err) {
        line('err', '! ' + err.message);
      }
    });
    input.select();
  }
  sendBtn.onclick = send;
  input.onkeydown = ev => {
    if (ev.key === 'Enter') send();
    if (ev.key === 'ArrowUp' && hpos > 0) { hpos--; input.value = history[hpos]; ev.preventDefault(); }
    if (ev.key === 'ArrowDown' && hpos < history.length) { hpos++; input.value = history[hpos] || ''; ev.preventDefault(); }
  };
  input.focus();
}
