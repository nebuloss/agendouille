import { h, type Child } from '@/shared/core/Dom.ts';
import { icon, type IconName } from '@/shared/components/icons.ts';
import { ui } from '@/shared/components/recipes.ts';

/**
 * The one way the admin UI lists things (users, groups, members,
 * calendars): a full-width row per item, the whole row opening the item's
 * page, its actions at the end, outside the link (a button cannot live in
 * a link). So navigating is always the same gesture: click the row.
 */
export interface Entry {
  /** The item's page; or open, for a row that opens a dialog; or neither. */
  href?: string;
  open?: () => void;
  lead?: Node;
  title: string;
  sub?: string;
  tags?: Child[];
  stats?: { n: string; label: string }[];
  actions?: Child[];
}

export function entries(list: Entry[], empty: string): HTMLElement {
  if (list.length === 0) return h('p', { class: ui.empty }, empty);
  return h('ul', { class: `${ui.well} m-0 list-none p-0` }, ...list.map((e) =>
    h('li', { class: 'flex items-center border-b border-outline-variant/50 last:border-0' },
      row(e,
        e.lead ?? null,
        h('span', { class: 'flex min-w-0 flex-1 flex-col' },
          h('span', { class: 'flex min-w-0 flex-wrap items-center gap-2' }, h('b', { class: 'truncate font-semibold' }, e.title), ...(e.tags ?? [])),
          e.sub !== undefined && e.sub !== '' ? h('span', { class: 'truncate text-xs text-on-surface-variant' }, e.sub) : null),
        ...(e.stats ?? []).map((s) => h('span', { class: 'hidden w-24 shrink-0 flex-col items-end md:flex' },
          h('b', { class: 'font-semibold tabular-nums' }, s.n), h('span', { class: 'text-[11px] text-on-surface-variant' }, s.label))),
        e.href !== undefined || e.open !== undefined ? h('span', { class: 'shrink-0 text-on-surface-variant' }, icon('right')) : null),
      e.actions !== undefined && e.actions.length > 0 ? h('span', { class: 'flex shrink-0 items-center gap-1 pr-3' }, ...e.actions) : null)));
}

/** The row's main part: a link, a button that opens something, or plain. */
function row(e: Entry, ...children: Child[]): HTMLElement {
  const cls = 'flex min-h-16 min-w-0 flex-1 items-center gap-3 px-4 py-3 text-left text-on-surface no-underline';
  if (e.href !== undefined) return h('a', { href: e.href, class: `state-layer ${cls}` }, ...children);
  if (e.open !== undefined) {
    const b = h('button', { type: 'button', class: `state-layer ${cls}` }, ...children);
    b.addEventListener('click', e.open);
    return b;
  }
  return h('div', { class: cls }, ...children);
}

/** A square mark holding one icon, on a tonal surface. */
export function iconMark(name: IconName): HTMLElement {
  return h('span', { class: 'grid size-10 shrink-0 place-items-center rounded-md3-m bg-secondary-container text-on-secondary-container [&_.ic]:size-5', 'aria-hidden': 'true' }, icon(name));
}

/** A person's round mark: their initial on their colour. */
export function avatar(name: string, color: string): HTMLElement {
  const a = h('span', { class: 'grid size-10 shrink-0 place-items-center rounded-full text-sm font-bold text-white', 'aria-hidden': 'true' }, (name || '?').slice(0, 1).toUpperCase());
  if (/^#[0-9a-f]{6}$/i.test(color)) a.style.setProperty('background-color', color);
  else a.classList.add('bg-primary');
  return a;
}

/** A group's square mark: its colour, with the group glyph. */
export function groupMark(color: string): HTMLElement {
  const g = h('span', { class: 'grid size-10 shrink-0 place-items-center rounded-md3-m text-white [&_.ic]:size-5', 'aria-hidden': 'true' }, icon('group'));
  if (/^#[0-9a-f]{6}$/i.test(color)) g.style.setProperty('background-color', color);
  else g.classList.add('bg-secondary');
  return g;
}

/** A calendar's mark: a rounded square of its colour, with the calendar glyph. */
export function calendarMark(color: string): HTMLElement {
  const c = h('span', { class: 'grid size-10 shrink-0 place-items-center rounded-md3-m text-white [&_.ic]:size-5', 'aria-hidden': 'true' }, icon('calendar'));
  if (/^#[0-9a-f]{6}$/i.test(color)) c.style.setProperty('background-color', color);
  else c.classList.add('bg-outline');
  return c;
}
