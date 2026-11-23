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

const FIELD_WORDS: Record<Field, string> = { user: 'qui', kind: 'type', calendar: 'calendrier' };

const valueOf = (a: Activity, f: Field): string => (f === 'user' ? a.by : f === 'kind' ? a.kind : (a.calendar ?? ''));
const labelOf = (f: Field, v: string): string => (f === 'kind' ? (KINDS[v] ?? v) : v === '' ? 'aucun' : v);

/** What a row did, in words. */
export function what(a: Activity): string {
  return `${KINDS[a.kind] ?? a.kind} · ${ACTIONS[a.action] ?? a.action}`;
}

/** What the columns show for one entry: every kind fills every column. */
export interface Described {
  who: string; // the actor, as shown: a user name, "administration", "console"
  actor?: string; // a user name the log can narrow to
  verb: string; // "a créé un événement"
  subject: string; // what it is about: a calendar, a user, a group, a setting, a device
  calendar?: string; // when the subject is a calendar: narrows the log
  href?: string; // where the subject's page is
  extra?: string; // a word more: the share's access, the setting's new value
  source: string; // where it came from: a device, CalDAV, the web app, the console
}

const VIA: Record<string, string> = { caldav: 'CalDAV', dav: 'CalDAV', web: 'application web', app: "l'app agendouille", import: 'import', ics: 'lien .ics', cli: 'console', test: 'test' };
const ACCESS_WORD: Record<string, string> = { read: 'lecture', readwrite: 'lecture et écriture' };

const VERBS: Record<string, string> = {
  'event created': 'a créé un événement', 'event changed': 'a modifié un événement', 'event deleted': 'a supprimé un événement',
  'calendar created': 'a créé un calendrier', 'calendar deleted': 'a supprimé un calendrier', 'calendar transferred': 'a donné un calendrier',
  'calendar changed': 'a modifié un calendrier',
  'share shared': 'a partagé un calendrier', 'share unshared': 'a retiré un partage',
  'user added': 'a ajouté un utilisateur', 'user changed': 'a modifié un utilisateur', 'user removed': 'a supprimé un utilisateur',
  'user disabled': 'a désactivé un utilisateur', 'user enabled': 'a réactivé un utilisateur', 'user groups': 'a changé les groupes de',
  'group added': 'a créé un groupe', 'group changed': 'a modifié un groupe', 'group removed': 'a supprimé un groupe',
  'group member_added': 'a ajouté au groupe', 'group member_removed': 'a retiré du groupe',
  'device revoked': 'a révoqué un appareil', 'enrolment made': 'a créé un QR de connexion pour',
  'setting changed': 'a changé un réglage', 'admin password': "a changé le mot de passe d'administration",
  'approval approved': 'a connecté un navigateur',
};

export function describe(a: Activity): Described {
  const parts = (a.detail ?? '').split(' ');
  const first = parts[0] ?? '';
  const who = a.by === 'admin' ? 'administration' : a.by === 'cli' || a.by === '' ? 'console' : a.by;
  const actor = a.by !== 'admin' && a.by !== 'cli' && a.by !== '' ? a.by : undefined;
  let source: string;
  if (a.kind === 'event') source = a.device ? `${a.device} · ${VIA[a.via ?? ''] ?? a.via ?? ''}` : (VIA[a.via ?? ''] ?? (a.via || 'console'));
  else if (a.by === 'admin') source = "interface d'administration";
  else if (a.by === 'cli' || a.by === '') source = 'ligne de commande';
  else source = 'application web';
  const d: Described = { who, actor, verb: VERBS[`${a.kind} ${a.action}`] ?? `${ACTIONS[a.action] ?? a.action}`, subject: '—', source };
  const user = (n: string): void => {
    d.subject = n;
    d.href = `/users/${encodeURIComponent(n)}`;
  };
  switch (a.kind) {
    case 'event':
    case 'calendar':
      d.subject = a.calendar || first || '—';
      if (a.calendar) d.calendar = a.calendar;
      if (a.action === 'transferred') d.extra = (a.detail ?? '').split(' -> ')[1] !== undefined ? `à ${(a.detail ?? '').split(' -> ')[1]}` : undefined;
      break;
    case 'share':
      d.subject = a.calendar || '—';
      if (a.calendar) d.calendar = a.calendar;
      d.extra = `${first}${parts[1] ? ` · ${ACCESS_WORD[parts[1]] ?? parts[1]}` : ''}`;
      break;
    case 'user':
      user(first);
      if (a.action === 'groups') d.extra = parts[1] ? parts[1].split(',').join(', ') : 'aucun groupe';
      break;
    case 'enrolment':
      user(first);
      break;
    case 'group':
      d.subject = first;
      d.href = `/groups/${encodeURIComponent(first)}`;
      if (parts[1]) d.extra = parts[1];
      break;
    case 'device':
      d.subject = `appareil n° ${first}`;
      break;
    case 'setting': {
      d.subject = first;
      d.href = '/settings';
      const arrow = (a.detail ?? '').indexOf(' -> ');
      if (arrow >= 0) d.extra = `→ ${(a.detail ?? '').slice(arrow + 4)}`;
      break;
    }
    case 'admin':
      d.subject = "mot de passe d'administration";
      break;
    case 'approval':
      d.subject = browserOf(a.detail ?? '');
      break;
  }
  return d;
}

