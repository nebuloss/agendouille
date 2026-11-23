import { h } from '@/shared/core/Dom.ts';

/**
 * The calendar stats page's two charts, drawn as SVG and DOM, on the shared
 * design's series colours (theme.css: --color-series-1..3 for created,
 * changed, deleted, in that fixed order, validated together light and dark;
 * the green is under 3:1 on the light surface, so a legend with totals and a
 * table of the figures are always there, as the dataviz method requires).
 */

export interface DayCount {
  day: string;
  created: number;
  changed: number;
  deleted: number;
}

const NS = 'http://www.w3.org/2000/svg';
const SERIES = [
  { key: 'created', label: 'Créés', color: 'var(--color-series-1)' },
  { key: 'changed', label: 'Modifiés', color: 'var(--color-series-2)' },
  { key: 'deleted', label: 'Supprimés', color: 'var(--color-series-3)' },
] as const;

function svg<K extends keyof SVGElementTagNameMap>(tag: K, attrs: Record<string, string | number>): SVGElementTagNameMap[K] {
  const el = document.createElementNS(NS, tag);
  for (const [k, v] of Object.entries(attrs)) el.setAttribute(k, String(v));
  return el;
}

const dayLabel = (iso: string): string =>
  new Date(`${iso}T12:00:00Z`).toLocaleDateString('fr-FR', { day: 'numeric', month: 'short', timeZone: 'UTC' });

/** Changes per day, stacked (created, changed, deleted), with a legend, a hover tooltip and a table. */
export function dailyChart(days: DayCount[]): HTMLElement {
  const totals = SERIES.map((s) => days.reduce((n, d) => n + d[s.key], 0));
  const legend = h('div', { class: 'flex flex-wrap items-center gap-x-5 gap-y-1 text-sm' },
    ...SERIES.map((s, i) => {
      const dot = h('span', { class: 'inline-block size-3 rounded-[3px]', 'aria-hidden': 'true' });
      dot.style.setProperty('background-color', s.color);
      return h('span', { class: 'inline-flex items-center gap-2 text-on-surface-variant' }, dot, s.label,
        h('b', { class: 'font-semibold tabular-nums text-on-surface' }, String(totals[i])));
    }));
  const max = Math.max(1, ...days.map((d) => d.created + d.changed + d.deleted));
  const W = 900, H = 180, top = 8, bottom = 22, left = 30;
  const plotH = H - top - bottom;
  const step = (W - left) / Math.max(1, days.length);
  const barW = Math.max(2, step - 2); // a 2px gap between bars
  const chart = svg('svg', { viewBox: `0 0 ${W} ${H}`, class: 'block h-auto w-full', role: 'img',
    'aria-label': `Modifications par jour sur ${days.length} jours : ${totals.join(', ')} (créés, modifiés, supprimés)` });
  // Recessive grid: the baseline and the top value only.
  for (const [v, y] of [[0, top + plotH], [max, top]] as const) {
    chart.append(svg('line', { x1: left, x2: W, y1: y, y2: y, stroke: 'var(--color-outline-variant)', 'stroke-width': v === 0 ? 1 : 0.5, 'stroke-dasharray': v === 0 ? '' : '3 4' }));
    const t = svg('text', { x: left - 6, y: y + 3, 'text-anchor': 'end', 'font-size': 10, fill: 'var(--color-on-surface-variant)' });
    t.textContent = String(v);
    chart.append(t);
  }
  const tip = h('div', { class: 'pointer-events-none absolute z-10 hidden rounded-md3-s bg-inverse px-3 py-2 text-xs whitespace-nowrap elevation-2', role: 'tooltip' });
  tip.style.setProperty('background-color', 'var(--color-on-surface)');
  tip.style.setProperty('color', 'var(--color-surface)');
  const wrap = h('div', { class: 'relative' }, chart, tip);
  days.forEach((d, i) => {
    const x = left + i * step + 1;
    let y = top + plotH;
    SERIES.forEach((s, k) => {
      const v = d[s.key];
      if (v === 0) return;
      const hgt = Math.max(2, (v / max) * plotH);
      const gap = k > 0 && y < top + plotH ? 2 : 0; // a 2px surface gap between stacked segments
      y -= hgt;
      chart.append(svg('rect', { x, y: y - gap, width: barW, height: Math.max(1, hgt - gap), rx: 1.5, fill: s.color }));
    });
    if (i % 15 === 0 || i === days.length - 1) {
      const last = i === days.length - 1;
      const t = svg('text', { x: last ? x + barW : x + barW / 2, y: H - 6, 'text-anchor': last ? 'end' : i === 0 ? 'start' : 'middle', 'font-size': 10, fill: 'var(--color-on-surface-variant)' });
      t.textContent = dayLabel(d.day);
      chart.append(t);
    }
    // The hit target is the whole column, bigger than the mark.
    const hit = svg('rect', { x: x - 1, y: top, width: step, height: plotH, fill: 'transparent' });
    hit.addEventListener('pointerenter', () => {
      tip.replaceChildren(h('b', { class: 'block pb-1' }, dayLabel(d.day)),
        ...SERIES.map((s) => h('span', { class: 'block tabular-nums' }, `${s.label} : ${d[s.key]}`)));
      tip.classList.remove('hidden');
      const pct = ((x - 1 + step / 2) / W) * 100;
      tip.style.setProperty('left', `${Math.min(80, Math.max(0, pct - 8))}%`);
      tip.style.setProperty('top', '0');
    });
    hit.addEventListener('pointerleave', () => tip.classList.add('hidden'));
    chart.append(hit);
  });
  const busy = days.filter((d) => d.created + d.changed + d.deleted > 0);
  const table = h('details', { class: 'text-sm' }, h('summary', { class: 'cursor-pointer text-primary' }, 'Voir les chiffres'),
    busy.length === 0 ? h('p', { class: 'py-2 text-on-surface-variant' }, 'Aucune modification sur la période.')
      : h('div', { class: 'mt-2 max-h-64 overflow-auto rounded-md3-m bg-surface-lowest' },
        h('table', { class: 'w-full text-sm' },
          h('thead', {}, h('tr', {}, ...['Jour', ...SERIES.map((s) => s.label)].map((c) => h('th', { class: 'px-3 py-2 text-left text-[11px] font-semibold uppercase text-on-surface-variant' }, c)))),
          h('tbody', {}, ...busy.reverse().map((d) => h('tr', { class: 'border-t border-outline-variant/50' },
            h('td', { class: 'px-3 py-1.5' }, dayLabel(d.day)), ...SERIES.map((s) => h('td', { class: 'px-3 py-1.5 tabular-nums' }, String(d[s.key]))))))))) ;
  return h('div', { class: 'flex flex-col gap-3' }, legend, wrap, table);
}

