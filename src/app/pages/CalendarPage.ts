import { ApiError, seg, type Api } from '@/shared/core/Api.ts';
import { h } from '@/shared/core/Dom.ts';
import { DisposableBag } from '@/shared/core/Disposable.ts';
import type { Page } from '@/shared/core/Pages.ts';
import { Toasts, confirmDialog, dialog, swatch } from '@/shared/components/Ui.ts';
import {
  addDays, addMinutes, addMonths, formatDate, formatLocal, labelDay, labelMonth, nowIn, parseLocal, startOfMonth, startOfWeek, toNaive, day,
  type Wall,
} from '@/shared/core/Dates.ts';
import { appApi, canWrite, type Calendar, type Calendars, type EventView, type Occurrence } from '@/app/api.ts';
import { agenda, month, week, type ViewContext } from '@/app/components/views.ts';
import { askScope, editEvent, showEvent } from '@/app/components/Editor.ts';

type View = 'agenda' | 'week' | 'month';

/** The calendar: agenda, week, month; the sheet and the editor; live. */
export class CalendarPage implements Page {
  private readonly bag = new DisposableBag();
  private readonly root: HTMLElement;
  private readonly toasts: Toasts;
  private readonly zone: string;
  private view: View;
  private anchor: Wall;
  private calendars: Calendar[] = [];
  private body = h('div', { class: 'cal-body' });
  private title = h('h2', { class: 'cal-title' });
  private reloadTimer: ReturnType<typeof setTimeout> | null = null;

  constructor(
    private readonly doc: Document,
    private readonly api: Api = appApi(),
  ) {
    this.root = doc.getElementById('app-content') ?? doc.body;
    this.toasts = new Toasts(doc.getElementById('toasts'));
    this.zone = doc.querySelector<HTMLElement>('.app')?.dataset['zone'] ?? 'Europe/Paris';
    const wide = (doc.defaultView?.innerWidth ?? 1024) >= 840;
    const saved = this.remembered();
    this.view = saved ?? (wide ? 'week' : 'agenda');
    this.anchor = day(nowIn(this.zone));
  }

  private remembered(): View | null {
    try {
      const v = this.doc.defaultView?.localStorage.getItem('agendouille.view');
      return v === 'agenda' || v === 'week' || v === 'month' ? v : null;
    } catch {
      return null;
    }
  }

  start(): void {
    this.root.replaceChildren(this.toolbar(), this.title, this.body);
    void this.load();
    this.live();
  }

  dispose(): void {
    this.bag.dispose();
  }

  private toolbar(): HTMLElement {
    const btn = (label: string, run: () => void, cls = 'btn text', aria?: string): HTMLButtonElement => {
      const b = h('button', { class: cls, type: 'button', 'aria-label': aria ?? label }, label);
      b.addEventListener('click', run);
      return b;
    };
    const seg_ = h('div', { class: 'segmented', role: 'group', 'aria-label': 'Vue' });
    for (const [v, l] of [['agenda', 'Agenda'], ['week', 'Semaine'], ['month', 'Mois']] as const) {
      const b = btn(l, () => {
        this.view = v;
        try {
          this.doc.defaultView?.localStorage.setItem('agendouille.view', v);
        } catch {
          /* private mode */
        }
        void this.load();
      }, 'seg');
      b.dataset['view'] = v;
      seg_.append(b);
    }
    return h('div', { class: 'cal-toolbar' },
      btn("Aujourd'hui", () => {
        this.anchor = day(nowIn(this.zone));
        void this.load();
      }, 'btn tonal'),
      btn('‹', () => this.step(-1), 'btn text icon', 'Précédent'),
      btn('›', () => this.step(1), 'btn text icon', 'Suivant'),
      seg_,
      btn('Calendriers', () => void this.pickCalendars(), 'btn text'),
      btn('+ Événement', () => void this.create(this.anchorStart(), false), 'btn filled'),
    );
  }

  private anchorStart(): Wall {
    const now = nowIn(this.zone);
    const start = { ...this.anchor, h: now.h + 1, mi: 0, s: 0 };
    return start.h > 23 ? { ...this.anchor, h: 9 } : start;
  }

