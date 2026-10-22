import { t } from '../i18n.js';
import { el, supportLinks } from '../ui.js';

const repo = 'https://github.com/AristarhUcolov/universal-diagnostic-tool';

// link opens a page outside the program (the system browser when the
// program runs in its own window).
function link(cls, href, text) {
  return el('a.btn' + cls, { href, target: '_blank', rel: 'noopener' }, text);
}

export function render(view) {
  view.append(
    el('h1', t('support.title')),
    el('p.secondary.support-lead', t('support.lead')),
    el('div.grid.donate', supportLinks.map(l => el('div.card.donate-card', { style: { borderTopColor: l.color } },
      el('h2', l.name),
      el('p.secondary', t(l.key)),
      link('.primary', l.url, '♥ ' + t('support.button')),
      el('span.muted.small', l.label)))),
    el('div.card',
      el('h2', t('support.free')),
      el('ul.support-list', [1, 2, 3, 4, 5].map(i => el('li', t('support.free' + i)))),
      el('div.row',
        link('', repo + '/issues', t('support.issues')),
        link('', repo + '/blob/main/CONTRIBUTING.md', t('support.contributing')),
        link('', repo, '★ GitHub'))),
    el('p.muted', t('support.thanks')));
}
