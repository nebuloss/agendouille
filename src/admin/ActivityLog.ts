import type { Api } from '@/shared/core/Api.ts';
import { h, type Child } from '@/shared/core/Dom.ts';
import { dialog } from '@/shared/components/Ui.ts';
import { icon } from '@/shared/components/icons.ts';
import { ui } from '@/shared/components/recipes.ts';
import { when } from '@/shared/render/format.ts';

/**
 * The activity log, as claudication's request log (web/src/ui/features/
 * requests there), in plain DOM: a table on its own tile, capped in height
 * with its heading pinned; the time column sorts; the columns the server can
 * filter (who, what, which calendar) open a menu of their values with their
 * counts; a value in a row narrows the log to itself; every filter in force
 * is a chip above the table that takes it off; a row opens its record; and
 * under the table, older rows, how many are shown, and the same rows as CSV.
 *
 * The filter lives in the URL (?user=&calendar=&kind=), so a link and Back
 * land on it. On a user's page the user is the page's scope: always applied,
 * never a chip, never in the URL.
 */

export interface Activity {
  at: string;
  kind: string;
  action: string;
  by: string;
  calendar?: string;
  via?: string;
  device?: string;
  detail?: string;
}

type Field = 'user' | 'kind' | 'calendar';

const ACTIONS: Record<string, string> = {
  created: 'créé', changed: 'modifié', deleted: 'supprimé', added: 'ajouté', removed: 'retiré', shared: 'partagé',
  unshared: 'partage retiré', disabled: 'désactivé', enabled: 'réactivé', groups: 'groupes changés', member_added: 'membre ajouté',
  member_removed: 'membre retiré', password: 'mot de passe changé', revoked: 'révoqué', made: 'créé', approved: 'approuvé',
  transferred: 'donné', calendar_created: 'calendrier créé', calendar_changed: 'calendrier modifié', calendar_deleted: 'calendrier supprimé',
};

export const KINDS: Record<string, string> = {
  event: 'événement', calendar: 'calendrier', share: 'partage', user: 'utilisateur', group: 'groupe', device: 'appareil',
  enrolment: 'QR de connexion', setting: 'réglage', admin: 'administration', approval: 'connexion',
};

const FIELD_WORDS: Record<Field, string> = { user: 'qui', kind: 'quoi', calendar: 'calendrier' };

const valueOf = (a: Activity, f: Field): string => (f === 'user' ? a.by : f === 'kind' ? a.kind : (a.calendar ?? ''));
const labelOf = (f: Field, v: string): string => (f === 'kind' ? (KINDS[v] ?? v) : v === '' ? 'aucun' : v);

/** What a row did, in words. */
export function what(a: Activity): string {
  return `${KINDS[a.kind] ?? a.kind} · ${ACTIONS[a.action] ?? a.action}`;
}

/** Same day: the time; else the date and the time (claudication's sameDayStamp). */
export function stamp(iso: string, now: Date = new Date()): string {
  const d = new Date(iso);
  const time = d.toLocaleTimeString('fr-FR', { hour: '2-digit', minute: '2-digit', second: '2-digit' });
  if (d.toDateString() === now.toDateString()) return time;
  return `${d.toLocaleDateString('fr-FR', { day: 'numeric', month: 'short' })} ${time}`;
}

const TH = 'sticky top-0 z-10 border-b border-outline-variant bg-surface-high px-3 py-2.5 text-left text-[11px] font-semibold tracking-wider whitespace-nowrap text-on-surface-variant uppercase first:pl-4 last:pr-4';
const TD = 'px-3 py-2 align-middle first:pl-4 last:pr-4';
const CELL_LINK = 'state-layer rounded-md3-xs px-1 underline decoration-dotted underline-offset-2 hover:text-primary';
const MENU = 'fixed z-50 max-h-80 min-w-56 overflow-y-auto overscroll-contain rounded-md3-l bg-surface-container p-1.5 text-sm elevation-2';
const MENU_ITEM = 'state-layer flex min-h-10 w-full cursor-pointer items-center gap-3 rounded-md3-m px-3 py-2 text-left text-sm text-on-surface';

