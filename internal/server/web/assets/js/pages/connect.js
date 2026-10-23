import { api } from '../api.js';
import { t, tr } from '../i18n.js';
import { el, clear, toast, busy, statusBadge, errText } from '../ui.js';
import { store, subscribe, connected } from '../app.js';

const portKinds = { kkl: 'serial', elm327: 'serial', slcan: 'serial' };

export function render(view) {
  const cfg = Object.assign({ adapter: 'sim', profile: 'ru-auto' }, store.settings.last || {});
  let data = null;
  let ports = [];
  let defs = [];

  const statusBox = el('div');
  const quickBox = el('div');
  const adaptersBox = el('div.adapters');
  const portBox = el('div');
  const profileBox = el('div');
  const advBox = el('div.fields');
  const goBtn = el('button.primary.big', t('connect.go'));

  view.append(
    el('h1', t('connect.title')),
    el('p.secondary', t('connect.lead')),
    statusBox,
    quickBox,
    el('div.card.steps',
      el('div.step-title', t('connect.adapter')), adaptersBox,
      el('div.step-title', t('connect.port')), portBox,
      el('div.step-title', t('connect.profile')), profileBox,
      el('details', el('summary', t('connect.advanced')), advBox),
      el('div.row', { style: { marginTop: '16px' } }, goBtn)),
    el('div.grid.two', { style: { marginTop: '14px' } }, tipsCard(), wiringCard()),
  );

  goBtn.onclick = () => busy(goBtn, async () => {
    const body = { ...cfg };
    for (const k of Object.keys(body)) if (body[k] === '' || body[k] === null) delete body[k];
    await api.post('/api/connect', body);
  });

  function renderStatus() {
    const s = store.status;
    clear(statusBox);
    if (s.state === 'connected') {
      const ecus = s.ecus || [];
      statusBox.append(el('div.card',
        el('div.row', el('span.status-line.good', '✓ ' + t('connect.connected')), el('span.secondary', s.adapter || '')),
        el('h3', t('connect.foundUnits')),
        el('ul', ecus.map(e => el('li', el('b', e.desc.name), ' — ', e.desc.protocol, ' ', statusBadge(e.desc.status)))),
        el('div.row', el('button.primary', { onclick: () => { location.hash = '#/dashboard'; } }, t('nav.dashboard') + ' →'),
          el('button', { onclick: () => api.post('/api/disconnect').catch(e => toast(e.message, 'bad')) }, t('connect.disconnect')))));
    } else if (s.state === 'connecting' || s.state === 'error') {
      const log = el('div.progress-log', (s.progress || []).map(p => el('div', p)));
      statusBox.append(el('div.card',
        el('div.row', s.state === 'connecting'
          ? el('span.status-line', '⏳ ' + t('conn.connecting'))
          : el('span.status-line.bad', '✕ ' + t('conn.error')),
        s.state === 'connecting' ? el('button.small', { onclick: () => api.post('/api/disconnect') }, t('common.cancel')) : null),
        s.error ? el('p', { style: { color: 'var(--critical-ink)' } }, errText({ message: s.error, code: s.errorCode })) : null,
        el('h3', t('connect.progress')), log));
      log.scrollTop = log.scrollHeight;
    }
    goBtn.disabled = s.state === 'connecting';
    paintQuick();
  }

  // paintQuick offers the last connection with one button, so a regular
  // user does not have to go through the form again.
  function paintQuick() {
    clear(quickBox);
    const last = store.settings.last;
    if (!data || !last || !last.adapter || ['connected', 'connecting'].includes(store.status.state)) return;
    const a = data.adapters.find(x => x.id === last.adapter);
    if (!a) return;
    const p = (data.profiles || []).find(x => x.id === last.profile);
    const parts = [tr(a.name), last.port || '', p ? tr(p.name) : '', last.simVariant || ''].filter(Boolean);
    const b = el('button.primary', '▶ ' + t('connect.go'));
    b.onclick = () => busy(b, () => api.post('/api/connect',
      Object.fromEntries(Object.entries(last).filter(([, v]) => v !== '' && v !== null && v !== undefined))));
    quickBox.append(el('div.card.quick',
      el('div', el('b', t('connect.quickTitle')), el('div.secondary.small', parts.join(' · '))), b));
  }

  function renderAdapters() {
    clear(adaptersBox);
    for (const a of data.adapters) {
      const b = el('button.adapter' + (cfg.adapter === a.id ? '.selected' : '') + (a.available ? '' : '.unavailable'),
        { type: 'button', onclick: () => { cfg.adapter = a.id; cfg.port = ''; renderAll(); } },
        el('b', tr(a.name)),
        el('span.ex', a.examples),
        a.available ? null : el('span.badge.warn', t('connect.unavailable')));
      adaptersBox.append(b);
    }
  }

  function adapterInfo() { return data.adapters.find(a => a.id === cfg.adapter) || data.adapters[0]; }

  function renderPort() {
    clear(portBox);
    const a = adapterInfo();
    const kind = portKinds[a.id] || a.port;
    const set = v => { cfg.port = v; };
    if (kind === 'serial') {
      const list = el('datalist#portlist', ports.map(p => el('option', { value: p.name }, [p.description, p.chip].filter(Boolean).join(' — '))));
      const inp = el('input', { list: 'portlist', value: cfg.port || (ports[0] ? ports[0].name : ''), placeholder: 'COM3 / /dev/ttyUSB0', oninput: e => set(e.target.value) });
      set(inp.value);
      portBox.append(el('div.row', inp, list,
        el('button.small', { onclick: async () => { await loadPorts(); renderPort(); } }, t('connect.refreshPorts'))));
      if (!ports.length) {
        portBox.append(el('p.muted.small', t('connect.noPorts')));
      } else {
        portBox.append(el('div.table-wrap', el('table', el('tbody', ports.map(p => el('tr', { style: { cursor: 'pointer' }, onclick: () => { inp.value = p.name; set(p.name); } },
          el('td', el('b', p.name)), el('td', p.description || ''), el('td.muted', [p.chip, p.hint].filter(Boolean).join(' · '))))))));
      }
    } else if (kind === 'network') {
      const ph = { 'elm327-wifi': '192.168.0.10:35000', doip: 'auto (UDP discovery) / 169.254.x.x', hsfz: '169.254.x.x' }[a.id] || '';
      const inp = el('input', { value: cfg.port || '', placeholder: ph, size: 30, oninput: e => set(e.target.value) });
      portBox.append(el('label.field', t('connect.network'), inp));
    } else if (kind === 'ble') {
      const inp = el('input', { value: cfg.port || '', placeholder: 'AA:BB:CC:DD:EE:FF', size: 22, oninput: e => set(e.target.value.trim()) });
      const list = el('div');
      const find = el('button.small', '⌕ ' + t('connect.bleFind'));
      find.onclick = () => busy(find, async () => {
        clear(list).append(el('p.muted.small', t('connect.bleScanning')));
        let devs = [];
        try {
          devs = (await api.get('/api/ble/scan?seconds=6')).devices || [];
        } finally {
          clear(list);
        }
        if (!devs.length) {
          list.append(el('p.muted.small', t('connect.bleNone')));
          return;
        }
        list.append(el('div.table-wrap', el('table', el('tbody', devs.map(d => el('tr', {
          style: { cursor: 'pointer' }, onclick: () => { inp.value = d.address; set(d.address); },
        }, el('td', el('b', d.name || '—')), el('td.mono', d.address), el('td.muted', d.rssi + ' dBm'),
        el('td', d.likely ? el('span.badge.good', 'OBD') : null)))))));
      });
      portBox.append(el('div.row', el('label.field', t('connect.bleAddr'), inp), find), list);
    } else if (kind === 'j2534') {
      if (!data.j2534 || !data.j2534.length) {
        portBox.append(el('p.muted', t('connect.noJ2534')));
      } else {
        const sel = el('select', { onchange: e => set(e.target.value) }, data.j2534.map(d =>
          el('option', { value: d.name, selected: cfg.port === d.name, disabled: !d.usable }, `${d.name} (${d.vendor}, ${d.arch})${d.usable ? '' : ' — ' + d.reason}`)));
        set(sel.value);
        portBox.append(el('label.field', t('connect.j2534'), sel));
      }
    } else if (kind === 'can') {
      const sel = el('select', { onchange: e => set(e.target.value) }, (data.socketcan || ['can0']).map(i => el('option', { value: i, selected: cfg.port === i }, i)));
      set(sel.value);
      portBox.append(el('label.field', t('connect.canIface'), sel));
    } else if (kind === 'pcan') {
      const sel = el('select', { onchange: e => set(e.target.value) }, data.pcan.map(i => el('option', { value: i, selected: cfg.port === i }, i)));
      set(sel.value);
      portBox.append(el('label.field', t('connect.channel'), sel));
    } else if (kind === 'kvaser') {
      portBox.append(el('label.field', t('connect.channel'), el('input', { type: 'number', min: 0, value: cfg.port || '0', oninput: e => set(e.target.value) })));
      set(cfg.port || '0');
    } else if (kind === 'rp1210') {
      const opts = (data.rp1210 || []).map(r => el('option', { value: `${r.implementation}:${r.deviceId}` }, `${r.description} (${r.vendor})`));
      const sel = el('select', { onchange: e => set(e.target.value) }, opts);
      set(sel.value);
      portBox.append(opts.length ? sel : el('p.muted', 'RP1210: —'));
    } else {
      const sel = el('select', { onchange: e => { cfg.simVariant = e.target.value; } },
        el('option', { value: '' }, t('connect.ecuAuto')),
        data.simVariants.map(v => el('option', { value: v.id, selected: cfg.simVariant === v.id }, tr(v.name))));
      portBox.append(el('label.field', t('connect.simVariant'), sel));
    }
  }

  function renderProfile() {
    clear(profileBox);
    const a = adapterInfo();
    const allowed = data.profiles.filter(p => a.profiles.includes(p.id));
    if (!allowed.find(p => p.id === cfg.profile)) cfg.profile = allowed[0].id;
    const hint = el('p.muted.small');
    const showHint = () => { hint.textContent = tr((allowed.find(p => p.id === cfg.profile) || {}).hint); };
    const sel = el('select', { onchange: e => { cfg.profile = e.target.value; showHint(); renderAdvanced(); } },
      allowed.map(p => el('option', { value: p.id, selected: cfg.profile === p.id }, tr(p.name))));
    showHint();
    profileBox.append(sel, hint);
  }

  function field(label, input, hint) {
    return el('label.field', label, input, hint ? el('span.muted.small', hint) : null);
  }

  function renderAdvanced() {
    clear(advBox);
    const a = cfg.adapter;
    const kline = ['kkl', 'elm327', 'elm327-wifi', 'j2534', 'sim'].includes(a);
    const canAd = ['slcan', 'socketcan', 'pcan', 'kvaser', 'j2534', 'sim'].includes(a);
    const text = (key, ph) => el('input', { value: cfg[key] || '', placeholder: ph, oninput: e => { cfg[key] = e.target.value.trim(); } });
    if (kline || cfg.profile === 'vag-tp20' || cfg.profile === 'uds') {
      advBox.append(field(t('connect.address'), text('address', ''), t('connect.addressHint')));
    }
    if (kline) {
      advBox.append(field(t('connect.init'), el('select', { onchange: e => { cfg.init = e.target.value; } },
        ['', 'fast', 'slow'].map(v => el('option', { value: v, selected: (cfg.init || '') === v }, t(v ? 'connect.init' + v[0].toUpperCase() + v.slice(1) : 'connect.initAuto'))))));
    }
    if (canAd) {
      advBox.append(field(t('connect.bitrate'), el('select', { onchange: e => { cfg.bitrate = +e.target.value || 0; } },
        [0, 1000000, 500000, 250000, 125000, 100000, 83333, 50000].map(v => el('option', { value: v, selected: (cfg.bitrate || 0) === v }, v ? (v / 1000) + ' kbit/s' : t('connect.bitrateAuto'))))));
    }
    if (cfg.profile === 'uds') {
      advBox.append(field(t('connect.txid'), text('txId', '7E0')), field(t('connect.rxid'), text('rxId', '7E8')));
      // CAN FD: the data phase rates each adapter can do.
      const rates = { slcan: [2000000, 5000000], pcan: [1000000, 2000000, 4000000, 5000000, 8000000], sim: [2000000] }[a];
      if (rates || a === 'socketcan') {
        const dr = rates ? el('select', { disabled: !cfg.canFd, onchange: e => { cfg.dataBitrate = +e.target.value; } },
          rates.map(v => el('option', { value: v, selected: (cfg.dataBitrate || 2000000) === v }, (v / 1e6) + ' Mbit/s'))) : null;
        advBox.append(
          el('label.check', el('input', { type: 'checkbox', checked: !!cfg.canFd, onchange: e => { cfg.canFd = e.target.checked; if (dr) dr.disabled = !cfg.canFd; } }), t('connect.canFd')),
          dr ? field(t('connect.dataBitrate'), dr, t('connect.canFdHint')) : el('span.muted.small', t('connect.canFdSocket')));
      }
    }
    advBox.append(field(t('connect.ecu'), el('select', { onchange: e => { cfg.ecu = e.target.value; } },
      el('option', { value: '' }, t('connect.ecuAuto')),
      defs.map(d => el('option', { value: d.id, selected: cfg.ecu === d.id }, `${d.name} [${t('status.' + d.status)}]`)))));
    if (['elm327', 'slcan', 'kkl'].includes(a)) {
      advBox.append(field(t('connect.baud'), el('select', { onchange: e => { cfg.baud = +e.target.value || 0; } },
        [0, 9600, 10400, 38400, 57600, 115200, 230400, 500000].map(v => el('option', { value: v, selected: (cfg.baud || 0) === v }, v || t('connect.baudAuto'))))));
    }
    if (a === 'kkl') {
      advBox.append(
        field(t('connect.echo'), el('select', { onchange: e => { cfg.echo = e.target.value; } },
          ['', 'on', 'off'].map(v => el('option', { value: v, selected: (cfg.echo || '') === v }, v || t('connect.echoAuto'))))),
        field(t('connect.initMode'), el('select', { onchange: e => { cfg.initMode = e.target.value; } },
          ['break', 'baud360'].map(v => el('option', { value: v, selected: (cfg.initMode || 'break') === v }, v === 'break' ? 'break (TxD low)' : '0x00 @ 360 baud')))),
        el('label.check', el('input', { type: 'checkbox', checked: cfg.dtr !== false, onchange: e => { cfg.dtr = e.target.checked; } }), t('connect.dtr')),
        el('label.check', el('input', { type: 'checkbox', checked: !!cfg.rts, onchange: e => { cfg.rts = e.target.checked; } }), t('connect.rts')));
    }
  }

  function renderAll() {
    if (!data) return;
    renderAdapters();
    renderPort();
    renderProfile();
    renderAdvanced();
  }

  async function loadPorts() {
    try { ports = await api.get('/api/ports'); } catch (e) { ports = []; }
  }

  renderStatus();
  const unsub = subscribe('status', renderStatus);
  (async () => {
    try {
      [data] = await Promise.all([api.get('/api/adapters'), loadPorts()]);
      defs = (await api.get('/api/definitions')).definitions || [];
      renderAll();
      paintQuick();
    } catch (e) {
      toast(e.message, 'bad');
    }
  })();
  return unsub;
}

