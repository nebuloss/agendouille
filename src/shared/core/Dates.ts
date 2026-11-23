/**
 * Wall-clock dates and times in the family zone (PLAN.md 4.4, 9.5).
 *
 * The server sends local times ("2026-11-25T14:00:00") already in the
 * family zone, so the browser never converts zones: it does calendar
 * arithmetic on naive wall times, kept in a Date's UTC fields, where a day
 * is always 24 hours. The only time a zone matters is "now", read with
 * Intl in the family zone, whatever zone the browser is in.
 */

export interface Wall {
  readonly y: number;
  readonly m: number; // 1-12
  readonly d: number;
  readonly h: number;
  readonly mi: number;
  readonly s: number;
}

const pad = (n: number, w = 2): string => String(n).padStart(w, '0');

/** Reads "YYYY-MM-DD", "YYYY-MM-DDTHH:MM" or "YYYY-MM-DDTHH:MM:SS". */
export function parseLocal(s: string): Wall {
  const m = /^(\d{4})-(\d{2})-(\d{2})(?:T(\d{2}):(\d{2})(?::(\d{2}))?)?$/.exec(s);
  if (m === null) throw new RangeError(`not a local time: ${s}`);
  return { y: +m[1]!, m: +m[2]!, d: +m[3]!, h: +(m[4] ?? 0), mi: +(m[5] ?? 0), s: +(m[6] ?? 0) };
}

export function formatLocal(w: Wall): string {
  return `${formatDate(w)}T${pad(w.h)}:${pad(w.mi)}:${pad(w.s)}`;
}

export function formatDate(w: Wall): string {
  return `${pad(w.y, 4)}-${pad(w.m)}-${pad(w.d)}`;
}

/** "14:05". */
export function formatTime(w: Wall): string {
  return `${pad(w.h)}:${pad(w.mi)}`;
}

export function toNaive(w: Wall): number {
  return Date.UTC(w.y, w.m - 1, w.d, w.h, w.mi, w.s);
}

export function fromNaive(ms: number): Wall {
  const d = new Date(ms);
  return { y: d.getUTCFullYear(), m: d.getUTCMonth() + 1, d: d.getUTCDate(), h: d.getUTCHours(), mi: d.getUTCMinutes(), s: d.getUTCSeconds() };
}

export function day(w: Wall): Wall {
  return { y: w.y, m: w.m, d: w.d, h: 0, mi: 0, s: 0 };
}

export function addDays(w: Wall, n: number): Wall {
  return fromNaive(Date.UTC(w.y, w.m - 1, w.d + n, w.h, w.mi, w.s));
}

export function addMonths(w: Wall, n: number): Wall {
  return fromNaive(Date.UTC(w.y, w.m - 1 + n, 1, w.h, w.mi, w.s));
}

export function addMinutes(w: Wall, n: number): Wall {
  return fromNaive(toNaive(w) + n * 60000);
}

/** Monday 0 ... Sunday 6. */
export function weekday(w: Wall): number {
  return (new Date(toNaive(day(w))).getUTCDay() + 6) % 7;
}

/** The Monday of w's week. */
export function startOfWeek(w: Wall): Wall {
  return addDays(day(w), -weekday(w));
}

export function startOfMonth(w: Wall): Wall {
  return { y: w.y, m: w.m, d: 1, h: 0, mi: 0, s: 0 };
}

export function sameDay(a: Wall, b: Wall): boolean {
  return a.y === b.y && a.m === b.m && a.d === b.d;
}

export function compare(a: Wall, b: Wall): number {
  return toNaive(a) - toNaive(b);
}

/** Whole days from a to b (b's day minus a's day). */
export function daysBetween(a: Wall, b: Wall): number {
  return Math.round((toNaive(day(b)) - toNaive(day(a))) / 86400000);
}

/** Minutes since midnight. */
export function minuteOfDay(w: Wall): number {
  return w.h * 60 + w.mi;
}

/** Now as wall time in zone (the family's), read with Intl. */
export function nowIn(zone: string, now: number = Date.now()): Wall {
  const parts = new Intl.DateTimeFormat('en-GB', {
    timeZone: zone, hourCycle: 'h23', year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', second: '2-digit',
  }).formatToParts(new Date(now));
  const get = (t: string): number => Number(parts.find((p) => p.type === t)?.value ?? 0);
  return { y: get('year'), m: get('month'), d: get('day'), h: get('hour') % 24, mi: get('minute'), s: get('second') };
}

const fmt = (o: Intl.DateTimeFormatOptions) => new Intl.DateTimeFormat('fr-FR', { ...o, timeZone: 'UTC' });
const longDay = fmt({ weekday: 'long', day: 'numeric', month: 'long' });
const shortDay = fmt({ weekday: 'short', day: 'numeric' });
const monthYear = fmt({ month: 'long', year: 'numeric' });
const fullDate = fmt({ day: 'numeric', month: 'long', year: 'numeric' });
const weekdayShort = fmt({ weekday: 'short' });

/** "mercredi 25 novembre". */
export function labelDay(w: Wall): string {
  return longDay.format(new Date(toNaive(w)));
}

/** "mer. 25". */
export function labelShortDay(w: Wall): string {
  return shortDay.format(new Date(toNaive(w)));
}

/** "novembre 2026". */
export function labelMonth(w: Wall): string {
  return monthYear.format(new Date(toNaive(w)));
}

/** "25 novembre 2026". */
export function labelDate(w: Wall): string {
  return fullDate.format(new Date(toNaive(w)));
}

/** "lun." ... "dim.", Monday first. */
export function weekdayNames(): string[] {
  const monday = { y: 2026, m: 11, d: 23, h: 0, mi: 0, s: 0 };
  return [0, 1, 2, 3, 4, 5, 6].map((i) => weekdayShort.format(new Date(toNaive(addDays(monday, i)))));
}
