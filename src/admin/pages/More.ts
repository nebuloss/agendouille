import { ApiError, seg } from '@/shared/core/Api.ts';
import { h, type Child } from '@/shared/core/Dom.ts';
import { action, confirmDialog, dialog, field, section, swatch, table } from '@/shared/components/Ui.ts';
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
      h('p', { class: 'muted' }, "Les membres d'un groupe se voient et peuvent partager leurs calendriers entre eux."),
      table(['Groupe', 'Membres', 'Partages', ''],
        groups.map((g) => [
          h('span', {}, swatch(g.color), g.display_name, ' ', h('span', { class: 'muted' }, `(${g.name})`)),
          h('span', {}, ...g.members.map((m) => h('span', { class: 'chip' }, m))),
          String(g.shares),
          h('span', { class: 'row' },
            action('Membres…', () => this.members(g, users), 'text'),
            action('Supprimer', () => this.remove(g), 'text')),
        ]), 'Aucun groupe.'),
      section('Créer un groupe', this.addForm()),
    ];
  }

  private addForm(): HTMLElement {
    const name = h('input', { required: true, pattern: '[a-z][a-z0-9-]{0,31}', placeholder: 'famille', autocomplete: 'off' });
    const display = h('input', { placeholder: 'Famille' });
    const form = h('form', {}, h('div', { class: 'form-grid' }, field('Identifiant', name), field('Nom affiché', display)),
      h('button', { class: 'btn filled', type: 'submit' }, 'Créer'));
    form.addEventListener('submit', (ev) => {
      ev.preventDefault();
      void this.act(() => this.api.post('/groups', { name: name.value.trim(), display_name: display.value.trim() }), 'Groupe créé.');
    });
    return form;
  }

  private async members(g: Group, users: UserLite[]): Promise<void> {
    const checks = users.map((u) => h('input', { type: 'checkbox', value: u.name, checked: g.members.includes(u.name) }));
    const v = await dialog(this.doc, `Membres de ${g.display_name}`,
      [h('div', { class: 'checks' }, ...users.map((u, i) => h('label', {}, checks[i]!, u.display_name)))],
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

interface CalendarMeta {
  ref: string;
  owner: string;
  name: string;
  color: string;
  timezone: string;
  events: number;
  bytes: number;
  shares: { target: string; access: string }[];
}

/** Every calendar, without its events (PLAN.md 10, privacy from the admin). */
export class CalendarsPage extends AdminPage {
  protected async render(): Promise<Child[]> {
    const [{ calendars }, { users }] = await Promise.all([
      this.api.get<{ calendars: CalendarMeta[] }>('/calendars'),
      this.api.get<{ users: UserLite[] }>('/users'),
    ]);
    return [
      h('p', { class: 'muted' }, "L'administration voit les calendriers, jamais leurs événements."),
      table(['Calendrier', 'Propriétaire', 'Événements', 'Taille', 'Partages', ''],
        calendars.map((c) => [
          h('span', {}, swatch(c.color), c.name, ' ', h('span', { class: 'muted mono' }, c.ref)),
          h('a', { href: `/admin/users/${seg(c.owner)}` }, c.owner),
          String(c.events),
          bytes(c.bytes),
          h('span', {}, ...c.shares.map((s) => h('span', { class: 'chip' }, `${s.target} · ${accessLabel(s.access)} `,
            action('×', () => this.act(() => this.api.delete(`/calendars/${seg(c.ref)}/shares/${seg(s.target)}`), 'Partage retiré.'), 'text')))),
          h('span', { class: 'row' },
            action('Donner…', () => this.transfer(c, users), 'text'),
            action('Supprimer…', () => this.remove(c), 'text')),
        ]), 'Aucun calendrier. Les utilisateurs les créent depuis leur téléphone ou l’application.'),
    ];
  }

  private async transfer(c: CalendarMeta, users: UserLite[]): Promise<void> {
    const select = h('select', {}, ...users.filter((u) => u.name !== c.owner).map((u) => h('option', { value: u.name }, u.display_name)));
    const keep = h('input', { type: 'checkbox', checked: true });
    const v = await dialog(this.doc, `Donner ${c.name}`, [field('À', select), h('label', { class: 'checks' }, keep, `${c.owner} garde lecture et écriture`)],
      [{ label: 'Annuler', value: 'no' }, { label: 'Donner', value: 'yes', kind: 'filled' }]);
    if (v !== 'yes' || select.value === '') return;
    await this.act(() => this.api.post(`/calendars/${seg(c.ref)}/transfer`, { to: select.value, keep: keep.checked }), 'Calendrier donné.');
  }

  private async remove(c: CalendarMeta): Promise<void> {
    const typed = h('input', { autocomplete: 'off' });
    const v = await dialog(this.doc, `Supprimer ${c.name} ?`, [
      h('p', {}, `Ses ${c.events} événement(s) disparaissent pour tout le monde. Seule une sauvegarde peut les rendre.`),
      field(`Tapez « ${c.name} » pour confirmer`, typed),
    ], [{ label: 'Annuler', value: 'no' }, { label: 'Supprimer', value: 'yes', kind: 'danger' }]);
    if (v !== 'yes') return;
    await this.act(() => this.api.delete(`/calendars/${seg(c.ref)}`, { confirm: typed.value }), 'Calendrier supprimé.');
  }
}

interface Activity {
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
    const form = h('form', { class: 'form-grid' }, field('Utilisateur', user), field('Calendrier', calendar), field('Quoi', kind),
      h('div', {}, h('button', { class: 'btn tonal', type: 'submit' }, 'Filtrer')));
    form.addEventListener('submit', (ev) => {
      ev.preventDefault();
      this.filters = { user: user.value.trim(), calendar: calendar.value.trim(), kind: kind.value };
      void this.refresh();
    });
    return form;
  }

  private tableOf(): HTMLElement {
    return table(['Quand', 'Qui', 'Quoi', 'Où', 'Par'],
      this.items.map((a) => [when(a.at), a.by, `${kinds[a.kind] ?? a.kind} · ${actions[a.action] ?? a.action}${a.detail ? ` · ${a.detail}` : ''}`,
        a.calendar ?? '', [a.via, a.device].filter((x) => x !== undefined && x !== '').join(' · ')]),
      "Rien dans l'historique.");
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
    const form = h('form', { class: 'settings' },
      ...['network', 'storage', 'limits', 'defaults'].map((g) => section(groupNames[g] ?? g, ...(groups.get(g) ?? []))),
      h('button', { class: 'btn filled', type: 'submit' }, 'Appliquer'));
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
    return [h('p', { class: 'muted' }, "Appliqués tout de suite, sans redémarrer. L'adresse de l'administration reste celle du fichier de démarrage."), form];
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
      h('div', { class: 'row' }, action('Sauvegarder maintenant', () => this.act(() => this.api.post('/backups'), 'Sauvegarde faite.'), 'filled')),
      table(['Sauvegarde', 'Faite', 'Taille', ''],
        backups.map((b) => [
          h('span', { class: 'mono' }, b.name, b.before_restore ? h('span', { class: 'chip' }, 'avant restauration') : null),
          when(b.made),
          bytes(b.size),
          h('span', { class: 'row' },
            h('a', { class: 'btn text', href: this.api.url(`/backups/${seg(b.name)}`), download: b.name }, 'Télécharger'),
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
      setTimeout(() => this.doc.location.assign('/admin/signin'), 5000);
    } catch (e) {
      this.toasts.error(e instanceof ApiError ? e : new Error(String(e)));
    }
  }
}
