import { h, type Child } from '@/shared/core/Dom.ts';
import { dialog, field, swatch } from '@/shared/components/Ui.ts';
import { addDays, addMinutes, formatDate, formatLocal, parseLocal, toNaive, type Wall } from '@/shared/core/Dates.ts';
import { DAYS, dayOf, describe, fromRule, none, toRule, type Repeat } from '@/app/model/Recurrence.ts';
import { canWrite, type Calendar, type EventInput, type EventView, type Occurrence, type Scope } from '@/app/api.ts';
import { whenText } from '@/app/components/views.ts';

const DAY_LETTERS: Record<string, string> = { MO: 'L', TU: 'M', WE: 'M', TH: 'J', FR: 'V', SA: 'S', SU: 'D' };

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
  const title = h('input', { name: 'summary', value: initial.summary, required: true, autocomplete: 'off', placeholder: 'Titre' });
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
  const interval = h('input', { type: 'number', min: 1, max: 99, value: repeat.interval, class: 'short' });
  const dayBoxes = DAYS.map((d) => h('input', { type: 'checkbox', value: d, checked: repeat.days.includes(d) || (repeat.days.length === 0 && d === dayOf(initial.start)) }));
  const end = h('select', {}, h('option', { value: 'never', selected: repeat.end === 'never' }, 'Sans fin'),
    h('option', { value: 'until', selected: repeat.end === 'until' }, "Jusqu'au"), h('option', { value: 'count', selected: repeat.end === 'count' }, 'Nombre de fois'));
  const until = h('input', { type: 'date', value: repeat.until });
  const count = h('input', { type: 'number', min: 1, max: 1000, value: repeat.count, class: 'short' });
  const weekly = h('div', { class: 'checks days' }, ...DAYS.map((d, i) => h('label', { title: d }, dayBoxes[i]!, DAY_LETTERS[d] ?? d)));
  const unit = h('span', {});
  const UNITS: Record<string, string> = { daily: 'jours', weekly: 'semaines', monthly: 'mois', yearly: 'ans' };
  const details = h('div', { class: 'repeat-details' }, h('div', { class: 'row' }, 'Tous les', interval, unit), weekly, h('div', { class: 'row' }, end, until, count));
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
    ? h('div', {}, field('Répétition', freq), details)
    : h('p', { class: 'muted' }, `${describe(initial.rrule)} (gardée telle quelle).`);

  const body: Child[] = [
    writable.length > 1 ? field('Calendrier', cal) : null,
    title,
    h('label', { class: 'checks' }, allDay, 'Toute la journée'),
    h('div', { class: 'row' }, h('span', { class: 'field-label' }, 'Début'), startDate, startTime),
    h('div', { class: 'row' }, h('span', { class: 'field-label' }, 'Fin'), endDate, endTime),
    where,
    notes,
    repeatBlock,
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
  const body: Child[] = [
    h('p', { class: 'sheet-cal' }, swatch(cal?.color ?? '#888888'), cal?.name ?? o.calendar),
    h('p', { class: 'sheet-when' }, whenText(o)),
    o.recurring || ev.rrule !== '' ? h('p', { class: 'muted' }, describe(ev.rrule)) : null,
    o.location !== '' ? h('p', {}, '📍 ', o.location) : null,
    ev.description !== '' ? h('p', { class: 'sheet-notes' }, ev.description) : null,
    ev.has_alarms ? h('p', { class: 'muted' }, 'Avec un rappel') : null,
    h('p', { class: 'muted' }, `Ajouté par ${ev.created_by || '?'}` + (ev.modified_by !== '' && ev.modified_by !== ev.created_by ? `, modifié par ${ev.modified_by}` : '')),
    ev.writable ? null : h('p', { class: 'chip' }, 'Lecture seule'),
  ];
  const buttons = ev.writable
    ? [{ label: 'Supprimer', value: 'delete', kind: 'danger' as const }, { label: 'Fermer', value: '' }, { label: 'Modifier', value: 'edit', kind: 'filled' as const }]
    : [{ label: 'Fermer', value: '', kind: 'filled' as const }];
  return dialog(doc, o.summary, body, buttons);
}
