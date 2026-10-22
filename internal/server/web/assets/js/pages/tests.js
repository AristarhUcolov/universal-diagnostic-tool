import { api } from '../api.js';
import { t, tr, unitName } from '../i18n.js';
import { el, clear, toast, busy, confirmBox } from '../ui.js';
import { ecuSelect, ecuPath } from './common.js';

let confirmed = false;

export function render(view) {
  const box = el('div');
  view.append(el('h1', t('tests.title')), el('p.secondary', t('tests.lead')), ecuSelect(() => load()), box);

  async function act(a, action, value, btn) {
    if (!confirmed) {
      if (!await confirmBox(t('tests.title'), t('tests.confirm'), t('common.yes'), true)) return;
      confirmed = true;
    }
    await busy(btn, async () => {
      const r = await api.post(ecuPath('actuate'), { id: a.id, action, value });
      toast(`${tr(a.name)}: ${action}${r.response ? ' → ' + r.response : ''}`, 'good');
    });
  }

  async function load() {
    clear(box).append(el('p.muted', '…'));
    try {
      const r = await api.get(ecuPath('actuators'));
      clear(box);
      const acts = r.actuators || [];
      if (!acts.length) { box.append(el('div.card.empty', t('tests.none'))); return; }
      const grid = el('div.grid.two');
      for (const a of acts) {
        const controls = el('div.row');
        if (a.kind === 'toggle') {
          const on = el('button.primary', t('tests.on'));
          const off = el('button', t('tests.off'));
          on.onclick = () => act(a, 'on', 0, on);
          off.onclick = () => act(a, 'off', 0, off);
          controls.append(on, off);
        } else if (a.kind === 'pulse') {
          const p = el('button.primary', '⚡ ' + t('tests.pulse'));
          p.onclick = () => act(a, 'pulse', 0, p);
          controls.append(p);
        } else {
          const val = el('input', { type: 'number', min: a.min, max: a.max, value: Math.round((a.min + a.max) / 2), style: { width: '110px' } });
          const set = el('button.primary', t('tests.set'));
          const rel = el('button', t('tests.release'));
          set.onclick = () => act(a, 'set', +val.value, set);
          rel.onclick = () => act(a, 'off', 0, rel);
          controls.append(val, el('span.muted', unitName(a.unit)), set, rel);
        }
        grid.append(el('div.card', el('h3', tr(a.name)), a.warning ? el('div.warnbox.small', '⚠ ', tr(a.warning)) : null,
          el('div', { style: { marginTop: '10px' } }, controls)));
      }
      box.append(grid);
    } catch (e) {
      clear(box).append(el('p', e.message));
    }
  }
  load();
}
