import { seg } from '@/shared/core/Api.ts';
import { h, svgDataUrl, type Child } from '@/shared/core/Dom.ts';
import { action, confirmDialog, dialog, field, section, swatch, table } from '@/shared/components/Ui.ts';
import { ago, bytes, day, when } from '@/shared/render/format.ts';
import { AdminPage } from '@/admin/pages/base.ts';
import { accessLabel } from '@/admin/pages/More.ts';

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

const userLink = (name: string): HTMLElement => h('a', { href: `/admin/users/${seg(name)}` }, name);

/** Shows a fresh enrolment's QR code and link, once (PLAN.md 7.2). */
export async function showEnrolment(doc: Document, who: string, e: Enrolment): Promise<void> {
  const body: Child[] = [
    h('p', {}, `${who} scanne ce code avec l'appareil photo de son téléphone. Il ne marche qu'une fois, jusqu'au ${when(e.expires_at)}.`),
    e.qr_svg !== undefined ? h('img', { class: 'qr', src: svgDataUrl(e.qr_svg), alt: 'QR code de connexion', width: 240, height: 240 }) : null,
    h('p', { class: 'field-hint' }, 'Ou envoyez-lui ce lien par message :'),
    h('p', { class: 'link-box' }, e.url ?? ''),
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
      table(
        ['Nom', 'Affiché', 'Groupes', 'Appareils', 'Calendriers', 'Vu', ''],
        users.map((u) => [
          userLink(u.name),
          h('span', {}, swatch(u.color), u.display_name),
          h('span', {}, ...u.groups.map((g) => h('span', { class: 'chip' }, g))),
          String(u.devices),
          String(u.calendars_owned),
          ago(u.last_seen_at),
          u.disabled ? h('span', { class: 'chip warn' }, 'désactivé') : '',
        ]),
        'Aucun utilisateur pour le moment.',
      ),
      section('Ajouter un utilisateur', this.addForm(groups)),
    ];
  }

  private addForm(groups: Group[]): HTMLElement {
    const name = h('input', { name: 'name', required: true, autocomplete: 'off', pattern: '[a-z][a-z0-9-]{0,31}', placeholder: 'lea' });
    const display = h('input', { name: 'display_name', placeholder: 'Léa' });
    const checks = groups.map((g) => h('input', { type: 'checkbox', value: g.name }));
    const form = h('form', { class: 'add-user' },
      h('div', { class: 'form-grid' },
        field('Identifiant', name, 'Minuscules, chiffres et tirets ; ne change plus.'),
        field("Nom affiché", display),
      ),
      groups.length > 0
        ? h('div', {}, h('span', { class: 'field-label' }, 'Groupes'), h('div', { class: 'checks' }, ...groups.map((g, i) => h('label', {}, checks[i]!, g.display_name))))
        : h('p', { class: 'muted' }, "Aucun groupe : créez-en un pour que les gens se voient et partagent."),
      h('button', { class: 'btn filled', type: 'submit' }, 'Ajouter et afficher le QR de connexion'),
    );
    form.addEventListener('submit', (ev) => {
      ev.preventDefault();
      void this.act(async () => {
        const res = await this.api.post<{ user: User; enrolment?: Enrolment }>('/users', {
          name: name.value.trim(),
          display_name: display.value.trim(),
          groups: checks.filter((c) => c.checked).map((c) => c.value),
          enrol: true,
        });
        if (res.enrolment !== undefined) await showEnrolment(this.doc, res.user.display_name, res.enrolment);
        else this.toasts.show("Ajouté. Configurez public_url dans les réglages pour créer son QR de connexion.");
      });
    });
    return form;
  }
}

/** One user: profile, groups, sign-in, devices, browsers, calendars. */
export class UserPage extends AdminPage {
  private path(): string {
    return `/users/${seg(this.name)}`;
  }