export class ActivityLog {
  readonly root = h('div', { class: 'flex flex-col gap-4' });
  private items: Activity[] = [];
  private filter: Partial<Record<Field, string>>;
  private sort: 'desc' | 'asc' = 'desc';
  private exhausted = false;
  private menu: HTMLElement | null = null;
  private readonly onPop = (): void => {
    this.filter = this.fromURL();
    void this.load();
  };

  /** scope: the page's own user, applied always and never shown as a filter. */
  constructor(
    private readonly doc: Document,
    private readonly api: Api,
    private readonly scope: { user?: string } = {},
  ) {
    this.filter = this.fromURL();
    this.doc.defaultView?.addEventListener('popstate', this.onPop);
  }

  dispose(): void {
    this.doc.defaultView?.removeEventListener('popstate', this.onPop);
    this.closeMenu();
  }

  private fromURL(): Partial<Record<Field, string>> {
    const q = new URLSearchParams(this.doc.location.search);
    const f: Partial<Record<Field, string>> = {};
    for (const k of ['user', 'kind', 'calendar'] as const) {
      const v = q.get(k);
      if (v !== null && v !== '' && !(k === 'user' && this.scope.user !== undefined)) f[k] = v;
    }
    return f;
  }

  private query(before?: string): Record<string, string | undefined> {
    return { ...this.filter, user: this.scope.user ?? this.filter.user, before };
  }

  /** Narrows (or widens) the log, keeping the URL in step. */
  private narrow(next: Partial<Record<Field, string>>): void {
    this.filter = next;
    const q = new URLSearchParams();
    for (const [k, v] of Object.entries(next)) if (v !== undefined && v !== '') q.set(k, v);
    const s = q.toString();
    this.doc.defaultView?.history.pushState(null, '', this.doc.location.pathname + (s === '' ? '' : `?${s}`));
    void this.load();
  }

  async load(): Promise<void> {
    try {
      const { activity } = await this.api.get<{ activity: Activity[] }>('/activity', this.query());
      this.items = activity;
      this.exhausted = activity.length < 100;
      this.draw();
    } catch (e) {
      this.root.replaceChildren(h('p', { class: ui.error, role: 'alert' }, e instanceof Error ? e.message : String(e)));
    }
  }

  private async more(): Promise<void> {
    const last = this.items[this.items.length - 1];
    if (last === undefined) return;
    const { activity } = await this.api.get<{ activity: Activity[] }>('/activity', this.query(last.at));
    this.items = this.items.concat(activity);
    this.exhausted = activity.length < 100;
    this.draw();
  }

  private draw(): void {
    this.closeMenu();
    const chips = (Object.entries(this.filter) as [Field, string][]).filter(([, v]) => v !== undefined && v !== '').map(([f, v]) => {
      const b = h('button', { type: 'button', title: `Ne plus filtrer par ce ${FIELD_WORDS[f]}`,
        class: 'state-layer inline-flex h-7 items-center gap-1.5 rounded-md3-s border border-outline px-2.5 text-xs font-medium text-on-surface-variant' },
      `${FIELD_WORDS[f]} : ${labelOf(f, v)}`, h('span', { class: 'text-base leading-none', 'aria-hidden': 'true' }, '×'));
      b.addEventListener('click', () => {
        const next = { ...this.filter };
        delete next[f];
        this.narrow(next);
      });
      return b;
    });
    const rows = [...this.items].sort((a, b) => (this.sort === 'desc' ? b.at.localeCompare(a.at) : a.at.localeCompare(b.at)));
    const parts: Child[] = [
      chips.length > 0 ? h('div', { class: 'flex flex-wrap items-center gap-2 text-sm' }, h('span', { class: 'text-on-surface-variant' }, 'Restreint à'), ...chips) : null,
      rows.length === 0
        ? h('p', { class: ui.empty }, chips.length > 0 ? 'Rien ne correspond à ce filtre.' : "Rien dans l'historique.")
        : this.table(rows),
    ];
    if (rows.length > 0) parts.push(this.footer(rows.length));
    this.root.replaceChildren(...parts.filter((p): p is Node => p !== null && p !== undefined && p !== false && typeof p !== 'string'));
  }

