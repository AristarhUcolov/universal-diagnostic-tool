import { api } from '../api.js';
import { t, setLang, count, fill } from '../i18n.js';
import { el, clear, toast, busy, copyText, button, icon } from '../ui.js';
import { store, saveSettings, subscribe, checkUpdates, showNews, openLink } from '../app.js';

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
      el('div.checks', check('expert', t('set.expert')), check('logToFile', t('set.logToFile')), check('window', t('set.window')),
        check('alerts', t('set.alerts')), check('alertSound', t('set.alertSound')), check('updates', t('set.updates')))),
    el('div.card', el('h2', t('set.bridge')), el('p.secondary', t('set.bridgeLead')), bridgeBox),
    lanCard(),
    dataCard(),
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

// dataCard shows where settings, journals, recordings and reports are
// kept and opens those folders.
function dataCard() {
  const d = store.info.data || { dir: store.info.configDir, mode: 'profile' };
  const mode = { portable: 'Portable', profile: 'Profile', custom: 'Custom' }[d.mode] || 'Profile';
  return el('div.card', el('h2', t('set.folders')),
    el('div.row.data-dir', el('span.badge' + (d.mode === 'portable' ? '.good' : '.info'), t('set.data' + mode)),
      el('code.data-path', d.dir), el('button.small', { type: 'button', onclick: () => copyText(d.dir) }, icon('copy'), t('common.copy'))),
    el('p.secondary', t('set.dataLead' + mode)),
    d.note ? el('p.muted.small', d.note) : null,
    d.copied ? el('p.secondary', fill(t('set.dataCopied'), { dir: d.copied })) : null,
    el('div.row',
      [['logs', 'Logs'], ['recordings', 'Recordings'], ['reports', 'Reports'], ['definitions', 'Definitions'], ['', 'Config']].map(([k, label]) => el('button', {
        type: 'button', onclick: () => api.post('/api/open-folder', { kind: k }).catch(e => toast(e.message, 'bad')),
      }, icon('folder'), t('set.folder' + label)))));
}

function lanCard() {
  const url = store.info.lanUrl;
  return el('div.card', el('h2', t('set.lan')),
    url ? el('div.row', el('code', url), el('button.small', { type: 'button', onclick: () => copyText(url) }, icon('copy'), t('common.copy')))
      : el('p.secondary', t('set.lanHint'), ' ', el('code', 'http://<IP>:8080/#t=…')));
}

function aboutCard() {
  const i = store.info;
  const state = el('span.muted.small');
  const paint = u => {
    clear(state);
    if (!u) return;
    if (u.error) state.append(fill(t('upd.failed'), { e: u.error }));
    else if (u.available) {
      state.append(el('b', fill(t('upd.title'), { v: u.latest })), ' ',
        el('a', { href: '#', onclick: e => { e.preventDefault(); openLink(u.url); } }, t('upd.download')));
    } else if (u.latest) state.append(icon('check', 'sm'), ' ', fill(t('upd.none'), { v: u.current }));
  };
  paint(store.update);
  const check = button('refresh', t('upd.check'), null, 'small');
  check.onclick = () => busy(check, async () => paint(await checkUpdates(true)));
  return el('div.card', el('h2', t('set.about')),
    el('div.row', { style: { marginBottom: '14px' } }, button('sparkle', t('news.title'), () => showNews(false), 'small'), check, state),
    el('div.kv',
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
