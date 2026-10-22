import { el, fmt } from './ui.js';
import { t } from './i18n.js';

function css(name) {
  return getComputedStyle(document.documentElement).getPropertyValue(name).trim();
}

// niceScale picks a round step (1, 2, 2.5 or 5 x 10^k) giving about n
// intervals and widens [lo, hi] to whole steps.
function niceScale(lo, hi, n) {
  if (!(hi > lo)) hi = lo + 1;
  const raw = (hi - lo) / n;
  const mag = Math.pow(10, Math.floor(Math.log10(raw)));
  const step = [1, 2, 2.5, 5, 10].map(m => m * mag).find(s => s >= raw * 0.999) || raw;
  const a = Math.floor(lo / step + 1e-9) * step;
  const b = Math.ceil(hi / step - 1e-9) * step;
  const ticks = [];
  for (let i = 0; a + i * step <= b + step * 1e-6; i++) ticks.push(+(a + i * step).toFixed(10));
  return { lo: a, hi: b, step, ticks };
}

// decimals is the number of digits after the point a step needs.
function decimals(step) {
  let d = 0;
  while (d < 6 && Math.abs(Math.round(step * 10 ** d) - step * 10 ** d) > 1e-6) d++;
  return d;
}

// tickLabel prints a tick with just enough digits for the step, so two
// ticks never get the same label.
function tickLabel(v, step) {
  if (step >= 1000 && Math.abs(v) >= 1000) return (v / 1000).toFixed(decimals(step / 1000)) + 'K';
  return v.toFixed(decimals(step));
}

// StripChart plots one parameter over the last windowSec seconds. Each
// parameter gets its own chart and its own axis (no dual axes).
export class StripChart {
  constructor({ title, unit, min, max, digits, windowSec = 60 }) {
    this.title = title; this.unit = unit || ''; this.digits = digits ?? 2;
    this.hint = [min, max];
    this.windowMs = windowSec * 1000;
    this.points = [];
    this.canvas = el('canvas', { role: 'img', 'aria-label': title });
    this.valueEl = el('span.cval', '—');
    this.tip = el('div.tooltip.hidden');
    this.root = el('div.card.chart', el('div.chead', el('span', title), el('span', this.valueEl, ' ', el('span.muted.small', this.unit))), this.canvas, this.tip);
    this.hover = null;
    this.canvas.addEventListener('mousemove', e => { this.hover = e.offsetX; this.draw(); });
    this.canvas.addEventListener('mouseleave', () => { this.hover = null; this.tip.classList.add('hidden'); this.draw(); });
  }

  push(t, v) {
    this.points.push([t, v]);
    const cut = t - this.windowMs - 2000;
    while (this.points.length && this.points[0][0] < cut) this.points.shift();
    this.valueEl.textContent = fmt(v, this.digits);
    this.schedule();
  }

  schedule() {
    if (this.pending) return;
    this.pending = true;
    requestAnimationFrame(() => { this.pending = false; this.draw(); });
  }