  private table(rows: Activity[]): HTMLElement {
    const sortBtn = h('button', { type: 'button', class: 'state-layer -mx-1 inline-flex items-center gap-1 rounded-md3-xs px-1 uppercase' }, 'Quand',
      h('span', { 'aria-hidden': 'true' }, this.sort === 'desc' ? '↓' : '↑'));
    sortBtn.addEventListener('click', () => {
      this.sort = this.sort === 'desc' ? 'asc' : 'desc';
      this.draw();
    });
    const head = h('tr', {},
      h('th', { class: TH, 'aria-sort': this.sort === 'desc' ? 'descending' : 'ascending' }, sortBtn),
      this.facetHead('Qui', 'user'),
      this.facetHead('Quoi', 'kind'),
      this.facetHead('Calendrier', 'calendar'),
      h('th', { class: TH }, 'Par'),
      h('th', { class: TH }, 'Détail'));
    const body = h('tbody', {}, ...rows.map((a) => this.row(a)));
    return h('div', { class: 'isolate max-h-[34rem] overflow-auto rounded-md3-l bg-surface-lowest ring-1 ring-outline-variant/60' },
      h('table', { class: 'w-full min-w-max text-sm [&>tbody>tr>td]:border-r [&>tbody>tr>td]:border-outline-variant/50 [&>tbody>tr>td:last-child]:border-r-0 [&>thead>tr>th]:border-r [&>thead>tr>th]:border-outline-variant/50 [&>thead>tr>th:last-child]:border-r-0' },
        h('thead', {}, head), body));
  }

  /** A heading that opens the values its column holds. */
  private facetHead(label: string, field: Field): HTMLElement {
    const scoped = field === 'user' && this.scope.user !== undefined;
    if (scoped) return h('th', { class: TH }, label);
    const on = this.filter[field] !== undefined;
    const b = h('button', { type: 'button', 'aria-haspopup': 'menu',
      class: `state-layer -mx-1 inline-flex items-center gap-1 rounded-md3-xs px-1 uppercase ${on ? 'text-primary' : ''}` },
    label, on ? h('span', { class: '[&_.ic]:size-3.5' }, icon('check')) : h('span', { 'aria-hidden': 'true', class: 'text-[9px]' }, '▾'));
    b.addEventListener('click', (e) => {
      e.stopPropagation();
      this.openMenu(b, field);
    });
    return h('th', { class: TH }, b);
  }

  private openMenu(anchor: HTMLElement, field: Field): void {
    this.closeMenu();
    const counts = new Map<string, number>();
    for (const a of this.items) counts.set(valueOf(a, field), (counts.get(valueOf(a, field)) ?? 0) + 1);
    const chosen = this.filter[field];
    if (chosen !== undefined && !counts.has(chosen)) counts.set(chosen, 0);
    const entries = [...counts.entries()].sort((x, y) => y[1] - x[1] || x[0].localeCompare(y[0]));
    const menu = h('div', { class: MENU, role: 'menu' });
    if (chosen !== undefined) {
      const clear = h('button', { type: 'button', class: MENU_ITEM, role: 'menuitem' }, 'Tout afficher');
      clear.addEventListener('click', () => {
        const next = { ...this.filter };
        delete next[field];
        this.narrow(next);
      });
      menu.append(clear, h('div', { role: 'separator', class: '-mx-1.5 my-1.5 border-t border-outline-variant' }));
    }
    if (entries.length === 0) menu.append(h('p', { class: 'px-3 py-2 text-xs text-on-surface-variant' }, 'Rien dans cette colonne.'));
    for (const [v, n] of entries) {
      const item = h('button', { type: 'button', role: 'menuitemradio', 'aria-checked': String(v === chosen),
        class: `${MENU_ITEM} ${v === chosen ? 'bg-secondary-container text-on-secondary-container' : ''}` },
      h('span', { class: 'min-w-0 flex-1 truncate' }, labelOf(field, v)),
      n > 0 ? h('span', { class: 'shrink-0 text-xs tabular-nums text-on-surface-variant' }, String(n)) : null);
      item.addEventListener('click', () => this.narrow({ ...this.filter, [field]: v }));
      menu.append(item);
    }
    menu.append(h('p', { class: 'px-3 pt-1.5 pb-1 text-[11px] text-on-surface-variant' }, 'Parmi les lignes affichées.'));
    const r = anchor.getBoundingClientRect();
    menu.style.setProperty('top', `${Math.round(r.bottom + 4)}px`);
    menu.style.setProperty('left', `${Math.round(Math.min(r.left, (this.doc.defaultView?.innerWidth ?? 800) - 240))}px`);
    this.doc.body.append(menu);
    this.menu = menu;
    const away = (e: Event): void => {
      if (e instanceof KeyboardEvent && e.key !== 'Escape') return;
      if (e.type === 'pointerdown' && menu.contains(e.target as Node)) return;
      this.closeMenu();
    };
    for (const t of ['pointerdown', 'keydown', 'scroll']) this.doc.addEventListener(t, away, { capture: true });
    this.closeMenuListeners = () => {
      for (const t of ['pointerdown', 'keydown', 'scroll']) this.doc.removeEventListener(t, away, { capture: true });
    };
  }

