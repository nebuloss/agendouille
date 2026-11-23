import { labelDate, parseLocal, weekday, type Wall } from '@/shared/core/Dates.ts';
import { locale } from '@/shared/i18n/i18n.ts';
import { t } from '@/app/model/Recurrence.msg.ts';

/**
 * The editor's repeat fields <-> an RRULE value (PLAN.md 9.4): none, daily,
 * weekly on chosen days, monthly by date, yearly; every n; until a date or
 * a number of times. A rule with more than that is shown in words and
 * kept as it is.
 */

export type Freq = 'none' | 'daily' | 'weekly' | 'monthly' | 'yearly';
export type End = 'never' | 'until' | 'count';

export interface Repeat {
  freq: Freq;
  interval: number;
  days: string[]; // MO..SU, weekly only
  end: End;
  until: string; // YYYY-MM-DD
  count: number;
}

export const DAYS = ['MO', 'TU', 'WE', 'TH', 'FR', 'SA', 'SU'] as const;

/** A weekday code's name in the page's language: "mercredi", "Wednesday"; "M", "W" when narrow. */
export function dayName(code: string, width: 'long' | 'narrow' = 'long'): string {
  const i = (DAYS as readonly string[]).indexOf(code);
  if (i < 0) return code;
  // 23 November 2026 is a Monday.
  return new Intl.DateTimeFormat(locale(), { weekday: width, timeZone: 'UTC' }).format(new Date(Date.UTC(2026, 10, 23 + i)));
}

const FREQS: Record<string, Freq> = { DAILY: 'daily', WEEKLY: 'weekly', MONTHLY: 'monthly', YEARLY: 'yearly' };

export function none(): Repeat {
  return { freq: 'none', interval: 1, days: [], end: 'never', until: '', count: 10 };
}

function parts(rule: string): Map<string, string> {
  const m = new Map<string, string>();
  for (const p of rule.split(';')) {
    const [k, v] = p.split('=');
    if (k !== undefined && v !== undefined) m.set(k.toUpperCase(), v.toUpperCase());
  }
  return m;
}

/** The editor's fields for a rule, or null when the editor cannot show it. */
export function fromRule(rule: string | null | undefined): Repeat | null {
  if (rule === null || rule === undefined || rule === '') return none();
  const p = parts(rule);
  const freq = FREQS[p.get('FREQ') ?? ''];
  if (freq === undefined) return null;
  for (const k of p.keys()) {
    if (!['FREQ', 'INTERVAL', 'COUNT', 'UNTIL', 'BYDAY', 'BYMONTHDAY', 'BYMONTH', 'WKST'].includes(k)) return null;
  }
  const days = p.get('BYDAY')?.split(',') ?? [];
  if (days.length > 0 && (freq !== 'weekly' || days.some((d) => !(DAYS as readonly string[]).includes(d)))) return null;
  const r: Repeat = { ...none(), freq, interval: Number(p.get('INTERVAL') ?? 1), days };
  const until = p.get('UNTIL');
  if (until !== undefined) {
    r.end = 'until';
    r.until = `${until.slice(0, 4)}-${until.slice(4, 6)}-${until.slice(6, 8)}`;
  } else if (p.has('COUNT')) {
    r.end = 'count';
    r.count = Number(p.get('COUNT'));
  }
  return r;
}

/** The RRULE value for the fields; null for no repeat. */
export function toRule(r: Repeat): string | null {
  if (r.freq === 'none') return null;
  const out = [`FREQ=${r.freq.toUpperCase()}`];
  if (r.interval > 1) out.push(`INTERVAL=${Math.floor(r.interval)}`);
  if (r.freq === 'weekly' && r.days.length > 0) out.push(`BYDAY=${DAYS.filter((d) => r.days.includes(d)).join(',')}`);
  // UNTIL as a date: the server turns it into the end of that day in the
  // family zone, in UTC, for a timed event (internal/ical untilOf).
  if (r.end === 'until' && r.until !== '') out.push(`UNTIL=${r.until.replaceAll('-', '')}`);
  if (r.end === 'count' && r.count > 0) out.push(`COUNT=${Math.floor(r.count)}`);
  return out.join(';');
}

/** The weekday code of a start, for the editor's default. */
export function dayOf(w: Wall): string {
  return DAYS[weekday(w)]!;
}

/** A rule in words: "Chaque semaine le mercredi, jusqu’au 30 juin 2027". */
export function describe(rule: string | null | undefined): string {
  if (rule === null || rule === undefined || rule === '') return t('never');
  const r = fromRule(rule);
  if (r === null) return t('advanced');
  const count = r.interval;
  let s: string;
  switch (r.freq) {
    case 'daily':
    case 'monthly':
    case 'yearly':
      s = t(r.freq, { count });
      break;
    case 'weekly': {
      const names = r.days.map((d) => dayName(d));
      if (names.length === 0) s = t('weekly', { count });
      else if (names.length === 1) s = t('weeklyOnDay', { count, day: names[0]! });
      else s = t('weeklyOnDays', { count, days: new Intl.ListFormat(locale(), { type: 'conjunction' }).format(names) });
      break;
    }
    default:
      s = t('never');
  }
  if (r.end === 'until' && r.until !== '') s = t('until', { rule: s, date: labelDate(parseLocal(r.until)) });
  if (r.end === 'count') s = t('times', { rule: s, count: r.count });
  return s;
}