  draw() {
    const c = this.canvas;
    const dpr = window.devicePixelRatio || 1;
    const w = c.clientWidth, h = c.clientHeight;
    if (!w || !h) return;
    if (c.width !== Math.round(w * dpr) || c.height !== Math.round(h * dpr)) {
      c.width = Math.round(w * dpr); c.height = Math.round(h * dpr);
    }
    const g = c.getContext('2d');
    g.setTransform(dpr, 0, 0, dpr, 0, 0);
    g.clearRect(0, 0, w, h);
    const R = 10, T = 8, B = 18;
    const now = this.points.length ? this.points[this.points.length - 1][0] : Date.now();
    const t0 = now - this.windowMs;
    const vis = this.points.filter(p => p[0] >= t0);
    let lo = Infinity, hi = -Infinity;
    for (const [, v] of vis) { lo = Math.min(lo, v); hi = Math.max(hi, v); }
    const [hmin, hmax] = this.hint;
    const hinted = Number.isFinite(hmin) && Number.isFinite(hmax) && hmax > hmin;
    if (!Number.isFinite(lo)) {
      [lo, hi] = hinted ? [hmin, hmax] : [0, 1];
    } else if (hinted && lo >= hmin && hi <= hmax && hi - lo > (hmax - hmin) * 0.3) {
      lo = hmin; hi = hmax;
    } else {
      // Fit the data, but keep a minimum span so sensor noise on a steady
      // value is not blown up to full height.
      const dataLo = lo, dataHi = hi;
      const span = Math.max(hi - lo, hinted ? (hmax - hmin) * 0.05 : Math.max(Math.abs(hi) * 0.02, 0.01));
      const mid = (hi + lo) / 2;
      lo = mid - span * 0.6; hi = mid + span * 0.6;
      if (hinted && lo < hmin && dataLo >= hmin) { hi += hmin - lo; lo = hmin; }
      if (hinted && hi > hmax && dataHi <= hmax) { lo -= hi - hmax; hi = hmax; }
    }
    const scale = niceScale(lo, hi, 4);
    lo = scale.lo; hi = scale.hi;
    const ticks = scale.ticks;
    const labels = ticks.map(v => tickLabel(v, scale.step));
    g.font = '11px system-ui, sans-serif';
    const L = Math.max(30, ...labels.map(s => g.measureText(s).width)) + 10;
    const pw = w - L - R, ph = h - T - B;
    const x = t => L + (t - t0) / this.windowMs * pw;
    const y = v => T + ph - (v - lo) / (hi - lo) * ph;
    // Recessive hairline grid and labels in text tokens.
    g.lineWidth = 1;
    g.strokeStyle = css('--grid');
    g.fillStyle = css('--text-muted');
    g.textAlign = 'right'; g.textBaseline = 'middle';
    ticks.forEach((tv, i) => {
      const yy = Math.round(y(tv)) + 0.5;
      g.beginPath(); g.moveTo(L, yy); g.lineTo(L + pw, yy); g.stroke();
      g.fillText(labels[i], L - 6, yy);
    });
    g.textAlign = 'center'; g.textBaseline = 'top';
    const secs = this.windowMs / 1000;
    for (const s of [secs, secs / 2, 0]) {
      const label = s === 0 ? t('chart.now') : '−' + s + ' ' + t('chart.sec');
      const half = g.measureText(label).width / 2;
      g.fillText(label, Math.min(Math.max(x(now - s * 1000), L + half), L + pw - half), T + ph + 4);
    }
    if (vis.length < 2) return;
    const series = css('--series-1');
    // Area wash, then the 2px line.
    g.beginPath();
    g.moveTo(x(vis[0][0]), y(vis[0][1]));
    for (const [t, v] of vis) g.lineTo(x(t), y(v));
    g.lineTo(x(vis[vis.length - 1][0]), T + ph);
    g.lineTo(x(vis[0][0]), T + ph);
    g.closePath();
    g.fillStyle = css('--series-1-wash');
    g.fill();
    g.beginPath();
    g.lineWidth = 2; g.lineJoin = 'round'; g.lineCap = 'round';
    g.strokeStyle = series;
    vis.forEach(([t, v], i) => (i ? g.lineTo(x(t), y(v)) : g.moveTo(x(t), y(v))));
    g.stroke();
    // End marker with a surface ring.
    const [lt, lv] = vis[vis.length - 1];
    g.beginPath(); g.arc(x(lt), y(lv), 5, 0, Math.PI * 2); g.fillStyle = css('--surface-1'); g.fill();
    g.beginPath(); g.arc(x(lt), y(lv), 3.5, 0, Math.PI * 2); g.fillStyle = series; g.fill();
    // Hover crosshair and tooltip.
    if (this.hover !== null && this.hover >= L) {
      let best = vis[0];
      for (const p of vis) if (Math.abs(x(p[0]) - this.hover) < Math.abs(x(best[0]) - this.hover)) best = p;
      const bx = Math.round(x(best[0])) + 0.5;
      g.strokeStyle = css('--text-muted'); g.lineWidth = 1;
      g.beginPath(); g.moveTo(bx, T); g.lineTo(bx, T + ph); g.stroke();
      g.beginPath(); g.arc(bx, y(best[1]), 5, 0, Math.PI * 2); g.fillStyle = css('--surface-1'); g.fill();
      g.beginPath(); g.arc(bx, y(best[1]), 3.5, 0, Math.PI * 2); g.fillStyle = series; g.fill();
      const ago = ((now - best[0]) / 1000).toFixed(1);
      this.tip.textContent = `${fmt(best[1], this.digits)} ${this.unit} · −${ago} ${t('chart.sec')}`;
      this.tip.classList.remove('hidden');
      const left = Math.min(bx + 10, w - this.tip.offsetWidth - 4);
      this.tip.style.left = left + 'px';
      this.tip.style.top = (this.canvas.offsetTop + 4) + 'px';
    }
  }
}

// sparkline draws a small trend line into a canvas.
export function sparkline(canvas, values) {
  const dpr = window.devicePixelRatio || 1;
  const w = canvas.clientWidth, h = canvas.clientHeight;
  if (!w || !h || values.length < 2) return;
  canvas.width = Math.round(w * dpr); canvas.height = Math.round(h * dpr);
  const g = canvas.getContext('2d');
  g.setTransform(dpr, 0, 0, dpr, 0, 0);
  g.clearRect(0, 0, w, h);
  let lo = Math.min(...values), hi = Math.max(...values);
  if (hi - lo < 1e-9) { lo -= 1; hi += 1; }
  const x = i => 2 + i / (values.length - 1) * (w - 6);
  const y = v => 3 + (h - 6) * (1 - (v - lo) / (hi - lo));
  g.beginPath();
  values.forEach((v, i) => (i ? g.lineTo(x(i), y(v)) : g.moveTo(x(i), y(v))));
  g.strokeStyle = css('--text-muted'); g.lineWidth = 1.5; g.lineJoin = 'round';
  g.stroke();
  const last = values[values.length - 1];
  g.beginPath(); g.arc(x(values.length - 1), y(last), 3, 0, Math.PI * 2);
  g.fillStyle = css('--series-1'); g.fill();
}

// meter renders a bar showing value within [min, max]; severity from
// limits {warn, bad} (above) or {warnBelow, badBelow}.
export function meter(value, min, max, limits = {}) {
  const pct = Math.max(0, Math.min(100, (value - min) / ((max - min) || 1) * 100));
  let cls = '';
  if ((limits.bad !== undefined && value >= limits.bad) || (limits.badBelow !== undefined && value <= limits.badBelow)) cls = '.bad';
  else if ((limits.warn !== undefined && value >= limits.warn) || (limits.warnBelow !== undefined && value <= limits.warnBelow)) cls = '.warn';
  return el('div.meter' + cls, el('div', { style: { width: pct + '%' } }));
}