  private closeMenuListeners: (() => void) | null = null;

  private closeMenu(): void {
    this.menu?.remove();
    this.menu = null;
    this.closeMenuListeners?.();
    this.closeMenuListeners = null;
  }

  /** A value that narrows the log to itself; the click does not open the row. */
  private narrowing(field: Field, value: string, shown: string): Child {
    if (value === '') return '—';
    if (field === 'user' && this.scope.user !== undefined) return shown;
    const b = h('button', { type: 'button', class: CELL_LINK, title: `Seulement ce ${FIELD_WORDS[field]}` }, shown);
    b.addEventListener('click', (e) => {
      e.stopPropagation();
      this.narrow({ ...this.filter, [field]: value });
    });
    return b;
  }

  private row(a: Activity): HTMLElement {
    const tr = h('tr', { class: 'cursor-pointer border-b border-outline-variant/50 last:border-0 hover:bg-surface-low' },
      h('td', { class: `${TD} whitespace-nowrap tabular-nums text-on-surface-variant`, title: a.at }, stamp(a.at)),
      h('td', { class: TD }, this.narrowing('user', a.by, a.by || '—')),
      h('td', { class: `${TD} whitespace-nowrap` }, this.narrowing('kind', a.kind, KINDS[a.kind] ?? a.kind), ` · ${ACTIONS[a.action] ?? a.action}`),
      h('td', { class: `${TD} font-mono text-[12.5px]` }, this.narrowing('calendar', a.calendar ?? '', a.calendar ?? '')),
      h('td', { class: `${TD} text-on-surface-variant` }, [a.via, a.device].filter((x) => x !== undefined && x !== '').join(' · ') || '—'),
      h('td', { class: `${TD} max-w-[22rem] truncate text-on-surface-variant`, title: a.detail ?? '' }, a.detail || '—'));
    tr.addEventListener('click', () => void this.open(a));
    return tr;
  }

  /** A row's whole record, in a dialog. */
  private async open(a: Activity): Promise<void> {
    const line = (k: string, v: string | undefined): Child[] => [h('dt', {}, k), h('dd', {}, v === undefined || v === '' ? '—' : v)];
    await dialog(this.doc, what(a), [h('dl', { class: ui.kv },
      ...line('Quand', when(a.at)), ...line('Qui', a.by), ...line('Quoi', what(a)), ...line('Calendrier', a.calendar),
      ...line('Par', a.via), ...line('Appareil', a.device), ...line('Détail', a.detail))],
    [{ label: 'Fermer', value: '', kind: 'filled' }]);
  }

  private footer(shown: number): HTMLElement {
    const more = h('button', { type: 'button', class: ui.btn('tonal') }, 'Plus ancien');
    more.addEventListener('click', () => {
      more.disabled = true;
      this.more().catch(() => undefined).finally(() => {
        more.disabled = false;
      });
    });
    const q = new URLSearchParams();
    for (const [k, v] of Object.entries(this.query())) if (v !== undefined && v !== '') q.set(k, v);
    const s = q.toString();
    return h('div', { class: 'flex flex-wrap items-center gap-3 border-t border-outline-variant pt-4' },
      this.exhausted ? h('span', { class: 'text-xs text-on-surface-variant' }, "C'est tout ce qui est gardé.") : more,
      h('span', { class: 'text-xs text-on-surface-variant', 'aria-live': 'polite' }, `${shown} affichées`),
      h('span', { class: 'grow' }),
      h('a', { class: ui.btn('text'), href: this.api.url(`/activity/export${s === '' ? '' : `?${s}`}`), download: 'agendouille-activite.csv' }, icon('download'), 'Télécharger en CSV'));
  }
}