  private step(n: number): void {
    this.anchor = this.view === 'month' ? addMonths(this.anchor, n) : addDays(this.anchor, n * (this.view === 'week' ? 7 : 14));
    void this.load();
  }

  /** The range a view shows: its first day and its number of days. */
  private range(): { from: Wall; days: number; first: Wall } {
    switch (this.view) {
      case 'week':
        return { from: startOfWeek(this.anchor), days: 7, first: this.anchor };
      case 'month': {
        const first = startOfMonth(this.anchor);
        return { from: startOfWeek(first), days: 42, first };
      }
      default:
        return { from: day(this.anchor), days: 14, first: this.anchor };
    }
  }

  private colorOf = (ref: string): string => this.calendars.find((c) => c.ref === ref)?.color ?? '#73777f';

  async load(): Promise<void> {
    const { from, days, first } = this.range();
    for (const b of this.root.querySelectorAll<HTMLElement>('.seg')) b.setAttribute('aria-pressed', String(b.dataset['view'] === this.view));
    this.title.textContent = this.view === 'month' ? labelMonth(first) : this.view === 'week'
      ? `Semaine du ${labelDay(from)}` : `À partir du ${labelDay(from)}`;
    try {
      const [cals, occ] = await Promise.all([
        this.api.get<Calendars>('/calendars'),
        this.api.get<{ occurrences: Occurrence[] }>('/occurrences', { from: formatDate(from), to: formatDate(addDays(from, days)) }),
      ]);
      this.calendars = cals.calendars;
      const ctx: ViewContext = {
        colorOf: this.colorOf,
        today: nowIn(this.zone),
        onOpen: (o) => void this.open(o),
        onNew: (start, allDay) => void this.create(start, allDay),
      };
      let content: HTMLElement;
      if (this.view === 'week') content = week(occ.occurrences, from, ctx);
      else if (this.view === 'month') content = month(occ.occurrences, first, from, ctx);
      else content = agenda(occ.occurrences, from, days, ctx);
      const notes: HTMLElement[] = [];
      if (this.calendars.length === 0) {
        notes.push(h('p', { class: 'banner' }, "Vous n'avez pas encore de calendrier. ", h('a', { href: '/calendars' }, 'En créer un')));
      }
      this.body.replaceChildren(...notes, content);
    } catch (e) {
      this.body.replaceChildren(h('p', { class: 'banner error' }, e instanceof Error ? e.message : String(e)));
    }
  }

  /** Reloads a little later, so a burst of changes is one reload. */
  private soon(): void {
    if (this.reloadTimer !== null) clearTimeout(this.reloadTimer);
    this.reloadTimer = setTimeout(() => void this.load(), 300);
  }

  private live(): void {
    const ES = this.doc.defaultView?.EventSource;
    if (ES === undefined) return;
    const es = new ES('/api/live');
    for (const ev of ['changed', 'access', 'resync']) es.addEventListener(ev, () => this.soon());
    this.bag.defer(() => es.close());
  }

  private async pickCalendars(): Promise<void> {
    const boxes = this.calendars.filter((c) => !c.hidden).map((c) => ({ c, box: h('input', { type: 'checkbox', checked: c.shown }) }));
    const v = await dialog(this.doc, 'Calendriers affichés', [
      h('div', { class: 'cal-picker' }, ...boxes.map(({ c, box }) => h('label', { class: 'checks' }, box, swatch(c.color), c.name,
        canWrite(c) ? null : h('span', { class: 'chip' }, 'lecture'),
        c.access !== 'owner' ? h('span', { class: 'muted' }, ` · ${c.owner}`) : null))),
    ], [{ label: 'Annuler', value: '' }, { label: 'Afficher', value: 'ok', kind: 'filled' }]);
    if (v !== 'ok') return;
    for (const { c, box } of boxes) {
      if (box.checked !== c.shown) {
        try {
          await this.api.patch(`/calendars/${seg(c.ref)}`, { shown: box.checked });
        } catch (e) {
          this.toasts.error(e);
        }
      }
    }
    await this.load();
  }