/** A browser's name from its user agent, roughly. */
function browserOf(ua: string): string {
  const os = /Android/.test(ua) ? 'Android' : /iPhone|iPad/.test(ua) ? 'iOS' : /Mac OS X/.test(ua) ? 'Mac' : /Windows/.test(ua) ? 'Windows' : /Linux/.test(ua) ? 'Linux' : '';
  const b = /Edg\//.test(ua) ? 'Edge' : /Firefox\//.test(ua) ? 'Firefox' : /Chrome\//.test(ua) ? 'Chrome' : /Safari\//.test(ua) ? 'Safari' : 'navigateur';
  return os === '' ? b : `${b}, ${os}`;
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

  /** scope: the page's own user or calendar, applied always and never shown as a filter. */
  constructor(
    private readonly doc: Document,
    private readonly api: Api,
    private readonly scope: Partial<Record<Field, string>> = {},
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
      if (v !== null && v !== '' && this.scope[k] === undefined) f[k] = v;
    }
    return f;
  }

  private query(before?: string): Record<string, string | undefined> {
    return { ...this.filter, ...this.scope, before };
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
      this.facetHead('Action', 'kind'),
      this.facetHead('Sur', 'calendar'),
      h('th', { class: TH }, 'Depuis'));
    const body = h('tbody', {}, ...rows.map((a) => this.row(a)));
    return h('div', { class: 'isolate max-h-[34rem] overflow-auto rounded-md3-l bg-surface-lowest ring-1 ring-outline-variant/60' },
      h('table', { class: 'w-full min-w-max text-sm [&>tbody>tr>td]:border-r [&>tbody>tr>td]:border-outline-variant/50 [&>tbody>tr>td:last-child]:border-r-0 [&>thead>tr>th]:border-r [&>thead>tr>th]:border-outline-variant/50 [&>thead>tr>th:last-child]:border-r-0' },
        h('thead', {}, head), body));
  }

  /** A heading that opens the values its column holds. */
  private facetHead(label: string, field: Field): HTMLElement {
    const scoped = this.scope[field] !== undefined;
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
    if (this.scope[field] !== undefined) return shown;
    const b = h('button', { type: 'button', class: CELL_LINK, title: `Seulement ce ${FIELD_WORDS[field]}` }, shown);
    b.addEventListener('click', (e) => {
      e.stopPropagation();
      this.narrow({ ...this.filter, [field]: value });
    });
    return b;
  }

  private row(a: Activity): HTMLElement {
    const d = describe(a);
    const subject: Node[] = [];
    if (d.calendar !== undefined) subject.push(this.asNode(this.narrowing('calendar', d.calendar, d.subject)));
    else if (d.href !== undefined) subject.push(h('a', { href: d.href, class: CELL_LINK }, d.subject));
    else subject.push(document.createTextNode(d.subject));
    if (d.extra !== undefined) subject.push(h('span', { class: 'ml-1.5 text-xs text-on-surface-variant' }, d.extra));
    const tr = h('tr', { class: 'cursor-pointer border-b border-outline-variant/50 last:border-0 hover:bg-surface-low' },
      h('td', { class: `${TD} whitespace-nowrap tabular-nums text-on-surface-variant`, title: a.at }, stamp(a.at)),
      h('td', { class: `${TD} whitespace-nowrap` }, d.actor !== undefined ? this.narrowing('user', d.actor, d.actor) : h('span', { class: 'italic text-on-surface-variant' }, d.who)),
      h('td', { class: `${TD} whitespace-nowrap`, title: KINDS[a.kind] ?? a.kind }, d.verb),
      h('td', { class: `${TD} max-w-[18rem] truncate` }, ...subject),
      h('td', { class: `${TD} whitespace-nowrap text-on-surface-variant` }, d.source));
    tr.addEventListener('click', () => void this.open(a));
    return tr;
  }

  private asNode(c: Child): Node {
    return typeof c === 'string' ? document.createTextNode(c) : (c as Node);
  }

  /** A row's whole record, in a dialog. */
  private async open(a: Activity): Promise<void> {
    const line = (k: string, v: string | undefined): Child[] => [h('dt', {}, k), h('dd', {}, v === undefined || v === '' ? '—' : v)];
    const d = describe(a);
    await dialog(this.doc, `${d.who} ${d.verb}`, [h('dl', { class: ui.kv },
      ...line('Quand', when(a.at)), ...line('Qui', d.who), ...line('Action', what(a)), ...line('Sur', d.extra !== undefined ? `${d.subject} (${d.extra})` : d.subject),
      ...line('Depuis', d.source), ...line('Calendrier', a.calendar), ...line('Détail brut', a.detail))],
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
