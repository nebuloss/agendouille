import { h } from '@/shared/core/Dom.ts';
import {
  addDays, compare, day, daysBetween, formatDate, formatTime, labelDay, labelShortDay, minuteOfDay, parseLocal, sameDay, weekdayNames,
  type Wall,
} from '@/shared/core/Dates.ts';
import type { Occurrence } from '@/app/api.ts';

/**
 * The three calendar views (PLAN.md 9.4) as pure renderers: occurrences
 * and a range in, DOM out. Clicks go to onOpen; an empty slot or day goes
 * to onNew with its start.
 */

export interface ViewContext {
  colorOf(ref: string): string;
  today: Wall;
  onOpen(o: Occurrence): void;
  onNew(start: Wall, allDay: boolean): void;
}

interface Placed {
  o: Occurrence;
  start: Wall;
  end: Wall; // exclusive for all-day events
}

function place(occ: Occurrence[]): Placed[] {
  return occ.map((o) => ({ o, start: parseLocal(o.start), end: parseLocal(o.end) }));
}

/** Whether an occurrence is on a given day. */
function onDay(p: Placed, d: Wall): boolean {
  const start = day(d);
  const next = addDays(start, 1);
  if (p.o.all_day) return compare(p.start, next) < 0 && compare(p.end, start) > 0;
  if (compare(p.end, p.start) === 0) return sameDay(p.start, d);
  return compare(p.start, next) < 0 && compare(p.end, start) > 0;
}

function chip(p: Placed, ctx: ViewContext, withTime: boolean): HTMLElement {
  const b = h('button', { class: 'event', type: 'button', title: p.o.summary },
    withTime && !p.o.all_day ? h('span', { class: 'event-time' }, formatTime(p.start)) : null,
    h('span', { class: 'event-title' }, p.o.summary),
    p.o.location !== '' ? h('span', { class: 'event-place' }, p.o.location) : null,
    p.o.created_by !== '' ? h('span', { class: 'event-who', 'aria-label': `ajouté par ${p.o.created_by}` }, p.o.created_by.slice(0, 1).toUpperCase()) : null,
  );
  b.style.setProperty('--cal', ctx.colorOf(p.o.calendar));
  if (p.o.all_day) b.classList.add('all-day');
  b.addEventListener('click', (e) => {
    e.stopPropagation();
    ctx.onOpen(p.o);
  });
  return b;
}

const byStart = (a: Placed, b: Placed): number =>
  Number(b.o.all_day) - Number(a.o.all_day) || compare(a.start, b.start) || a.o.summary.localeCompare(b.o.summary);

/** Agenda: a list of days with their events, empty days left out. */
export function agenda(occ: Occurrence[], from: Wall, days: number, ctx: ViewContext): HTMLElement {
  const placed = place(occ).sort(byStart);
  const root = h('div', { class: 'agenda' });
  let any = false;
  for (let i = 0; i < days; i++) {
    const d = addDays(from, i);
    const todays = placed.filter((p) => onDay(p, d));
    const isToday = sameDay(d, ctx.today);
    if (todays.length === 0 && !isToday) continue;
    any = true;
    const head = h('h2', { class: 'agenda-day' + (isToday ? ' today' : '') }, labelDay(d));
    const list = h('div', { class: 'agenda-list' }, ...todays.map((p) => chip(p, ctx, true)));
    if (todays.length === 0) list.append(h('p', { class: 'muted' }, "Rien aujourd'hui."));
    const add = h('button', { class: 'btn text add-here', type: 'button', 'aria-label': `Nouvel événement le ${labelDay(d)}` }, '+');
    add.addEventListener('click', () => ctx.onNew({ ...d, h: 9 }, false));
    root.append(h('section', { class: 'agenda-section', 'data-date': formatDate(d) }, h('div', { class: 'agenda-head' }, head, add), list));
  }
  if (!any) root.append(h('p', { class: 'empty' }, 'Rien sur cette période.'));
  return root;
}

const HOUR = 48; // px

/** Lays out one day's timed events in side-by-side lanes. */
function lanes(items: Placed[]): { p: Placed; lane: number; of: number }[] {
  const sorted = [...items].sort((a, b) => compare(a.start, b.start));
  const out: { p: Placed; lane: number; of: number }[] = [];
  let group: { p: Placed; lane: number; of: number }[] = [];
  let ends: Wall[] = [];
  let groupEnd: Wall | null = null;
  const flush = (): void => {
    for (const g of group) g.of = ends.length;
    out.push(...group);
    group = [];
    ends = [];
  };
  for (const p of sorted) {
    if (groupEnd !== null && compare(p.start, groupEnd) >= 0) {
      flush();
      groupEnd = null;
    }
    let lane = ends.findIndex((e) => compare(e, p.start) <= 0);
    if (lane < 0) {
      lane = ends.length;
      ends.push(p.end);
    } else ends[lane] = p.end;
    group.push({ p, lane, of: 1 });
    groupEnd = groupEnd === null || compare(p.end, groupEnd) > 0 ? p.end : groupEnd;
  }
  flush();
  return out;
}

