import { h, type Child } from '@/shared/core/Dom.ts';
import { check, dialog, styleControl, swatch } from '@/shared/components/Ui.ts';
import { icon, type IconName } from '@/shared/components/icons.ts';
import { ui } from '@/shared/components/recipes.ts';
import { addDays, addMinutes, formatDate, formatLocal, parseLocal, toNaive, type Wall } from '@/shared/core/Dates.ts';
import { DAYS, dayOf, describe, fromRule, none, toRule, type Repeat } from '@/app/model/Recurrence.ts';
import { canWrite, type Calendar, type EventInput, type EventView, type Occurrence, type Scope } from '@/app/api.ts';
import { whenText } from '@/app/components/views.ts';

const DAY_LETTERS: Record<string, string> = { MO: 'L', TU: 'M', WE: 'M', TH: 'J', FR: 'V', SA: 'S', SU: 'D' };

/** A row of the editor or the sheet: an icon, then its content. */
function row(ic: IconName, ...content: (Node | string | null)[]): HTMLElement {
  return h('div', { class: 'flex items-start gap-4' }, h('span', { class: 'mt-2.5 text-on-surface-variant' }, icon(ic)),
    h('div', { class: 'flex min-w-0 flex-1 flex-col gap-2' }, ...content));
}

/** A day of the week as a round toggle (L M M J V S D). */
function dayToggle(box: HTMLInputElement, letter: string, name: string): HTMLElement {
  box.className = 'peer sr-only';
  return h('label', { title: name, class: 'cursor-pointer' }, box,
    h('span', { class: 'grid size-9 place-items-center rounded-full border border-outline-variant text-sm font-semibold peer-checked:border-primary peer-checked:bg-primary peer-checked:text-on-primary peer-focus-visible:outline-2 peer-focus-visible:outline-primary' }, letter));
}

const DAY_NAMES: Record<string, string> = { MO: 'lundi', TU: 'mardi', WE: 'mercredi', TH: 'jeudi', FR: 'vendredi', SA: 'samedi', SU: 'dimanche' };

export interface EditorResult {
  calendar: string;
  input: EventInput;
}

/**
 * The event editor (PLAN.md 9.4): calendar, title, all-day, start, end,
 * place, notes, repeat. Resolves with the input, or null when cancelled.
 */
