import { icon } from '@/shared/components/icons.ts';
import { ui } from '@/shared/components/recipes.ts';
import { seg } from '@/shared/core/Api.ts';
import { h, svgDataUrl, type Child } from '@/shared/core/Dom.ts';
import { action, card, confirmDialog, dialog, field, formDialog, section, swatch, table } from '@/shared/components/Ui.ts';
import { ago, bytes, day, when } from '@/shared/render/format.ts';
import { AdminPage } from '@/admin/pages/base.ts';
import { accessLabel, activityTable, tileLink, type Activity } from '@/admin/pages/More.ts';

export interface User {
  name: string;
  display_name: string;
  color: string;
  disabled: boolean;
  created_at: string;
  last_seen_at?: string;
}

export interface UserRow extends User {
  groups: string[];
  devices: number;
  calendars_owned: number;
}

interface Group {
  name: string;
  display_name: string;
}

export interface Enrolment {
  id: number;
  url?: string;
  qr_svg?: string;
  made_by?: string;
  created_at: string;
  expires_at: string;
  used_at?: string;
}

interface Device {
  id: number;
  kind: string;
  label: string;
  cap: string;
  client: string;
  created_at: string;
  last_used_at?: string;
  last_ip?: string;
}

interface Session {
  id: number;
  user_agent: string;
  app: boolean;
  seen_at: string;
}

interface CalendarMeta {
  ref: string;
  name: string;
  color: string;
  events: number;
  bytes: number;
  shares: { target: string; access: string }[];
}

interface UserDetail {
  user: User;
  groups: string[];
  enrolments: Enrolment[];
  devices: Device[];
  sessions: Session[];
  calendars_owned: CalendarMeta[];
  calendars_shared: { ref: string; name: string; access: string; via: string }[];
}

const userLink = (name: string): HTMLElement => h('a', { href: `/users/${seg(name)}` }, name);

/** Shows a fresh enrolment's QR code and link, once (PLAN.md 7.2). */
export async function showEnrolment(doc: Document, who: string, e: Enrolment): Promise<void> {
  const body: Child[] = [
    h('p', {}, `${who} scanne ce code avec l'appareil photo de son téléphone. Il ne marche qu'une fois, jusqu'au ${when(e.expires_at)}.`),
    e.qr_svg !== undefined ? h('img', { class: 'qr mx-auto rounded-md3-l p-2', src: svgDataUrl(e.qr_svg), alt: 'QR code de connexion', width: 240, height: 240 }) : null,
    h('p', { class: 'text-xs text-on-surface-variant' }, 'Ou envoyez-lui ce lien par message :'),
    h('p', { class: ui.secret }, e.url ?? ''),
  ];
  await dialog(doc, 'Première connexion', body, [{ label: 'Fermer', value: 'close', kind: 'filled' }]);
}

/** The users, and a form to add one. */
export class UsersPage extends AdminPage {
  protected async render(): Promise<Child[]> {
    const [{ users }, { groups }] = await Promise.all([
      this.api.get<{ users: UserRow[] }>('/users'),
      this.api.get<{ groups: Group[] }>('/groups'),
    ]);
    return [
      card('Utilisateurs', [action('Nouvel utilisateur', () => this.create(groups), 'filled', 'plus')],
      table(
        ['Nom', 'Affiché', 'Groupes', 'Appareils', 'Calendriers', 'Vu', ''],
        users.map((u) => [
          userLink(u.name),
          h('span', {}, swatch(u.color), u.display_name),
          h('span', {}, ...u.groups.map((g) => h('span', { class: 'mr-1 inline-flex h-6 items-center gap-1 rounded-md3-s bg-surface-high px-2 text-xs font-semibold text-on-surface' }, g))),
          String(u.devices),
          String(u.calendars_owned),
          ago(u.last_seen_at),
          u.disabled ? h('span', { class: ui.pill.err }, 'désactivé') : '',
        ]),
        'Aucun utilisateur pour le moment.',
      )),
    ];
  }