/** Labelled horizontal bars of one measure (one hue: the title names it). */
export function barList(rows: { label: string; value: number; sub?: string; href?: string }[], empty: string): HTMLElement {
  if (rows.length === 0) return h('p', { class: 'py-3 text-sm text-on-surface-variant' }, empty);
  const max = Math.max(1, ...rows.map((r) => r.value));
  return h('ul', { class: 'm-0 flex list-none flex-col gap-3 p-0' }, ...rows.map((r) => {
    const bar = h('i', { class: 'block h-2 rounded-r-[4px] rounded-l-[2px]' });
    bar.style.setProperty('width', `${Math.max(2, (r.value / max) * 100).toFixed(1)}%`);
    bar.style.setProperty('background-color', 'var(--color-series-1)');
    const name = r.href !== undefined ? h('a', { href: r.href, class: 'font-medium text-on-surface no-underline hover:underline' }, r.label) : h('span', { class: 'font-medium' }, r.label);
    return h('li', { class: 'flex flex-col gap-1' },
      h('div', { class: 'flex items-baseline gap-2 text-sm' }, name, r.sub !== undefined ? h('span', { class: 'text-xs text-on-surface-variant' }, r.sub) : null,
        h('span', { class: 'ml-auto font-semibold tabular-nums' }, String(r.value))),
      h('div', { class: 'h-2 w-full rounded-full bg-surface-highest' }, bar));
  }));
}