  private async create(start: Wall, allDay: boolean): Promise<void> {
    const target = this.calendars.find((c) => canWrite(c) && c.shown) ?? this.calendars.find(canWrite);
    if (target === undefined) {
      this.toasts.show('Créez d’abord un calendrier (page Calendriers).');
      return;
    }
    const res = await editEvent(this.doc, this.calendars, {
      calendar: target.ref, summary: '', location: '', description: '', allDay, start, end: allDay ? addDays(start, 1) : addMinutes(start, 60),
      rrule: null, ruleEditable: true, isNew: true,
    });
    if (res === null) return;
    try {
      await this.api.post(`/calendars/${seg(res.calendar)}/events`, res.input);
      this.toasts.show('Événement ajouté.');
      await this.load();
    } catch (e) {
      this.toasts.error(e);
    }
  }

  private async open(o: Occurrence): Promise<void> {
    let ev: EventView;
    try {
      ev = await this.api.get<EventView>(`/calendars/${seg(o.calendar)}/events/${seg(o.name)}`);
    } catch (e) {
      this.toasts.error(e);
      return;
    }
    const cal = this.calendars.find((c) => c.ref === o.calendar);
    const choice = await showEvent(this.doc, o, ev, cal);
    if (choice === 'edit') await this.edit(o, ev);
    if (choice === 'delete') await this.remove(o, ev);
  }

  private path(o: Occurrence): string {
    return `/calendars/${seg(o.calendar)}/events/${seg(o.name)}`;
  }

  private async edit(o: Occurrence, ev: EventView): Promise<void> {
    const res = await editEvent(this.doc, this.calendars, {
      calendar: o.calendar, summary: o.summary, location: o.location, description: ev.description, allDay: o.all_day,
      start: parseLocal(o.start), end: parseLocal(o.end), rrule: ev.rrule || null, ruleEditable: ev.editable_rrule, isNew: false,
    });
    if (res === null) return;
    let scope: string = 'all';
    let occurrence = '';
    const input = { ...res.input };
    if (o.recurring && o.recurrence_id !== null) {
      const s = await askScope(this.doc, 'modify');
      if (s === null) return;
      scope = s;
      occurrence = o.recurrence_id;
      if (s === 'this') delete input.rrule;
      if (s === 'all') {
        // The form shows this occurrence; the series moves by as much as it did.
        const minutes = (a: string, b: string): number => (toNaive(parseLocal(b)) - toNaive(parseLocal(a))) / 60000;
        const start = addMinutes(parseLocal(ev.start), minutes(o.start, res.input.start));
        const end = addMinutes(start, minutes(res.input.start, res.input.end));
        const fmt = (w: Wall): string => (res.input.all_day ? formatDate(w) : formatLocal(w));
        input.start = fmt(start);
        input.end = fmt(end);
      }
    }
    try {
      if (res.calendar !== o.calendar && scope === 'all') {
        await this.api.post(`${this.path(o)}/move`, { to: res.calendar }, ev.etag);
        o = { ...o, calendar: res.calendar };
        ev = await this.api.get<EventView>(this.path(o));
      }
      await this.api.put(`${this.path(o)}`, input, ev.etag, { scope, occurrence });
      this.toasts.show('Enregistré.');
    } catch (e) {
      this.changedMeanwhile(e);
    }
    await this.load();
  }

  private changedMeanwhile(e: unknown): void {
    if (e instanceof ApiError && e.code === 'precondition_failed') {
      const by = typeof e.body['by'] === 'string' && e.body['by'] !== '' ? ` par ${e.body['by']}` : '';
      this.toasts.show(`Modifié entre-temps${by} : rouvrez-le pour voir la nouvelle version.`, 'error');
      return;
    }
    this.toasts.error(e);
  }

  private async remove(o: Occurrence, ev: EventView): Promise<void> {
    let scope = 'all';
    let occurrence = '';
    if (o.recurring && o.recurrence_id !== null) {
      const s = await askScope(this.doc, 'delete');
      if (s === null) return;
      scope = s;
      occurrence = o.recurrence_id;
    } else if (!(await confirmDialog(this.doc, `Supprimer « ${o.summary} » ?`, 'Il disparaît pour tous ceux qui voient ce calendrier.', 'Supprimer', true))) {
      return;
    }
    try {
      await this.api.deleteIf(this.path(o), ev.etag, { scope, occurrence });
      this.toasts.show('Supprimé.');
    } catch (e) {
      this.changedMeanwhile(e);
    }
    await this.load();
  }
}
