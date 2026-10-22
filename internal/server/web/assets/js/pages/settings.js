import { api } from '../api.js';
import { t, setLang, count } from '../i18n.js';
import { el, clear, toast, busy, copyText } from '../ui.js';
import { store, saveSettings, subscribe } from '../app.js';

export function render(view) {
  const s = store.settings;
  const save = async patch => {
    try {
      await saveSettings(patch);
      toast(t('common.saved'), 'good');
    } catch (e) { toast(e.message, 'bad'); }
  };
  const select = (value, opts, onchange) => el('select', { onchange: e => onchange(e.target.value) },
    opts.map(([v, label]) => el('option', { value: v, selected: value === v }, label)));
  const check = (key, label) => el('label.check', el('input', { type: 'checkbox', checked: !!s[key], onchange: e => save({ [key]: e.target.checked }) }), label);

  const bridgeBox = el('div');
  view.append(
    el('h1', t('set.title')),
    el('div.card', el('div.fields',
      el('label.field', t('set.language'), select(s.language || '', [['', t('set.langAuto')], ['ru', 'Русский'], ['en', 'English']], v => {
        save({ language: v });
        if (v) { setLang(v); location.reload(); }
      })),
      el('label.field', t('set.theme'), select(s.theme || 'auto', [['auto', t('set.themeAuto')], ['dark', t('set.themeDark')], ['light', t('set.themeLight')]], v => save({ theme: v }))),
      el('label.field', t('set.units'), select(s.units || 'metric', [['metric', t('set.metric')], ['imperial', t('set.imperial')]], v => save({ units: v })))),
      el('div.checks', check('expert', t('set.expert')), check('logToFile', t('set.logToFile')), check('window', t('set.window')))),
    el('div.card', el('h2', t('set.bridge')), el('p.secondary', t('set.bridgeLead')), bridgeBox),
    lanCard(),
    el('div.card', el('h2', t('set.folders')), el('div.row',
      [['logs', 'Logs'], ['recordings', 'Recordings'], ['reports', 'Reports'], ['definitions', 'Definitions'], ['', 'Config']].map(([k, label]) => el('button', {
        onclick: () => api.post('/api/open-folder', { kind: k }).catch(e => toast(e.message, 'bad')),
      }, t('set.folder' + label))))),
    aboutCard());

  function paintBridge() {
    clear(bridgeBox);
    const b = store.status.bridge;
    if (b) {
      const ip = store.info.lanIp || '';
      bridgeBox.append(el('p', el('span.status-line.good', '● ' + t('set.bridgeRunning')), ' ', el('code', b.addr),
        ' · ' + count(b.clients, 'set.clients')),
        el('button.danger', { onclick: e => busy(e.target, () => api.post('/api/bridge/stop')) }, t('set.bridgeStop')));
      if (ip) bridgeBox.append(el('p.muted', 'Wi-Fi/LAN: ', el('code', ip)));
    } else {
      const addr = el('input', { value: s.bridgeAddr || '0.0.0.0:35000', size: 22 });
      const start = el('button.primary', t('set.bridgeStart'));
      start.onclick = () => busy(start, async () => {
        const r = await api.post('/api/bridge/start', { addr: addr.value.trim() });
        store.info.lanIp = r.lanIp;
        await save({ bridgeAddr: addr.value.trim() });
      });
      bridgeBox.append(el('div.row', el('label.field', t('set.bridgeAddr'), addr), start));
    }
  }
  paintBridge();
  return subscribe('status', paintBridge);
}

function lanCard() {
  const url = store.info.lanUrl;
  return el('div.card', el('h2', t('set.lan')),
    url ? el('div.row', el('code', url), el('button.small', { onclick: () => copyText(url) }, t('common.copy')))
      : el('p.secondary', t('set.lanHint'), ' ', el('code', 'http://<IP>:8080/#t=…')));
}

function aboutCard() {
  const i = store.info;
  return el('div.card', el('h2', t('set.about')), el('div.kv',
    el('div', t('about.version')), el('div', i.version),
    el('div', t('about.system')), el('div', `${i.os}/${i.arch}`),
    el('div', t('about.config')), el('div.mono', i.configDir),
    el('div', t('about.journal')), el('div.mono', i.logFile || '—'),
    el('div', t('about.definitions')), el('div', String(i.definitions)),
    el('div', t('about.dtc')), el('div', Object.entries(i.dtcFamilies || {}).map(([f, n]) => `${f}: ${n}`).join(', ')),
    el('div', t('about.license')), el('div', 'AGPL-3.0'),
    el('div', t('support.title')), el('div', el('a', { href: '#/support' }, '♥ ' + t('nav.support'))),
    el('div', t('about.source')), el('div', el('a', { href: 'https://github.com/AristarhUcolov/universal-diagnostic-tool', target: '_blank', rel: 'noopener' }, 'github.com/AristarhUcolov/universal-diagnostic-tool'))));
}