function tipsCard() {
  return el('div.card',
    el('h2', t('connect.tips')),
    el('ol', ['connect.tip1', 'connect.tip2', 'connect.tip3', 'connect.tip4', 'connect.tip5'].map(k => el('li', t(k)))));
}

function wiringCard() {
  const ru = document.documentElement.lang === 'ru';
  const lines = ru ? [
    'Колодка 12 pin (ВАЗ/ГАЗ/УАЗ)        OBD-II (адаптер, 16 pin)',
    '  A  масса  ─────────────────────►  4 и 5  (масса)',
    '  M  K-Line ─────────────────────►  7      (K-Line)',
    '  +12 В в колодке обычно нет ────►  16     (+12 В от АКБ',
    '                                           через предохранитель 1 А)',
    '  остальные контакты не подключать',
  ] : [
    '12-pin socket (VAZ/GAZ/UAZ)          OBD-II (adapter, 16 pin)',
    '  A  ground ─────────────────────►  4 and 5 (ground)',
    '  M  K-Line ─────────────────────►  7       (K-Line)',
    '  +12 V: usually not here ───────►  16      (+12 V from the',
    '                                            battery via a 1 A fuse)',
    '  leave the other contacts unconnected',
  ];
  return el('div.card',
    el('h2', t('connect.wiring')),
    el('div.pinout', lines.join('\n')),
    el('p.muted.small', ru
      ? 'Это типичная разводка, а не стандарт: у разных годов и блоков бывает иначе. До подключения кабеля прозвоните контакты мультиметром при выключенном зажигании: масса — на кузов, K-Line — до вывода ЭБУ (у Микас-7 — 55-й). С включённым зажиганием K-Line в покое показывает почти напряжение бортсети. Подробно — docs/GAZ-VOLGA.md.'
      : 'This is the usual wiring, not a standard: it differs between years and control units. Before connecting the cable, check the contacts with a multimeter, ignition off: ground to the body, K-Line to the ECU pin (pin 55 on a Микас-7). With the ignition on, an idle K-Line reads close to battery voltage. Details: docs/GAZ-VOLGA.md.'));
}