  /** "Nouvel utilisateur": the form in a modal, then the sign-in QR. */
  private async create(groups: Group[]): Promise<void> {
    const name = h('input', { name: 'name', required: true, autocomplete: 'off', pattern: '[a-z][a-z0-9-]{0,31}', placeholder: 'lea' });
    const display = h('input', { name: 'display_name', placeholder: 'Léa' });
    const checks = groups.map((g) => h('input', { type: 'checkbox', value: g.name }));
    let made: { user: User; enrolment?: Enrolment } | null = null;
    const ok = await formDialog(this.doc, 'Nouvel utilisateur', [
      field('Identifiant', name, 'Minuscules, chiffres et tirets ; ne change plus.'),
      field('Nom affiché', display),
      groups.length > 0
        ? h('div', { class: ui.field }, h('span', { class: ui.label }, 'Groupes'),
          h('div', { class: 'add-user-groups flex flex-wrap gap-x-5 gap-y-2' }, ...groups.map((g, i) => h('label', { class: ui.check }, checks[i]!, g.display_name))))
        : h('p', { class: 'text-sm' }, "Aucun groupe : créez-en un pour que les gens se voient et partagent."),
    ], 'Ajouter', async () => {
      made = await this.api.post<{ user: User; enrolment?: Enrolment }>('/users', {
        name: name.value.trim(),
        display_name: display.value.trim(),
        groups: checks.filter((c) => c.checked).map((c) => c.value),
        enrol: true,
      });
    });
    if (!ok || made === null) return;
    const res: { user: User; enrolment?: Enrolment } = made;
    this.doc.dispatchEvent(new Event('agd:changed'));
    await this.refresh();
    if (res.enrolment !== undefined) await showEnrolment(this.doc, res.user.display_name, res.enrolment);
    else this.toasts.show("Ajouté. Configurez public_url dans les réglages pour créer son QR de connexion.");
  }
}

/** One user: profile, groups, sign-in, devices, browsers, calendars. */
export class UserPage extends AdminPage {
  private path(): string {
    return `/users/${seg(this.name)}`;
  }

  private get sub(): string {
    return this.root.dataset['sub'] ?? '';
  }

  protected async render(): Promise<Child[]> {
    const [d, { groups }, { users }] = await Promise.all([
      this.api.get<UserDetail>(this.path()),
      this.api.get<{ groups: Group[] }>('/groups'),
      this.api.get<{ users: UserRow[] }>('/users'),
    ]);
    const u = d.user;
    for (const el of this.doc.querySelectorAll<HTMLElement>('[data-item-name] span')) el.textContent = u.display_name || u.name;
    switch (this.sub) {
      case 'signin':
        return [
          this.signin(u, d.enrolments),
          section('Navigateurs connectés',
            table(['Navigateur', 'Vu', ''],
              d.sessions.map((x) => [x.app ? `Application · ${x.user_agent}` : x.user_agent, ago(x.seen_at),
                action('Déconnecter', () => this.act(() => this.api.delete(`${this.path()}/sessions/${x.id}`), 'Déconnecté.'), 'text', 'logout', 'sm')]),
              'Aucun navigateur.')),
        ];
      case 'devices':
        return [section('Appareils de synchronisation',
          table(['Appareil', 'Type', 'Accès', 'Dernière synchro', 'Client', ''],
            d.devices.map((x) => [x.label, x.kind, x.cap === 'read' ? 'lecture' : 'lecture et écriture', ago(x.last_used_at), x.client,
              action('Révoquer', () => this.act(() => this.api.delete(`${this.path()}/devices/${x.id}`), 'Appareil révoqué.'), 'danger-text', 'trash', 'sm')]),
            'Aucun appareil. La personne en ajoute depuis Appareils, dans l’application.'))];
      case 'calendars':
        return [this.calendarsCard(d, users)];
      case 'groups':
        return [this.groupsCard(d.groups, groups)];
      case 'activity': {
        const { activity } = await this.api.get<{ activity: Activity[] }>('/activity', { user: u.name });
        return [section('Activité', activityTable(activity))];
      }
      default:
        return this.overview(u, d, users);
    }
  }

