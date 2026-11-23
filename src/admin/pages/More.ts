import { avatar, calendarMark, entries, groupMark } from '@/admin/entries.ts';
import { ui } from '@/shared/components/recipes.ts';
import { icon } from '@/shared/components/icons.ts';
import { ActivityLog } from '@/admin/ActivityLog.ts';
import { ApiError, seg } from '@/shared/core/Api.ts';
import { h, type Child } from '@/shared/core/Dom.ts';
import { action, card, confirmDialog, field, formDialog, section, swatch, table } from '@/shared/components/Ui.ts';
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
  color: string;
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
        entries(groups.map((g) => ({
          href: `/groups/${seg(g.name)}`,
          lead: groupMark(g.color),
          title: g.display_name || g.name,
          sub: `${g.name} · ${g.members.length === 0 ? 'aucun membre' : g.members.map((m) => users.find((u) => u.name === m)?.display_name || m).join(', ')}`,
          stats: [{ n: String(g.members.length), label: 'membres' }, { n: String(g.shares), label: 'partages' }],
        })), 'Aucun groupe.')),
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
    return [
      h('section', { class: `${ui.card} flex-row items-center gap-4` },
        swatch(g.color, 'md'),
        h('div', { class: 'flex min-w-0 flex-1 flex-col' }, h('h2', { class: 'truncate text-2xl font-normal' }, g.display_name || g.name),
          h('span', { class: 'text-sm text-on-surface-variant' }, `Identifiant ${g.name} · ses membres se voient et peuvent partager leurs calendriers entre eux.`))),
      h('div', { class: 'grid gap-3 [grid-template-columns:repeat(auto-fill,minmax(150px,1fr))]' },
        tileLink(String(g.members.length), 'membres', `${this.path()}/members`),
        tileLink(String(g.shares), 'partages de calendrier', `${this.path()}/calendars`)),
      this.members(g, users),
      section('Profil', form),
      section('Supprimer le groupe',
        h('p', { class: 'text-sm text-on-surface-variant' }, g.shares > 0 ? `${g.shares} partage(s) de calendrier passent par ce groupe ; ils disparaîtront.` : 'Ses membres ne se verront plus par ce groupe.'),
        action('Supprimer…', () => this.remove(g), 'danger', 'trash')),
    ];
  }

  private members(g: Group, users: UserLite[]): HTMLElement {
    const set = (members: string[], done: string): Promise<void> => this.act(() => this.api.patch(this.path(), { members }), done);
    const byName = new Map(users.map((u) => [u.name, u]));
    return card('Membres', [action('Ajouter un membre', () => this.addMember(g, users), 'filled', 'plus')],
      h('p', { class: 'text-sm text-on-surface-variant' }, 'Ceux qui sont dans le groupe se voient, et peuvent partager leurs calendriers avec lui.'),
      entries(g.members.map((m) => {
        const u = byName.get(m);
        return {
          href: `/users/${seg(m)}`,
          lead: avatar(u?.display_name || m, u?.color ?? ''),
          title: u?.display_name || m,
          sub: m,
          actions: [action('Retirer', () => set(g.members.filter((x) => x !== m), 'Retiré du groupe.'), 'danger-text', 'close', 'sm')],
        };
      }), 'Personne dans ce groupe.'));
  }

  private async addMember(g: Group, users: UserLite[]): Promise<void> {
    const others = users.filter((u) => !g.members.includes(u.name));
    if (others.length === 0) {
      this.toasts.show('Tout le monde en est déjà membre.');
      return;
    }
    const who = h('select', { required: true }, ...others.map((u) => h('option', { value: u.name }, u.display_name || u.name)));
    const ok = await formDialog(this.doc, `Ajouter à ${g.display_name || g.name}`, [field('Qui', who)], 'Ajouter',
      () => this.api.patch(this.path(), { members: [...g.members, who.value] }));
    if (!ok) return;
    this.toasts.show('Ajouté au groupe.');
    this.doc.dispatchEvent(new Event('agd:changed'));
    await this.refresh();
  }


  private async calendars(g: Group): Promise<HTMLElement> {
    const { calendars } = await this.api.get<{ calendars: CalendarShared[] }>('/calendars');
    const target = `g:${g.name}`;
    const shared = calendars.filter((c) => c.shares.some((s) => s.target === target));
    return section('Calendriers partagés avec le groupe',
      h('p', { class: 'text-sm text-on-surface-variant' }, "L'administration voit les calendriers, jamais leurs événements."),
      entries(shared.map((c) => {
        const s = c.shares.find((x) => x.target === target)!;
        return {
          href: `/calendars/${seg(c.ref)}`,
          lead: calendarMark(c.color),
          title: c.name,
          sub: `à ${c.owner} · ${c.events} événement${c.events === 1 ? '' : 's'}`,
          tags: [h('span', { class: s.access === 'read' ? ui.pill.muted : ui.pill.ok }, accessLabel(s.access))],
          actions: [action('Retirer', () => this.act(() => this.api.delete(`/calendars/${seg(c.ref)}/shares/${seg(target)}`), 'Partage retiré.'), 'danger-text', 'close', 'sm')],
        };
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

export const accessLabel = (a: string): string => ({ read: 'lecture', readwrite: 'lecture et écriture', owner: 'propriétaire' })[a] ?? a;

/** Who did what, never what an event said: claudication's request log, for the activity (ActivityLog). */
export class ActivityPage extends AdminPage {
  private log: ActivityLog | null = null;

  protected async render(): Promise<Child[]> {
    this.log ??= new ActivityLog(this.doc, this.api);
    await this.log.load();
    return [card('Activité', [],
      h('p', { class: 'text-sm text-on-surface-variant' }, "Qui a fait quoi, et par quel appareil ; jamais le contenu d'un événement."),
      this.log.root)];
  }

  override dispose(): void {
    this.log?.dispose();
    super.dispose();
  }
}

interface Setting {
  key: string;
  group: string;
  type: 'string' | 'bool' | 'int' | 'list';
  default: unknown;
  value: unknown;
  origin: 'file' | 'env' | 'flag' | 'default';
  env: string;
}

const groupNames: Record<string, string> = { network: 'Réseau', storage: 'Stockage et sauvegardes', limits: 'Limites', defaults: 'Valeurs par défaut' };

const hints: Record<string, string> = {
  public_listen: 'Adresse où écoutent CalDAV et l’application (celle vers laquelle le proxy de la DMZ renvoie), par exemple 0.0.0.0:8791.',
  public_url: 'Adresse publique, en https, telle que le proxy la sert : elle est dans chaque QR et chaque téléphone.',
  public_allow_insecure: 'Accepter http:// pour public_url (essais sur le réseau local seulement).',
  trusted_proxies: 'Adresses ou CIDR du proxy : leur X-Forwarded-For est cru.',
  backup_schedule: 'Heure de la sauvegarde quotidienne (HH:MM, heure du serveur).',
};

const ORIGINS: Record<string, { label: string; cls: string }> = {
  file: { label: 'fichier', cls: ui.pill.ok },
  env: { label: "variable d'environnement", cls: ui.pill.warn },
  flag: { label: 'option de lancement', cls: ui.pill.warn },
  default: { label: 'par défaut', cls: ui.pill.muted },
};

const shown = (v: unknown): string => (Array.isArray(v) ? (v.length === 0 ? '—' : v.join(', ')) : v === '' || v === null || v === undefined ? '—' : String(v));

/**
 * The settings, as cafard and claudication show theirs: read from the
 * configuration file (and the environment), each with where its value
 * came from. Edited in the file; the server applies an edit within
 * seconds (DECISIONS: settings in the configuration file).
 */
export class SettingsPage extends AdminPage {
  /**
   * How CalDAV signs a device in: the one choice made here rather than in
   * the configuration file (the user's call; DECISIONS: device keys).
   */
  private async davAuthCard(): Promise<HTMLElement> {
    const st = await this.api.get<{ mode: string; devices_without_certificate: number }>('/dav-auth');
    const choices: [string, string, string][] = [
      ['password', 'Mot de passe', 'Chaque appareil a son mot de passe, comme DAVx5, l’iPhone et Thunderbird le savent.'],
      ['both', 'Mot de passe et certificat', 'Il faut les deux : un mot de passe volé ne suffit plus. L’iPhone ne sait pas toujours présenter un certificat.'],
      ['keys', 'Certificat seul', 'Une clé par appareil, sans mot de passe. Pour DAVx5 et Thunderbird ; pas pour l’iPhone.'],
    ];
    const options = choices.map(([value, label, help]) => {
      const on = st.mode === value;
      const b = h('button', { type: 'button', role: 'radio', 'aria-checked': String(on),
        class: on ? 'state-layer flex flex-1 flex-col gap-1 rounded-md3-l bg-secondary-container p-3 text-left text-on-secondary-container ring-2 ring-primary'
          : 'state-layer flex flex-1 flex-col gap-1 rounded-md3-l bg-surface-lowest p-3 text-left text-on-surface' },
      h('b', { class: 'flex items-center gap-2' }, on ? icon('check') : null, label), h('span', { class: 'text-xs opacity-80' }, help));
      b.addEventListener('click', () => {
        if (on) return;
        void this.act(() => this.api.put('/dav-auth', { mode: value }), `Connexion des appareils : ${label.toLowerCase()}.`);
      });
      return b;
    });
    return card('Connexion des appareils (CalDAV)', [],
      h('p', { class: 'text-sm text-on-surface-variant' }, "Choisi ici, pas dans le fichier : il s'applique tout de suite à toutes les synchronisations."),
      h('div', { class: 'flex flex-col gap-2 md:flex-row', role: 'radiogroup', 'aria-label': 'Connexion des appareils' }, ...options),
      st.mode !== 'password' && st.devices_without_certificate > 0
        ? h('p', { class: ui.notice }, icon('warn'), `${st.devices_without_certificate} appareil(s) sans certificat ne synchronisent plus : chacun en obtient un depuis Appareils, dans l’application.`)
        : null,
      st.mode !== 'password'
        ? h('p', { class: 'text-xs text-on-surface-variant' }, 'Le proxy de la DMZ doit demander le certificat et le transmettre dans X-Client-Cert, sur /dav/ (deploy/README.md) ; il est vérifié ici, et seulement venant de trusted_proxies.')
        : null);
  }

  protected async render(): Promise<Child[]> {
    const { settings, path, exists } = await this.api.get<{ settings: Setting[]; path: string; exists: boolean }>('/settings');
    const groups = new Map<string, Setting[]>();
    for (const st of settings) groups.set(st.group, [...(groups.get(st.group) ?? []), st]);
    const how = card('Fichier de configuration', [h('code', { class: 'rounded-md3-s bg-surface-lowest px-2 py-1 font-mono text-[13px]' }, path)],
      h('p', { class: 'text-sm text-on-surface-variant' },
        exists ? "Les réglages se changent dans ce fichier, sur le serveur. Le serveur l'applique tout seul en quelques secondes ; une erreur dans le fichier est notée dans son journal et les réglages en place restent."
          : "Aucun fichier à cet endroit : tout vient des valeurs par défaut et de l'environnement. Créez-le pour changer un réglage."),
      h('pre', { class: 'overflow-x-auto rounded-md3-m bg-surface-lowest px-4 py-3 font-mono text-[12.5px] leading-relaxed text-on-surface' },
        `sudo -e ${path}\n# par exemple :\npublic_url: https://agenda.example.org\nbackup_keep: 30\n\n# puis, pour vérifier :\nagendouille config`),
      h('p', { class: 'text-xs text-on-surface-variant' }, "Une variable d'environnement (AGENDOUILLE_…) l'emporte sur le fichier. Les clés data_dir, admin_listen, admin_public_url et log_level demandent un redémarrage."));
    const cards = ['network', 'storage', 'limits', 'defaults'].map((g) => section(groupNames[g] ?? g,
      h('dl', { class: 'm-0 flex flex-col divide-y divide-outline-variant/50 rounded-md3-m bg-surface-lowest' }, ...(groups.get(g) ?? []).map((st) => {
        const o = ORIGINS[st.origin] ?? ORIGINS['default']!;
        const changed = JSON.stringify(st.value) !== JSON.stringify(st.default);
        return h('div', { class: 'flex flex-col gap-1 px-4 py-3' },
          h('dt', { class: 'flex flex-wrap items-center gap-2' },
            h('code', { class: 'font-mono text-[13px] font-semibold' }, st.key),
            h('span', { class: o.cls, title: st.origin === 'env' ? st.env : undefined }, o.label)),
          h('dd', { class: 'm-0 flex flex-col gap-0.5' },
            h('span', { class: 'font-mono text-[13px] break-all text-on-surface' }, shown(st.value)),
            hints[st.key] !== undefined ? h('span', { class: 'text-xs text-on-surface-variant' }, hints[st.key]) : null,
            changed ? h('span', { class: 'text-xs text-on-surface-variant' }, `Par défaut : ${shown(st.default)} · ${st.env}`)
              : h('span', { class: 'text-xs text-on-surface-variant' }, st.env)));
      }))));
    return [await this.davAuthCard(), how, h('div', { class: ui.cards }, ...cards)];
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
    const { calendars } = await this.api.get<{ calendars: { ref: string; name: string; owner: string; color: string; events: number }[] }>('/calendars');
    const icsCard = card('Calendriers en .ics', [
      h('a', { class: ui.btn('tonal'), href: this.api.url('/calendars/export.zip'), download: '' }, icon('download'), 'Tous, en .zip'),
    ],
    h('p', { class: 'text-sm text-on-surface-variant' }, "Chaque calendrier dans un fichier que n'importe quelle application d'agenda relit, ou que son propriétaire réimporte. Ces fichiers contiennent les événements ; chaque téléchargement est noté dans l'activité."),
    calendars.length === 0 ? h('p', { class: ui.empty }, 'Aucun calendrier.')
      : h('div', { class: ui.well }, ...calendars.map((c) => h('div', { class: 'flex min-h-14 items-center gap-3 border-b border-outline-variant/50 px-4 py-2.5 last:border-0' },
        swatch(c.color, 'md'),
        h('span', { class: 'flex min-w-0 flex-1 flex-col' }, h('a', { href: `/calendars/${seg(c.ref)}`, class: 'truncate font-semibold text-on-surface no-underline hover:underline' }, c.name),
          h('span', { class: 'text-xs text-on-surface-variant' }, `à ${c.owner} · ${c.events} événement${c.events === 1 ? '' : 's'}`)),
        h('a', { class: ui.btn('text', 'sm'), href: this.api.url(`/calendars/${seg(c.ref)}/export`), download: `${c.ref}.ics` }, icon('download'), '.ics')))));
    return [
      card('Base de données', [action('Sauvegarder maintenant', () => this.act(() => this.api.post('/backups'), 'Sauvegarde faite.'), 'filled', 'disk')],
      h('p', { class: 'text-sm text-on-surface-variant' }, "Tout agendouille en un fichier : utilisateurs, groupes, calendriers, appareils, réglages. Faite chaque jour à l'heure des réglages ; restaurer redémarre le serveur."),
      table(['Sauvegarde', 'Faite', 'Taille', ''],
        backups.map((b) => [
          h('span', { class: 'font-mono text-[13px]' }, b.name, b.before_restore ? h('span', { class: 'mr-1 inline-flex h-6 items-center gap-1 rounded-md3-s bg-surface-high px-2 text-xs font-semibold text-on-surface' }, 'avant restauration') : null),
          when(b.made),
          bytes(b.size),
          h('span', { class: 'flex flex-wrap items-center gap-2' },
            h('a', { class: ui.btn('text'), href: this.api.url(`/backups/${seg(b.name)}`), download: b.name }, 'Télécharger'),
            action('Restaurer…', () => this.restore(b), 'text')),
        ]), 'Aucune sauvegarde pour le moment.')),
      icsCard,
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
