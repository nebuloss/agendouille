/** Dates and sizes as the UIs show them, in the page's language. */
import { locale } from '@/shared/i18n/i18n.ts';
import { t } from '@/shared/render/format.msg.ts';

const cache = new Map<string, Intl.DateTimeFormat>();
function dtf(o: Intl.DateTimeFormatOptions): Intl.DateTimeFormat {
  const key = locale() + JSON.stringify(o);
  let f = cache.get(key);
  if (f === undefined) {
    f = new Intl.DateTimeFormat(locale(), o);
    cache.set(key, f);
  }
  return f;
}

/** "23 nov. 2026, 14:05", or "Jamais" for a missing time. */
export function when(iso: string | null | undefined): string {
  if (iso === null || iso === undefined || iso === '') return t('never');
  const d = new Date(iso);
  return Number.isNaN(d.getTime()) ? iso : dtf({ dateStyle: 'medium', timeStyle: 'short' }).format(d);
}

export function day(iso: string | null | undefined): string {
  if (iso === null || iso === undefined || iso === '') return '';
  const d = new Date(iso);
  return Number.isNaN(d.getTime()) ? iso : dtf({ dateStyle: 'medium' }).format(d);
}

/** "il y a 3 jours", relative to now (ms). */
export function ago(iso: string | null | undefined, now: number = Date.now()): string {
  if (iso === null || iso === undefined || iso === '') return t('never');
  const ms = new Date(iso).getTime();
  if (Number.isNaN(ms)) return iso;
  const s = Math.round((now - ms) / 1000);
  if (s < 60) return t('justNow');
  const rtf = new Intl.RelativeTimeFormat(locale(), { numeric: 'auto', style: 'long' });
  const m = Math.round(s / 60);
  if (m < 60) return rtf.format(-m, 'minute');
  const h = Math.round(m / 60);
  if (h < 48) return rtf.format(-h, 'hour');
  return rtf.format(-Math.round(h / 24), 'day');
}

/** 1536 -> "1,5 Kio" (French) or "1.5 KiB" (English). */
export function bytes(n: number): string {
  const units = t('byteUnits').split(' ');
  if (n < 1024) return `${n.toLocaleString(locale())} ${units[0]}`;
  let v = n / 1024;
  let i = 1;
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024;
    i++;
  }
  return `${v.toLocaleString(locale(), { maximumFractionDigits: v < 10 ? 1 : 0 })} ${units[i]}`;
}
