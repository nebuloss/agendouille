/**
 * What a coloured thing means, apart from how a given component paints it
 * (claudication's web/src/ui/styles/tone.ts). One place maps each meaning
 * onto the MD3 roles of theme.css, so a meaning is the same colour in the
 * Android app, the web app and the admin UI.
 *
 * - ok: done, fine, up to date.
 * - warn: needs a look (never synced, a device's password only).
 * - error: broken, refused.
 * - accent: worth noticing, no judgement (shared, read and write, a figure).
 * - neutral: just a fact.
 */
export type Tone = 'ok' | 'warn' | 'error' | 'accent' | 'neutral';

/** A filled container and the ink that sits on it: chips, banners. */
export const toneContainer: Record<Tone, string> = {
  ok: 'bg-success-container text-on-success-container',
  warn: 'bg-warning-container text-on-warning-container',
  error: 'bg-error-container text-on-error-container',
  accent: 'bg-primary-container text-on-primary-container',
  neutral: 'bg-surface-high text-on-surface-variant',
};

/** Text in that meaning, on an ordinary surface: a headline figure. */
export const toneInk: Record<Tone, string> = {
  ok: 'text-success',
  warn: 'text-warning',
  error: 'text-error',
  accent: 'text-primary',
  neutral: 'text-on-surface',
};

/** How recent a sync or a visit is: two days fine, a fortnight a fact, more (or never) a look. */
export function recencyTone(iso: string | null | undefined, now: number = Date.now()): Tone {
  if (iso === null || iso === undefined || iso === '') return 'warn';
  const days = (now - new Date(iso).getTime()) / 86400000;
  if (Number.isNaN(days)) return 'neutral';
  return days <= 2 ? 'ok' : days <= 14 ? 'neutral' : 'warn';
}
