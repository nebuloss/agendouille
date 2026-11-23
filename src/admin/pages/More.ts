import { ui } from '@/shared/components/recipes.ts';
import { ApiError, seg } from '@/shared/core/Api.ts';
import { h, type Child } from '@/shared/core/Dom.ts';
import { action, card, confirmDialog, dialog, field, formDialog, section, swatch, table } from '@/shared/components/Ui.ts';
import { bytes, when } from '@/shared/render/format.ts';
import { AdminPage } from '@/admin/pages/base.ts';

interface Group {
  name: string;
  display_name: string;
  color: string;
  members: string[];
  shares: number;
}

interface UserLite {
  name: string;
  display_name: string;
}

/** Groups: who sees whom (PLAN.md 4.1). */
export class GroupsPage extends AdminPage {
  protected async render(): Promise<Child[]> {
    const [{ groups }, { users }] = await Promise.all([
      this.api.get<{ groups: Group[] }>('/groups'),
      this.api.get<{ users: UserLite[] }>('/users'),
    ]);
    return [
      card('Groupes', [action('Nouveau groupe', () => this.create(), 'filled', 'plus')],
      h('p', { class: 'text-sm text-on-surface-variant' }, "Les membres d'un groupe se voient et peuvent partager leurs calendriers entre eux."),
      table(['Groupe', 'Membres', 'Partages', ''],
        groups.map((g) => [
          h('a', { href: `/groups/${seg(g.name)}`, class: ui.link }, swatch(g.color), g.display_name, ' ', h('span', { class: 'text-sm font-normal text-on-surface-variant' }, `(${g.name})`)),
          h('span', {}, ...g.members.map((m) => h('span', { class: 'mr-1 inline-flex h-6 items-center gap-1 rounded-md3-s bg-surface-high px-2 text-xs font-semibold text-on-surface' }, m))),
          String(g.shares),
          h('span', { class: 'flex flex-wrap items-center gap-2' },
            action('Membres…', () => this.members(g, users), 'text'),
            action('Supprimer', () => this.remove(g), 'text')),
        ]), 'Aucun groupe.')),
    ];
  }

  /** "Nouveau groupe": the form in a modal. */
  private async create(): Promise<void> {
    const name = h('input', { required: true, pattern: '[a-z][a-z0-9-]{0,31}', placeholder: 'famille', autocomplete: 'off' });
    const display = h('input', { placeholder: 'Famille' });
    const ok = await formDialog(this.doc, 'Nouveau groupe', [
      field('Identifiant', name, 'Minuscules, chiffres et tirets ; ne change plus.'),
      field('Nom affiché', display),
    ], 'Créer', async () => {
      await this.api.post('/groups', { name: name.value.trim(), display_name: display.value.trim() });
    });
    if (!ok) return;
    this.toasts.show('Groupe créé.');
    this.doc.dispatchEvent(new Event('agd:changed'));
    await this.refresh();
  }

  private async members(g: Group, users: UserLite[]): Promise<void> {
    const checks = users.map((u) => h('input', { type: 'checkbox', value: u.name, checked: g.members.includes(u.name) }));
    const v = await dialog(this.doc, `Membres de ${g.display_name}`,
      [h('div', { class: 'flex flex-wrap gap-x-5 gap-y-2' }, ...users.map((u, i) => h('label', { class: ui.check }, checks[i]!, u.display_name)))],
      [{ label: 'Annuler', value: 'no' }, { label: 'Enregistrer', value: 'yes', kind: 'filled' }]);
    if (v !== 'yes') return;
    await this.act(() => this.api.patch(`/groups/${seg(g.name)}`, { members: checks.filter((c) => c.checked).map((c) => c.value) }), 'Membres enregistrés.');
  }

  private async remove(g: Group): Promise<void> {
    const text = g.shares > 0
      ? `${g.shares} partage(s) de calendrier passent par ce groupe ; ils disparaîtront.`
      : 'Ses membres ne se verront plus par ce groupe.';
    if (!(await confirmDialog(this.doc, `Supprimer ${g.display_name} ?`, text, 'Supprimer', true))) return;
    await this.act(() => this.api.delete(`/groups/${seg(g.name)}`, { confirm: '1' }), 'Groupe supprimé.');
  }
}