export async function editEvent(doc: Document, calendars: Calendar[], initial: {
  calendar: string; summary: string; location: string; description: string; allDay: boolean; start: Wall; end: Wall; rrule: string | null;
  ruleEditable: boolean; isNew: boolean;
}): Promise<EditorResult | null> {
  const writable = calendars.filter(canWrite);
  if (writable.length === 0) return null;
  const cal = h('select', { name: 'calendar' }, ...writable.map((c) => h('option', { value: c.ref, selected: c.ref === initial.calendar }, c.name)));
  const title = h('input', { name: 'summary', value: initial.summary, required: true, autocomplete: 'off', placeholder: 'Ajouter un titre',
    class: 'w-full border-0 border-b-2 border-outline-variant bg-transparent px-1 pb-2 text-2xl text-on-surface outline-none placeholder:text-on-surface-variant focus:border-primary focus-visible:outline-none' });
  const allDay = h('input', { type: 'checkbox', name: 'all_day', checked: initial.allDay });
  const startDate = h('input', { type: 'date', value: formatDate(initial.start), required: true });
  const startTime = h('input', { type: 'time', value: formatLocal(initial.start).slice(11, 16) });
  const endDate = h('input', { type: 'date', value: formatDate(initial.allDay ? addDays(initial.end, -1) : initial.end), required: true });
  const endTime = h('input', { type: 'time', value: formatLocal(initial.end).slice(11, 16) });
  const where = h('input', { value: initial.location, placeholder: 'Lieu' });
  const notes = h('textarea', { rows: 3, placeholder: 'Notes' });
  notes.value = initial.description;
  const times = [startTime, endTime];
  const syncAllDay = (): void => times.forEach((t) => { t.hidden = allDay.checked; });
  allDay.addEventListener('change', syncAllDay);
  syncAllDay();
  // Moving the start moves the end with it, as calendar apps do.
  let lastStart = toNaive(initial.start);
  const readStart = (): Wall => parseLocal(`${startDate.value}T${allDay.checked ? '00:00' : startTime.value || '00:00'}`);
  const onStart = (): void => {
    try {
      const s = readStart();
      const delta = (toNaive(s) - lastStart) / 60000;
      const e = addMinutes(parseLocal(`${endDate.value}T${endTime.value || '00:00'}`), delta);
      endDate.value = formatDate(e);
      endTime.value = formatLocal(e).slice(11, 16);
      lastStart = toNaive(s);
    } catch {
      /* a half-typed date */
    }
  };
  startDate.addEventListener('change', onStart);
  startTime.addEventListener('change', onStart);

  // Repeat.
  const repeat: Repeat = fromRule(initial.rrule) ?? none();
  const freq = h('select', {},
    ...([['none', 'Ne se répète pas'], ['daily', 'Chaque jour'], ['weekly', 'Chaque semaine'], ['monthly', 'Chaque mois'], ['yearly', 'Chaque année']] as const)
      .map(([v, l]) => h('option', { value: v, selected: repeat.freq === v }, l)));
  const interval = h('input', { type: 'number', min: 1, max: 99, value: repeat.interval, class: `${ui.input} !w-20` });
  const dayBoxes = DAYS.map((d) => h('input', { type: 'checkbox', value: d, checked: repeat.days.includes(d) || (repeat.days.length === 0 && d === dayOf(initial.start)) }));
  const end = h('select', {}, h('option', { value: 'never', selected: repeat.end === 'never' }, 'Sans fin'),
    h('option', { value: 'until', selected: repeat.end === 'until' }, "Jusqu'au"), h('option', { value: 'count', selected: repeat.end === 'count' }, 'Nombre de fois'));
  const until = h('input', { type: 'date', value: repeat.until });
  const count = h('input', { type: 'number', min: 1, max: 1000, value: repeat.count, class: `${ui.input} !w-24` });
  const weekly = h('div', { class: 'flex flex-wrap gap-1.5' }, ...DAYS.map((d, i) => dayToggle(dayBoxes[i]!, DAY_LETTERS[d] ?? d, DAY_NAMES[d] ?? d)));
  for (const el of [cal, startDate, startTime, endDate, endTime, where, notes, freq, end, until]) styleControl(el);
  for (const el of [startDate, endDate]) el.classList.add('!w-auto', 'min-w-0', 'flex-1');
  for (const el of [startTime, endTime]) el.classList.add('!w-32', 'shrink-0');
  const when = (label: string, date: HTMLInputElement, time: HTMLInputElement): HTMLElement =>
    h('div', { class: 'flex flex-col gap-1 sm:grid sm:grid-cols-[3rem_minmax(0,1fr)] sm:items-center sm:gap-2' },
      h('span', { class: 'text-sm text-on-surface-variant' }, label),
      h('div', { class: 'flex min-w-0 items-center gap-2' }, date, time));
  const unit = h('span', {});
  const UNITS: Record<string, string> = { daily: 'jours', weekly: 'semaines', monthly: 'mois', yearly: 'ans' };
  const details = h('div', { class: 'flex flex-col gap-3' },
    h('div', { class: 'flex items-center gap-2 text-sm' }, 'Tous les', interval, unit), weekly,
    h('div', { class: 'flex flex-wrap items-center gap-2' }, end, until, count));
  const syncRepeat = (): void => {
    unit.textContent = UNITS[freq.value] ?? '';
    details.hidden = freq.value === 'none';
    weekly.hidden = freq.value !== 'weekly';
    until.hidden = end.value !== 'until';
    count.hidden = end.value !== 'count';
  };
  for (const el of [freq, end]) el.addEventListener('change', syncRepeat);
  syncRepeat();
  const repeatBlock: Child = initial.ruleEditable
    ? row('repeat', freq, details)
    : row('repeat', h('p', { class: 'pt-2 text-sm' }, `${describe(initial.rrule)} (gardée telle quelle).`));
  where.setAttribute('placeholder', 'Ajouter un lieu');
  notes.setAttribute('placeholder', 'Ajouter des notes');

  const body: Child[] = [
    title,
    row('clock',
      h('div', { class: 'pt-2' }, check(allDay, 'Toute la journée')),
      when('Début', startDate, startTime),
      when('Fin', endDate, endTime)),
    repeatBlock,
    row('pin', where),
    row('notes', notes),
    writable.length > 1 ? row('calendar', cal) : null,
  ];
  const v = await dialog(doc, initial.isNew ? 'Nouvel événement' : "Modifier l'événement", body, [
    { label: 'Annuler', value: 'cancel' },
    { label: 'Enregistrer', value: 'save', kind: 'filled' },
  ]);
  if (v !== 'save') return null;
  const isAllDay = allDay.checked;
  const start = isAllDay ? startDate.value : `${startDate.value}T${startTime.value || '00:00'}:00`;
  let endValue = isAllDay ? formatDate(addDays(parseLocal(endDate.value), 1)) : `${endDate.value}T${endTime.value || '00:00'}:00`;
  if (!isAllDay && endValue < start) endValue = start;
  let rrule: string | null | undefined;
  if (initial.ruleEditable) {
    rrule = toRule({
      freq: freq.value as Repeat['freq'], interval: Number(interval.value) || 1,
      days: dayBoxes.filter((b) => b.checked).map((b) => b.value), end: end.value as Repeat['end'],
      until: until.value, count: Number(count.value) || 1,
    });
  } else rrule = undefined; // keep the rule the editor cannot show
  const input: EventInput = { summary: title.value.trim(), location: where.value.trim(), description: notes.value, all_day: isAllDay, start, end: endValue };
  if (rrule !== undefined) input.rrule = rrule ?? '';
  return { calendar: writable.length > 1 ? cal.value : writable[0]!.ref, input };
}