  /** The user's overview: who, at a glance, and the account. */
  private overview(u: User, d: UserDetail, users: UserRow[]): Child[] {
    const base = this.path();
    const avatar = h('span', { class: 'grid size-14 shrink-0 place-items-center rounded-full text-2xl font-bold text-white' }, (u.display_name || u.name).slice(0, 1).toUpperCase());
    if (/^#[0-9a-f]{6}$/i.test(u.color)) avatar.style.setProperty('background-color', u.color);
    else avatar.classList.add('bg-primary');
    return [
      h('section', { class: `${ui.card} flex-row flex-wrap items-center gap-4` }, avatar,
        h('div', { class: 'flex min-w-0 flex-1 flex-col gap-1' },
          h('h2', { class: 'truncate text-2xl font-normal' }, u.display_name || u.name),
          h('span', { class: 'text-sm text-on-surface-variant' }, `Identifiant ${u.name} · créé le ${day(u.created_at)} · vu ${ago(u.last_seen_at)}`),
          d.groups.length > 0 ? h('div', { class: 'flex flex-wrap gap-1.5 pt-1' }, ...d.groups.map((g) => h('a', { href: `/groups/${seg(g)}`, class: 'inline-flex h-6 items-center rounded-md3-s bg-surface-high px-2 text-xs font-semibold text-on-surface no-underline' }, g))) : null),
        u.disabled ? h('span', { class: ui.pill.err }, 'désactivé') : null),
      u.disabled ? h('p', { class: ui.error }, 'Désactivé : ses appareils et ses navigateurs ne marchent plus.') : null,
      h('div', { class: 'grid gap-3 [grid-template-columns:repeat(auto-fill,minmax(150px,1fr))]' },
        tileLink(String(d.devices.length), 'appareils', `${base}/devices`),
        tileLink(String(d.calendars_owned.length), 'calendriers à lui ou elle', `${base}/calendars`),
        tileLink(String(d.calendars_shared.length), 'calendriers partagés avec lui ou elle', `${base}/calendars`),
        tileLink(String(d.sessions.length), 'navigateurs connectés', `${base}/signin`)),
      this.profile(u),
      section('Compte',
        h('div', { class: 'flex flex-wrap items-center gap-2' },
          u.disabled
            ? action('Réactiver', () => this.act(() => this.api.patch(this.path(), { disabled: false }), 'Réactivé.'))
            : action('Désactiver', () => this.act(() => this.api.patch(this.path(), { disabled: true }), 'Désactivé.')),
          action('Supprimer…', () => this.remove(u, d.calendars_owned, users), 'danger'),
        )),
    ];
  }

  private calendarsCard(d: UserDetail, users: UserRow[]): HTMLElement {
    return section('Calendriers',
        h('p', { class: 'text-sm text-on-surface-variant' }, "L'administration voit les calendriers, jamais leurs événements."),
        h('h3', { class: 'text-sm font-semibold' }, 'Les siens'),
        table(['Calendrier', 'Événements', 'Taille', 'Partagé avec', ''],
          d.calendars_owned.map((c) => [
            h('span', { class: 'inline-flex items-center' }, swatch(c.color), c.name),
            String(c.events),
            bytes(c.bytes),
            h('span', { class: 'flex flex-wrap gap-1' }, ...c.shares.map((s) => h('span', { class: 'inline-flex h-7 items-center gap-1 rounded-md3-s bg-surface-high pl-2 text-xs font-semibold text-on-surface' },
              `${s.target} · ${accessLabel(s.access)}`,
              iconButtonSm('close', `Retirer le partage avec ${s.target}`, () => this.act(() => this.api.delete(`/calendars/${seg(c.ref)}/shares/${seg(s.target)}`), 'Partage retiré.'))))),
            h('span', { class: 'flex flex-wrap items-center justify-end gap-1' },
              action('Donner', () => this.transferCalendar(c, users), 'text', 'give', 'sm'),
              action('Supprimer', () => this.removeCalendar(c), 'danger-text', 'trash', 'sm')),
          ]), 'Aucun.'),
        h('h3', { class: 'text-sm font-semibold' }, 'Partagés avec lui ou elle'),
        table(['Calendrier', 'Accès', 'Par'], d.calendars_shared.map((c) => [c.name, accessLabel(c.access), c.via]), 'Aucun.'),
      );
  }

  private async transferCalendar(c: CalendarMeta, users: UserRow[]): Promise<void> {
    const select = h('select', {}, ...users.filter((u) => u.name !== this.name).map((u) => h('option', { value: u.name }, u.display_name)));
    const keep = h('input', { type: 'checkbox', checked: true });
    const v = await dialog(this.doc, `Donner ${c.name}`, [field('À', select), h('label', { class: ui.check }, keep, `${this.name} garde lecture et écriture`)],
      [{ label: 'Annuler', value: 'no' }, { label: 'Donner', value: 'yes', kind: 'filled' }]);
    if (v !== 'yes' || select.value === '') return;
    await this.act(() => this.api.post(`/calendars/${seg(c.ref)}/transfer`, { to: select.value, keep: keep.checked }), 'Calendrier donné.');
  }

  private async removeCalendar(c: CalendarMeta): Promise<void> {
    const typed = h('input', { autocomplete: 'off' });
    const v = await dialog(this.doc, `Supprimer ${c.name} ?`, [
      h('p', {}, `Ses ${c.events} événement(s) disparaissent pour tout le monde. Seule une sauvegarde peut les rendre.`),
      field(`Tapez « ${c.name} » pour confirmer`, typed),
    ], [{ label: 'Annuler', value: 'no' }, { label: 'Supprimer', value: 'yes', kind: 'danger' }]);
    if (v !== 'yes') return;
    await this.act(() => this.api.delete(`/calendars/${seg(c.ref)}`, { confirm: typed.value }), 'Calendrier supprimé.');
  }

  private profile(u: User): HTMLElement {
    const display = h('input', { value: u.display_name });
    const color = h('input', { type: 'color', value: u.color });
    const form = h('form', { class: 'flex flex-col items-start gap-4 [&>div]:w-full' }, h('div', { class: 'grid gap-4 [grid-template-columns:repeat(auto-fill,minmax(min(220px,100%),1fr))]' }, field('Nom affiché', display), field('Couleur', color)),
      h('button', { class: ui.btn('tonal'), type: 'submit' }, 'Enregistrer'));
    form.addEventListener('submit', (ev) => {
      ev.preventDefault();
      void this.act(() => this.api.patch(this.path(), { display_name: display.value.trim(), color: color.value }), 'Enregistré.');
    });
    return section('Profil', form);
  }

  private groupsCard(mine: string[], all: Group[]): HTMLElement {
    const checks = all.map((g) => h('input', { type: 'checkbox', value: g.name, checked: mine.includes(g.name) }));
    const form = h('form', { class: 'flex flex-col items-start gap-4 [&>div]:w-full' },
      h('div', { class: 'flex flex-wrap gap-x-5 gap-y-2' }, ...all.map((g, i) => h('label', { class: ui.check }, checks[i]!, g.display_name))),
      h('p', { class: 'text-xs text-on-surface-variant' }, "On voit les gens de ses groupes, et on ne partage qu'avec eux."),
      h('button', { class: ui.btn('tonal'), type: 'submit' }, 'Enregistrer les groupes'));
    form.addEventListener('submit', (ev) => {
      ev.preventDefault();
      void this.act(() => this.api.patch(this.path(), { groups: checks.filter((c) => c.checked).map((c) => c.value) }), 'Groupes enregistrés.');
    });
    return section('Groupes', all.length > 0 ? form : h('p', { class: 'text-sm text-on-surface-variant' }, 'Aucun groupe.'));
  }

  private signin(u: User, enrolments: Enrolment[]): HTMLElement {
    return section('Connexion',
      h('p', {}, 'Un QR de connexion connecte son premier appareil, ou un nouveau téléphone après une perte.'),
      action('Créer un QR de connexion', async () => {
        try {
          const e = await this.api.post<Enrolment>(`${this.path()}/enrolments`);
          await showEnrolment(this.doc, u.display_name, e);
          await this.refresh();
        } catch (err) {
          this.toasts.error(err);
        }
      }, 'filled'),
      table(['Créé', 'Expire', 'État', ''],
        enrolments.map((e) => [when(e.created_at), when(e.expires_at),
          e.used_at !== undefined ? `utilisé ${ago(e.used_at)}` : new Date(e.expires_at).getTime() < Date.now() ? 'expiré' : 'en attente',
          e.used_at === undefined ? action('Annuler', () => this.act(() => this.api.delete(`${this.path()}/enrolments/${e.id}`), 'Lien annulé.'), 'text') : '']),
        'Aucun lien pour le moment.'),
    );
  }

  /** Removing asks, calendar by calendar, whom to give it to or to delete it (PLAN.md 6). */
  private async remove(u: User, owned: CalendarMeta[], users: UserRow[]): Promise<void> {
    const others = users.filter((x) => x.name !== u.name && !x.disabled);
    const choices = owned.map((c) => {
      const select = h('select', {}, h('option', { value: '' }, 'Supprimer ce calendrier'),
        ...others.map((o) => h('option', { value: o.name }, `Donner à ${o.display_name}`)));
      return { ref: c.ref, select, row: field(c.name, select) };
    });
    const body: Child[] = [
      h('p', {}, `${u.display_name} ne pourra plus se connecter. Ses événements restent dans les calendriers partagés.`),
      ...(owned.length > 0 ? [h('p', {}, 'Ses calendriers :'), ...choices.map((c) => c.row)] : []),
    ];
    const v = await dialog(this.doc, `Supprimer ${u.display_name} ?`, body, [
      { label: 'Annuler', value: 'no' },
      { label: 'Supprimer', value: 'yes', kind: 'danger' },
    ]);
    if (v !== 'yes') return;
    const calendars: Record<string, { transfer_to?: string; delete?: boolean }> = {};
    for (const c of choices) calendars[c.ref] = c.select.value === '' ? { delete: true } : { transfer_to: c.select.value };
    try {
      await this.api.post(`${this.path()}/remove`, { calendars });
      this.doc.location.assign('/users');
    } catch (e) {
      this.toasts.error(e);
    }
  }
}

export { confirmDialog };

/** A small round icon button inside a chip. */
function iconButtonSm(ic: 'close', label: string, run: () => Promise<void>): HTMLButtonElement {
  const b = h('button', { type: 'button', class: 'state-layer grid size-7 place-items-center rounded-full [&_.ic]:size-4', 'aria-label': label, title: label }, icon(ic));
  b.addEventListener('click', () => void run());
  return b;
}