interface CalendarShared {
  ref: string;
  name: string;
  owner: string;
  color: string;
  events: number;
  shares: { target: string; access: string }[];
}

/**
 * One group, as claudication shows an item: its overview at /groups/<name>
 * (name, colour, what it is for, delete), and its pages beside it in the
 * rail: Membres, Calendriers (those shared with it).
 */
export class GroupPage extends AdminPage {
  private get sub(): string {
    return this.root.dataset['sub'] ?? '';
  }

  private path(): string {
    return `/groups/${seg(this.name)}`;
  }

  protected async render(): Promise<Child[]> {
    const [{ groups }, { users }] = await Promise.all([
      this.api.get<{ groups: Group[] }>('/groups'),
      this.api.get<{ users: UserLite[] }>('/users'),
    ]);
    const g = groups.find((x) => x.name === this.name);
    if (g === undefined) return [h('p', { class: ui.error, role: 'alert' }, `Pas de groupe « ${this.name} ».`)];
    for (const el of this.doc.querySelectorAll<HTMLElement>('[data-item-name] span')) el.textContent = g.display_name || g.name;
    switch (this.sub) {
      case 'members':
        return [this.members(g, users)];
      case 'calendars':
        return [await this.calendars(g)];
      default:
        return this.overview(g, users);
    }
  }

