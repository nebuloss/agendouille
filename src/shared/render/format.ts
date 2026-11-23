/** Dates and sizes as the UIs show them, in French. */

const dateTime = new Intl.DateTimeFormat('fr-FR', { dateStyle: 'medium', timeStyle: 'short' });
const dateOnly = new Intl.DateTimeFormat('fr-FR', { dateStyle: 'medium' });

/** "23 nov. 2026, 14:05", or "jamais" for a missing time. */
export function when(iso: string | null | undefined): string {
  if (iso === null || iso === undefined || iso === '') return 'jamais';
  const d = new Date(iso);
  return Number.isNaN(d.getTime()) ? iso : dateTime.format(d);
}

export function day(iso: string | null | undefined): string {
  if (iso === null || iso === undefined || iso === '') return '';
  const d = new Date(iso);
  return Number.isNaN(d.getTime()) ? iso : dateOnly.format(d);
}

/** "il y a 3 jours", relative to now (ms). */
export function ago(iso: string | null | undefined, now: number = Date.now()): string {
  if (iso === null || iso === undefined || iso === '') return 'jamais';
  const t = new Date(iso).getTime();
  if (Number.isNaN(t)) return iso;
  const s = Math.round((now - t) / 1000);
  if (s < 60) return "à l'instant";
  const m = Math.round(s / 60);
  if (m < 60) return `il y a ${m} min`;
  const h = Math.round(m / 60);
  if (h < 48) return `il y a ${h} h`;
  return `il y a ${Math.round(h / 24)} jours`;
}

/** 1536 -> "1,5 Kio". */
export function bytes(n: number): string {
  if (n < 1024) return `${n} o`;
  const units = ['Kio', 'Mio', 'Gio', 'Tio'];
  let v = n / 1024;
  let i = 0;
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024;
    i++;
  }
  return `${v.toLocaleString('fr-FR', { maximumFractionDigits: v < 10 ? 1 : 0 })} ${units[i]}`;
}