/** Week: seven columns, an all-day strip, hours, the line of now. */
export function week(occ: Occurrence[], monday: Wall, ctx: ViewContext): HTMLElement {
  const placed = place(occ);
  const days = [0, 1, 2, 3, 4, 5, 6].map((i) => addDays(monday, i));
  const names = weekdayNames();
  const head = h('div', { class: 'week-head' }, h('div', { class: 'week-gutter' }),
    ...days.map((d, i) => h('div', { class: 'week-day-name' + (sameDay(d, ctx.today) ? ' today' : '') },
      h('span', {}, names[i] ?? ''), h('strong', {}, String(d.d)))));
  const strip = h('div', { class: 'week-allday' }, h('div', { class: 'week-gutter' }, 'jour'),
    ...days.map((d) => h('div', { class: 'week-allday-cell' }, ...placed.filter((p) => p.o.all_day && onDay(p, d)).map((p) => chip(p, ctx, false)))));
  const hours = h('div', { class: 'week-hours' });
  for (let hr = 0; hr < 24; hr++) hours.append(h('div', { class: 'week-hour' }, `${String(hr).padStart(2, '0')}:00`));
  const cols = days.map((d) => {
    const col = h('div', { class: 'week-col' + (sameDay(d, ctx.today) ? ' today' : ''), 'data-date': formatDate(d) });
    col.addEventListener('click', (e) => {
      const y = (e as MouseEvent).offsetY;
      const minutes = Math.max(0, Math.min(23 * 60 + 30, Math.floor(y / HOUR * 2) * 30));
      ctx.onNew({ ...d, h: Math.floor(minutes / 60), mi: minutes % 60 }, false);
    });
    const timed = placed.filter((p) => !p.o.all_day && onDay(p, d)).map((p) => ({
      ...p,
      start: sameDay(p.start, d) ? p.start : day(d),
      end: sameDay(p.end, d) || compare(p.end, addDays(day(d), 1)) < 0 ? p.end : { ...d, h: 23, mi: 59, s: 59 },
    }));
    for (const { p, lane, of } of lanes(timed)) {
      const c = chip(p, ctx, true);
      const top = (minuteOfDay(p.start) / 60) * HOUR;
      const span = Math.max(22, ((minuteOfDay(p.end) - minuteOfDay(p.start)) / 60) * HOUR);
      c.classList.add('timed');
      c.style.setProperty('top', `${top}px`);
      c.style.setProperty('height', `${span}px`);
      c.style.setProperty('left', `${(lane / of) * 100}%`);
      c.style.setProperty('width', `${100 / of}%`);
      col.append(c);
    }
    if (sameDay(d, ctx.today)) {
      const now = h('div', { class: 'now-line', 'aria-hidden': 'true' });
      now.style.setProperty('top', `${(minuteOfDay(ctx.today) / 60) * HOUR}px`);
      col.append(now);
    }
    return col;
  });
  const grid = h('div', { class: 'week-grid' }, hours, ...cols);
  const scroller = h('div', { class: 'week-scroll' }, grid);
  queueMicrotask(() => {
    scroller.scrollTop = 7.5 * HOUR;
  });
  return h('div', { class: 'week' }, head, strip, scroller);
}

/** Month: a grid of six weeks, three events a day and "+n". */
export function month(occ: Occurrence[], first: Wall, gridStart: Wall, ctx: ViewContext): HTMLElement {
  const placed = place(occ).sort(byStart);
  const names = weekdayNames();
  const root = h('div', { class: 'month' }, ...names.map((n) => h('div', { class: 'month-name' }, n)));
  for (let i = 0; i < 42; i++) {
    const d = addDays(gridStart, i);
    const todays = placed.filter((p) => onDay(p, d));
    const cell = h('div', { class: 'month-cell' + (d.m !== first.m ? ' other' : '') + (sameDay(d, ctx.today) ? ' today' : ''), 'data-date': formatDate(d) },
      h('span', { class: 'month-num' }, String(d.d)),
      ...todays.slice(0, 3).map((p) => chip(p, ctx, false)),
      todays.length > 3 ? h('span', { class: 'more' }, `+${todays.length - 3}`) : null);
    cell.addEventListener('click', () => ctx.onNew({ ...d, h: 9 }, false));
    root.append(cell);
  }
  return root;
}

/** "mer. 25, 14:00 – 15:30", "mer. 25" for a day, across days too. */
export function whenText(o: { start: string; end: string; all_day: boolean }): string {
  const s = parseLocal(o.start);
  const e = parseLocal(o.end);
  if (o.all_day) {
    const last = addDays(e, -1);
    return daysBetween(s, last) <= 0 ? labelDay(s) : `du ${labelDay(s)} au ${labelDay(last)}`;
  }
  if (sameDay(s, e)) return `${labelDay(s)}, ${formatTime(s)} – ${formatTime(e)}`;
  return `${labelShortDay(s)} ${formatTime(s)} – ${labelShortDay(e)} ${formatTime(e)}`;
}