  private overview(g: Group, users: UserLite[]): Child[] {
    const display = h('input', { value: g.display_name });
    const color = h('input', { type: 'color', value: /^#[0-9a-f]{6}$/i.test(g.color) ? g.color : '#4a6572' });
    const form = h('form', { class: 'flex flex-col items-start gap-4 [&>div]:w-full' },
      h('div', { class: 'grid gap-4 [grid-template-columns:repeat(auto-fill,minmax(min(220px,100%),1fr))]' }, field('Nom affiché', display), field('Couleur', color)),
      h('button', { class: ui.btn('tonal'), type: 'submit' }, 'Enregistrer'));
    form.addEventListener('submit', (ev) => {
      ev.preventDefault();
      void this.act(() => this.api.patch(this.path(), { display_name: display.value.trim(), color: color.value }), 'Enregistré.');
    });
    const names = new Map(users.map((u) => [u.name, u.display_name || u.name]));
    return [
      h('section', { class: `${ui.card} flex-row items-center gap-4` },
        swatch(g.color, 'md'),
        h('div', { class: 'flex min-w-0 flex-1 flex-col' }, h('h2', { class: 'truncate text-2xl font-normal' }, g.display_name || g.name),
          h('span', { class: 'text-sm text-on-surface-variant' }, `Identifiant ${g.name} · ses membres se voient et peuvent partager leurs calendriers entre eux.`))),
      h('div', { class: 'grid gap-3 [grid-template-columns:repeat(auto-fill,minmax(150px,1fr))]' },
        tileLink(String(g.members.length), 'membres', `${this.path()}/members`),
        tileLink(String(g.shares), 'partages de calendrier', `${this.path()}/calendars`)),
      g.members.length > 0
        ? section('Membres', h('div', { class: 'flex flex-wrap gap-1.5' }, ...g.members.map((m) => h('a', { href: `/users/${seg(m)}`, class: 'inline-flex h-7 items-center rounded-md3-s bg-surface-high px-2.5 text-xs font-semibold text-on-surface no-underline hover:bg-surface-highest' }, names.get(m) ?? m))))
        : null,
      section('Profil', form),
      section('Supprimer le groupe',
        h('p', { class: 'text-sm text-on-surface-variant' }, g.shares > 0 ? `${g.shares} partage(s) de calendrier passent par ce groupe ; ils disparaîtront.` : 'Ses membres ne se verront plus par ce groupe.'),
        action('Supprimer…', () => this.remove(g), 'danger', 'trash')),
    ];
  }

  private members(g: Group, users: UserLite[]): HTMLElement {
    const checks = users.map((u) => h('input', { type: 'checkbox', value: u.name, checked: g.members.includes(u.name) }));
    const form = h('form', { class: 'flex flex-col items-start gap-4' },
      users.length === 0 ? h('p', { class: ui.empty }, 'Aucun utilisateur.')
        : h('div', { class: 'flex flex-wrap gap-x-6 gap-y-3' }, ...users.map((u, i) => h('label', { class: ui.check }, checks[i]!, u.display_name || u.name))),
      h('button', { class: ui.btn('filled'), type: 'submit' }, 'Enregistrer les membres'));
    form.addEventListener('submit', (ev) => {
      ev.preventDefault();
      void this.act(() => this.api.patch(this.path(), { members: checks.filter((c) => c.checked).map((c) => c.value) }), 'Membres enregistrés.');
    });
    return section('Membres', h('p', { class: 'text-sm text-on-surface-variant' }, "Ceux qui sont dans le groupe se voient, et peuvent partager leurs calendriers avec lui."), form);
  }

  private async calendars(g: Group): Promise<HTMLElement> {
    const { calendars } = await this.api.get<{ calendars: CalendarShared[] }>('/calendars');
    const target = `g:${g.name}`;
    const shared = calendars.filter((c) => c.shares.some((s) => s.target === target));
    return section('Calendriers partagés avec le groupe',
      h('p', { class: 'text-sm text-on-surface-variant' }, "L'administration voit les calendriers, jamais leurs événements."),
      table(['Calendrier', 'Propriétaire', 'Accès', 'Événements', ''],
        shared.map((c) => {
          const s = c.shares.find((x) => x.target === target)!;
          return [
            h('span', { class: 'inline-flex items-center' }, swatch(c.color), c.name),
            h('a', { href: `/users/${seg(c.owner)}/calendars`, class: ui.link }, c.owner),
            accessLabel(s.access),
            String(c.events),
            action('Retirer le partage', () => this.act(() => this.api.delete(`/calendars/${seg(c.ref)}/shares/${seg(target)}`), 'Partage retiré.'), 'text', 'close', 'sm'),
          ];
        }), 'Aucun calendrier partagé avec ce groupe.'));
  }

  private async remove(g: Group): Promise<void> {
    const text = g.shares > 0
      ? `${g.shares} partage(s) de calendrier passent par ce groupe ; ils disparaîtront.`
      : 'Ses membres ne se verront plus par ce groupe.';
    if (!(await confirmDialog(this.doc, `Supprimer ${g.display_name} ?`, text, 'Supprimer', true))) return;
    try {
      await this.api.delete(this.path(), { confirm: '1' });
      this.doc.location.assign('/groups');
    } catch (e) {
      this.toasts.error(e);
    }
  }
}

/** A count that opens its page. */
export function tileLink(n: string, label: string, href: string): HTMLElement {
  return h('a', { href, class: 'state-layer flex flex-col gap-1 rounded-md3-xl bg-surface-container p-4 text-on-surface no-underline' },
    h('span', { class: 'text-[32px] leading-10 font-semibold' }, n), h('span', { class: 'text-sm text-on-surface-variant' }, label));
}

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

const actions: Record<string, string> = {
  created: 'créé', changed: 'modifié', deleted: 'supprimé', added: 'ajouté', removed: 'retiré', shared: 'partagé',
  unshared: 'partage retiré', disabled: 'désactivé', enabled: 'réactivé', groups: 'groupes changés', member_added: 'membre ajouté',
  member_removed: 'membre retiré', password: 'mot de passe changé', revoked: 'révoqué', made: 'créé', approved: 'approuvé',
  transferred: 'donné', calendar_created: 'calendrier créé', calendar_changed: 'calendrier modifié', calendar_deleted: 'calendrier supprimé',
};

export const accessLabel = (a: string): string => ({ read: 'lecture', readwrite: 'lecture et écriture', owner: 'propriétaire' })[a] ?? a;

const kinds: Record<string, string> = {
  event: 'événement', calendar: 'calendrier', share: 'partage', user: 'utilisateur', group: 'groupe', device: 'appareil',
  enrolment: 'QR de connexion', setting: 'réglage', admin: 'administration', approval: 'connexion',
};

/** The activity as a table: when, who, what, where, through what. */
export function activityTable(items: Activity[]): HTMLElement {
  return table(['Quand', 'Qui', 'Quoi', 'Où', 'Par'],
    items.map((a) => [when(a.at), a.by, `${kinds[a.kind] ?? a.kind} · ${actions[a.action] ?? a.action}${a.detail ? ` · ${a.detail}` : ''}`,
      a.calendar ?? '', [a.via, a.device].filter((x) => x !== undefined && x !== '').join(' · ')]),
    "Rien dans l'historique.");
}

/** Who did what, never what an event said. */
export class ActivityPage extends AdminPage {
  private filters = { user: '', calendar: '', kind: '' };
  private items: Activity[] = [];

  protected async render(): Promise<Child[]> {
    const { activity } = await this.api.get<{ activity: Activity[] }>('/activity', this.filters);
    this.items = activity;
    return [this.filterForm(), this.tableOf(), this.more()];
  }

  private filterForm(): HTMLElement {
    const user = h('input', { value: this.filters.user, placeholder: 'lea' });
    const calendar = h('input', { value: this.filters.calendar, placeholder: 'maman~ecole' });
    const kind = h('select', {}, h('option', { value: '' }, 'Tout'),
      ...Object.entries(kinds).map(([k, l]) => h('option', { value: k, selected: k === this.filters.kind }, l)));
    const form = h('form', { class: 'grid gap-4 [grid-template-columns:repeat(auto-fill,minmax(min(220px,100%),1fr))]' }, field('Utilisateur', user), field('Calendrier', calendar), field('Quoi', kind),
      h('div', {}, h('button', { class: ui.btn('tonal'), type: 'submit' }, 'Filtrer')));
    form.addEventListener('submit', (ev) => {
      ev.preventDefault();
      this.filters = { user: user.value.trim(), calendar: calendar.value.trim(), kind: kind.value };
      void this.refresh();
    });
    return form;
  }

  private tableOf(): HTMLElement {
    return activityTable(this.items);
  }

  private more(): Child {
    if (this.items.length < 100) return null;
    return action('Plus ancien', async () => {
      const last = this.items[this.items.length - 1]!;
      try {
        const { activity } = await this.api.get<{ activity: Activity[] }>('/activity', { ...this.filters, before: last.at });
        this.items = this.items.concat(activity);
        await this.refreshKeepingItems();
      } catch (e) {
        this.toasts.error(e);
      }
    });
  }

  private async refreshKeepingItems(): Promise<void> {
    const keep = this.items;
    const content = this.root;
    const tbl = content.querySelector('.table-wrap, .empty');
    this.items = keep;
    tbl?.replaceWith(this.tableOf());
  }
}

interface Setting {
  key: string;
  group: string;
  type: 'string' | 'bool' | 'int' | 'list';
  default: unknown;
  value: unknown;
}

const groupNames: Record<string, string> = { network: 'Réseau', storage: 'Stockage et sauvegardes', limits: 'Limites', defaults: 'Valeurs par défaut' };

const hints: Record<string, string> = {
  public_listen: 'Adresse où écoutent CalDAV et l’application (celle vers laquelle le proxy de la DMZ renvoie), par exemple 0.0.0.0:8791.',
  public_url: 'Adresse publique, en https, telle que le proxy la sert : elle est dans chaque QR et chaque téléphone.',
  public_allow_insecure: 'Accepter http:// pour public_url (essais sur le réseau local seulement).',
  trusted_proxies: 'Adresses ou CIDR du proxy, séparés par des virgules : leur X-Forwarded-For est cru.',
  backup_schedule: 'Heure de la sauvegarde quotidienne (HH:MM, heure du serveur).',
};

/** Settings, applied live (PLAN.md 11.2). */
export class SettingsPage extends AdminPage {
  protected async render(): Promise<Child[]> {
    const { settings } = await this.api.get<{ settings: Setting[] }>('/settings');
    const inputs = new Map<string, () => unknown>();
    const groups = new Map<string, Child[]>();
    for (const s of settings) {
      let input: HTMLInputElement;
      let read: () => unknown;
      switch (s.type) {
        case 'bool':
          input = h('input', { type: 'checkbox', checked: s.value === true });
          read = () => input.checked;
          break;
        case 'int':
          input = h('input', { type: 'number', value: String(s.value), min: 0 });
          read = () => Number(input.value);
          break;
        case 'list':
          input = h('input', { value: (s.value as string[]).join(', ') });
          read = () => input.value.split(',').map((x) => x.trim()).filter((x) => x !== '');
          break;
        default:
          input = h('input', { value: String(s.value) });
          read = () => input.value.trim();
      }
      input.dataset['key'] = s.key;
      inputs.set(s.key, read);
      const list = groups.get(s.group) ?? [];
      list.push(field(s.key, input, hints[s.key] ?? `Par défaut : ${JSON.stringify(s.default)}`));
      groups.set(s.group, list);
    }
    const initial = new Map(settings.map((s) => [s.key, JSON.stringify(s.value)]));
    const form = h('form', { class: 'flex flex-col gap-4' },
      h('div', { class: ui.cards }, ...['network', 'storage', 'limits', 'defaults'].map((g) => section(groupNames[g] ?? g, ...(groups.get(g) ?? [])))),
      h('button', { class: ui.btn('filled'), type: 'submit' }, 'Appliquer'));
    form.addEventListener('submit', (ev) => {
      ev.preventDefault();
      const changes: Record<string, unknown> = {};
      for (const [k, read] of inputs) {
        const v = read();
        if (JSON.stringify(v) !== initial.get(k)) changes[k] = v;
      }
      if (Object.keys(changes).length === 0) {
        this.toasts.show('Rien à changer.');
        return;
      }
      void this.act(() => this.api.patch('/settings', changes), 'Réglages appliqués.');
    });
    return [h('p', { class: 'text-sm text-on-surface-variant' }, "Appliqués tout de suite, sans redémarrer. L'adresse de l'administration reste celle du fichier de démarrage."), form];
  }
}

interface Backup {
  name: string;
  size: number;
  made: string;
  before_restore: boolean;
}

/** Backups: list, make, download, restore (PLAN.md 13.6). */
export class BackupsPage extends AdminPage {
  protected async render(): Promise<Child[]> {
    const { backups } = await this.api.get<{ backups: Backup[] }>('/backups');
    return [
      h('div', { class: 'flex flex-wrap items-center gap-2' }, action('Sauvegarder maintenant', () => this.act(() => this.api.post('/backups'), 'Sauvegarde faite.'), 'filled')),
      table(['Sauvegarde', 'Faite', 'Taille', ''],
        backups.map((b) => [
          h('span', { class: 'font-mono text-[13px]' }, b.name, b.before_restore ? h('span', { class: 'mr-1 inline-flex h-6 items-center gap-1 rounded-md3-s bg-surface-high px-2 text-xs font-semibold text-on-surface' }, 'avant restauration') : null),
          when(b.made),
          bytes(b.size),
          h('span', { class: 'flex flex-wrap items-center gap-2' },
            h('a', { class: ui.btn('text'), href: this.api.url(`/backups/${seg(b.name)}`), download: b.name }, 'Télécharger'),
            action('Restaurer…', () => this.restore(b), 'text')),
        ]), 'Aucune sauvegarde pour le moment.'),
    ];
  }

  private async restore(b: Backup): Promise<void> {
    const ok = await confirmDialog(this.doc, 'Restaurer cette sauvegarde ?',
      `Le serveur redémarre avec la sauvegarde du ${when(b.made)}. Tout ce qui a changé depuis est perdu ; la base actuelle est gardée à part.`,
      'Restaurer', true);
    if (!ok) return;
    try {
      const res = await this.api.post<{ message: string }>(`/backups/${seg(b.name)}/restore`, { confirm: b.name });
      this.toasts.show(res.message);
      setTimeout(() => this.doc.location.assign('/signin'), 5000);
    } catch (e) {
      this.toasts.error(e instanceof ApiError ? e : new Error(String(e)));
    }
  }
}