  protected async render(): Promise<Child[]> {
    const [d, { groups }, { users }] = await Promise.all([
      this.api.get<UserDetail>(this.path()),
      this.api.get<{ groups: Group[] }>('/groups'),
      this.api.get<{ users: UserRow[] }>('/users'),
    ]);
    const u = d.user;
    return [
      h('p', { class: 'muted' }, `Identifiant ${u.name} · créé le ${day(u.created_at)} · vu ${ago(u.last_seen_at)}`),
      u.disabled ? h('p', { class: 'banner error' }, 'Désactivé : ses appareils et ses navigateurs ne marchent plus.') : null,
      this.profile(u),
      this.groupsCard(d.groups, groups),
      this.signin(u, d.enrolments),
      section('Appareils de synchronisation',
        table(['Appareil', 'Type', 'Accès', 'Dernière synchro', 'Client', ''],
          d.devices.map((x) => [x.label, x.kind, x.cap === 'read' ? 'lecture' : 'lecture et écriture', ago(x.last_used_at), x.client,
            action('Révoquer', () => this.act(() => this.api.delete(`${this.path()}/devices/${x.id}`), 'Appareil révoqué.'), 'text')]),
          'Aucun appareil.')),
      section('Navigateurs connectés',
        table(['Navigateur', 'Vu', ''],
          d.sessions.map((x) => [x.app ? `Application · ${x.user_agent}` : x.user_agent, ago(x.seen_at),
            action('Déconnecter', () => this.act(() => this.api.delete(`${this.path()}/sessions/${x.id}`), 'Déconnecté.'), 'text')]),
          'Aucun navigateur.')),
      section('Calendriers',
        h('h3', {}, 'Les siens'),
        table(['Calendrier', 'Événements', 'Taille', 'Partagé avec'],
          d.calendars_owned.map((c) => [h('span', {}, swatch(c.color), c.name), String(c.events), bytes(c.bytes),
            h('span', {}, ...c.shares.map((s) => h('span', { class: 'chip' }, `${s.target} · ${accessLabel(s.access)}`)))]), 'Aucun.'),
        h('h3', {}, 'Partagés avec lui ou elle'),
        table(['Calendrier', 'Accès', 'Par'], d.calendars_shared.map((c) => [c.name, accessLabel(c.access), c.via]), 'Aucun.'),
      ),
      section('Compte',
        h('div', { class: 'row' },
          u.disabled
            ? action('Réactiver', () => this.act(() => this.api.patch(this.path(), { disabled: false }), 'Réactivé.'))
            : action('Désactiver', () => this.act(() => this.api.patch(this.path(), { disabled: true }), 'Désactivé.')),
          action('Supprimer…', () => this.remove(u, d.calendars_owned, users), 'danger'),
        )),
    ];
  }

  private profile(u: User): HTMLElement {
    const display = h('input', { value: u.display_name });
    const color = h('input', { type: 'color', value: u.color });
    const form = h('form', {}, h('div', { class: 'form-grid' }, field('Nom affiché', display), field('Couleur', color)),
      h('button', { class: 'btn tonal', type: 'submit' }, 'Enregistrer'));
    form.addEventListener('submit', (ev) => {
      ev.preventDefault();
      void this.act(() => this.api.patch(this.path(), { display_name: display.value.trim(), color: color.value }), 'Enregistré.');
    });
    return section('Profil', form);
  }

  private groupsCard(mine: string[], all: Group[]): HTMLElement {
    const checks = all.map((g) => h('input', { type: 'checkbox', value: g.name, checked: mine.includes(g.name) }));
    const form = h('form', {},
      h('div', { class: 'checks' }, ...all.map((g, i) => h('label', {}, checks[i]!, g.display_name))),
      h('p', { class: 'field-hint' }, "On voit les gens de ses groupes, et on ne partage qu'avec eux."),
      h('button', { class: 'btn tonal', type: 'submit' }, 'Enregistrer les groupes'));
    form.addEventListener('submit', (ev) => {
      ev.preventDefault();
      void this.act(() => this.api.patch(this.path(), { groups: checks.filter((c) => c.checked).map((c) => c.value) }), 'Groupes enregistrés.');
    });
    return section('Groupes', all.length > 0 ? form : h('p', { class: 'muted' }, 'Aucun groupe.'));
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
      this.doc.location.assign('/admin/users');
    } catch (e) {
      this.toasts.error(e);
    }
  }
}

export { confirmDialog };