/** Asks which occurrences an edit or a deletion of a recurring event is for. */
export async function askScope(doc: Document, verb: 'modify' | 'delete'): Promise<Scope | null> {
  const v = await dialog(doc, verb === 'modify' ? 'Modifier un événement récurrent' : 'Supprimer un événement récurrent',
    [h('p', {}, verb === 'modify' ? 'Quels événements modifier ?' : 'Quels événements supprimer ?')], [
      { label: 'Annuler', value: '' },
      { label: 'Celui-ci', value: 'this', kind: 'filled' },
      { label: 'Celui-ci et les suivants', value: 'following' },
      { label: 'Tous', value: 'all' },
    ]);
  return v === 'this' || v === 'following' || v === 'all' ? v : null;
}

/** The detail sheet of an occurrence. Resolves with "edit", "delete" or "". */
export async function showEvent(doc: Document, o: Occurrence, ev: EventView, cal: Calendar | undefined): Promise<string> {
  const line = (ic: IconName, ...content: (Node | string | null)[]): HTMLElement =>
    h('div', { class: 'flex items-start gap-4 text-on-surface' }, h('span', { class: 'mt-0.5 text-on-surface-variant' }, icon(ic)), h('div', { class: 'min-w-0 flex-1' }, ...content));
  const body: Child[] = [
    line('clock', h('p', { class: 'font-medium first-letter:uppercase' }, whenText(o)),
      o.recurring || ev.rrule !== '' ? h('p', { class: 'text-sm text-on-surface-variant' }, describe(ev.rrule)) : null),
    o.location !== '' ? line('pin', h('p', {}, o.location)) : null,
    ev.description !== '' ? line('notes', h('p', { class: 'whitespace-pre-wrap' }, ev.description)) : null,
    ev.has_alarms ? line('bell', h('p', {}, 'Avec un rappel')) : null,
    line('calendar', h('p', { class: 'flex items-center gap-2' }, swatch(cal?.color ?? '#888888'), cal?.name ?? o.calendar,
      ev.writable ? null : h('span', { class: ui.pill.muted }, 'lecture seule'))),
    line('person', h('p', { class: 'text-sm text-on-surface-variant' },
      `Ajouté par ${ev.created_by || '?'}` + (ev.modified_by !== '' && ev.modified_by !== ev.created_by ? `, modifié par ${ev.modified_by}` : ''))),
  ];
  const buttons = ev.writable
    ? [{ label: 'Supprimer', value: 'delete', kind: 'danger-text' as const }, { label: 'Fermer', value: '' }, { label: 'Modifier', value: 'edit', kind: 'filled' as const }]
    : [{ label: 'Fermer', value: '', kind: 'filled' as const }];
  return dialog(doc, o.summary, body, buttons);
}
