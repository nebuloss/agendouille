import { seg, type Api } from '@/shared/core/Api.ts';
import { h, svgDataUrl, type Child } from '@/shared/core/Dom.ts';
import { ApiPage } from '@/shared/components/ApiPage.ts';
import { action, confirmDialog, dialog, field, section, swatch, table } from '@/shared/components/Ui.ts';
import { ago } from '@/shared/render/format.ts';
import { appApi, type Calendar, type Calendars, type Me, type Person, type Share } from '@/app/api.ts';

/** A page of the family web app that renders from the API into #app-content. */
abstract class AppPage extends ApiPage {
  constructor(doc: Document, api: Api = appApi()) {
    super(doc, api, 'app-content');
  }
}

const ACCESS: Record<string, string> = { owner: 'à moi', readwrite: 'lecture et écriture', read: 'lecture seule' };
const COLORS = ['#1e88e5', '#43a047', '#e53935', '#fb8c00', '#8e24aa', '#00897b', '#6d4c41', '#3949ab'];

interface People {
  users: Person[];
  groups: Person[];
}

/** Calendriers (PLAN.md 9.3): mine, shared with me; create, share, hide. */
export class CalendarsPage extends AppPage {
  protected async render(): Promise<Child[]> {
    const [cals, people] = await Promise.all([this.api.get<Calendars>('/calendars'), this.api.get<People>('/people')]);
    const me = this.doc.querySelector<HTMLElement>('.app')?.dataset['user'] ?? '';
    const mine = cals.calendars.filter((c) => c.owner === me);
    const others = cals.calendars.filter((c) => c.owner !== me);
    const newBtn = action('+ Nouveau calendrier', () => this.create(), 'filled');
    return [
      section('Mes calendriers',
        h('div', { class: 'card-actions' }, newBtn),
        mine.length === 0 ? h('p', { class: 'empty' }, "Vous n'avez pas encore de calendrier.") : null,
        ...mine.map((c) => this.ownedCard(c, people))),
      section('Partagés avec moi',
        others.length === 0 ? h('p', { class: 'empty' }, 'Personne ne vous a encore partagé de calendrier.') : null,
        ...others.map((c) => this.sharedCard(c))),
    ];
  }

  private nameOf(people: People, target: string): string {
    const [kind, name] = [target.slice(0, 1), target.slice(2)];
    const list = kind === 'g' ? people.groups : people.users;
    const p = list.find((x) => x.name === name);
    return (kind === 'g' ? 'groupe ' : '') + (p?.display_name || name);
  }

  private ownedCard(c: Calendar, people: People): HTMLElement {
    const shares = c.shares ?? [];
    const path = `/calendars/${seg(c.ref)}`;
    const shareRows = shares.map((s: Share) => [
      this.nameOf(people, s.target),
      ACCESS[s.access] ?? s.access,
      action('Retirer', () => this.act(() => this.api.delete(`${path}/shares/${seg(s.target)}`), 'Partage retiré.'), 'text'),
    ]);
    return h('div', { class: 'cal-card' },
      h('div', { class: 'cal-card-head' }, swatch(c.color), h('strong', {}, c.name), c.hidden ? h('span', { class: 'chip' }, 'masqué') : null),
      c.description !== '' ? h('p', { class: 'muted' }, c.description) : null,
      shares.length > 0 ? table(['Partagé avec', 'Accès', ''], shareRows) : h('p', { class: 'muted' }, 'Partagé avec personne.'),
      h('div', { class: 'card-actions' },
        action('Partager', () => this.share(c, people)),
        action('Modifier', () => this.editProps(c), 'text'),
        h('a', { class: 'btn text', href: this.api.url(`${path}/export`), download: `${c.ref}.ics` }, 'Exporter'),
        action('Importer', () => this.importInto(c), 'text'),
        action('Donner', () => this.transfer(c, people), 'text'),
        action('Supprimer', () => this.remove(c), 'danger')));
  }

  private sharedCard(c: Calendar): HTMLElement {
    const path = `/calendars/${seg(c.ref)}`;
    return h('div', { class: 'cal-card' },
      h('div', { class: 'cal-card-head' }, swatch(c.color), h('strong', {}, c.name), h('span', { class: 'muted' }, ` · ${c.owner}`),
        h('span', { class: 'chip' }, ACCESS[c.access] ?? c.access)),
      h('div', { class: 'card-actions' },
        c.hidden
          ? action('Ne plus masquer', () => this.act(() => this.api.patch(path, { hidden: false }), 'Il revient sur vos appareils.'))
          : action('Masquer', () => this.act(() => this.api.patch(path, { hidden: true }), 'Masqué, ici et sur vos appareils.'), 'text'),
        action('Ma couleur', () => this.editProps(c), 'text')));
  }

  private colorPicker(current: string): HTMLInputElement {
    return h('input', { type: 'color', value: /^#[0-9a-f]{6}$/i.test(current) ? current : COLORS[0]! });
  }

  private async create(): Promise<void> {
    const name = h('input', { required: true, placeholder: 'Ex. École de Léa' });
    const color = this.colorPicker(COLORS[Math.floor(Math.random() * COLORS.length)]!);
    const v = await dialog(this.doc, 'Nouveau calendrier', [field('Nom', name), field('Couleur', color)],
      [{ label: 'Annuler', value: '' }, { label: 'Créer', value: 'ok', kind: 'filled' }]);
    if (v !== 'ok' || name.value.trim() === '') return;
    await this.act(() => this.api.post('/calendars', { name: name.value.trim(), color: color.value }), 'Calendrier créé.');
  }

  private async editProps(c: Calendar): Promise<void> {
    const owner = c.access === 'owner';
    const name = h('input', { value: c.name, required: true });
    const color = this.colorPicker(c.color);
    const desc = h('textarea', { rows: 2 });
    desc.value = c.description;
    const v = await dialog(this.doc, owner ? 'Modifier le calendrier' : 'Chez moi', [
      field(owner ? 'Nom' : 'Nom (pour moi)', name), field('Couleur', color), owner ? field('Description', desc) : null,
    ], [{ label: 'Annuler', value: '' }, { label: 'Enregistrer', value: 'ok', kind: 'filled' }]);
    if (v !== 'ok') return;
    const body: Record<string, string> = { name: name.value.trim(), color: color.value };
    if (owner) body['description'] = desc.value;
    await this.act(() => this.api.patch(`/calendars/${seg(c.ref)}`, body), 'Enregistré.');
  }

  private async share(c: Calendar, people: People): Promise<void> {
    const options = [
      ...people.groups.map((g) => h('option', { value: `g:${g.name}` }, `Groupe ${g.display_name || g.name}`)),
      ...people.users.map((u) => h('option', { value: `u:${u.name}` }, u.display_name || u.name)),
    ];
    if (options.length === 0) {
      this.toasts.show("Il n'y a personne dans vos groupes : l'administrateur vous en ajoute.", 'error');
      return;
    }
    const who = h('select', {}, ...options);
    const access = h('select', {}, h('option', { value: 'read' }, 'Lecture seule'), h('option', { value: 'readwrite' }, 'Lecture et écriture'));
    const v = await dialog(this.doc, `Partager « ${c.name} »`, [field('Avec', who), field('Accès', access)],
      [{ label: 'Annuler', value: '' }, { label: 'Partager', value: 'ok', kind: 'filled' }]);
    if (v !== 'ok') return;
    await this.act(() => this.api.put(`/calendars/${seg(c.ref)}/shares/${seg(who.value)}`, { access: access.value }), 'Partagé.');
  }

  private async transfer(c: Calendar, people: People): Promise<void> {
    if (people.users.length === 0) return;
    const who = h('select', {}, ...people.users.map((u) => h('option', { value: u.name }, u.display_name || u.name)));
    const v = await dialog(this.doc, `Donner « ${c.name} »`, [
      h('p', {}, "Le calendrier devient le sien ; vous le gardez en lecture et écriture. L'adresse change : vos appareils le retrouvent seuls."),
      field('À', who),
    ], [{ label: 'Annuler', value: '' }, { label: 'Donner', value: 'ok', kind: 'filled' }]);
    if (v !== 'ok') return;
    await this.act(() => this.api.post(`/calendars/${seg(c.ref)}/transfer`, { to: who.value }), 'Calendrier donné.');
  }

  private async importInto(c: Calendar): Promise<void> {
    const file = h('input', { type: 'file', accept: '.ics,text/calendar' });
    const replace = h('input', { type: 'checkbox' });
    const v = await dialog(this.doc, `Importer dans « ${c.name} »`, [
      file, h('label', { class: 'checks' }, replace, 'Remplacer les événements déjà là (même UID)'),
    ], [{ label: 'Annuler', value: '' }, { label: 'Importer', value: 'ok', kind: 'filled' }]);
    const f = file.files?.[0];
    if (v !== 'ok' || f === undefined) return;
    try {
      const r = await this.api.upload<{ added: number; replaced: number; skipped: number; errors: string[] }>(
        `/calendars/${seg(c.ref)}/import`, f, { replace: replace.checked ? '1' : undefined });
      this.toasts.show(`${r.added} ajoutés, ${r.replaced} remplacés, ${r.skipped} ignorés.`);
      await this.refresh();
    } catch (e) {
      this.toasts.error(e);
    }
  }

  private async remove(c: Calendar): Promise<void> {
    if (!(await confirmDialog(this.doc, `Supprimer « ${c.name} » ?`, 'Ses événements disparaissent pour tout le monde, sur tous les appareils.', 'Supprimer', true))) return;
    await this.act(() => this.api.delete(`/calendars/${seg(c.ref)}`), 'Calendrier supprimé.');
  }
}

interface Device {
  id: number;
  kind: string;
  label: string;
  cap: string;
  client: string;
  created_at: string;
  last_used_at?: string;
}

interface NewDevice extends Device {
  username: string;
  secret: string;
  davx5_uri?: string;
  qr_svg?: string;
  mobileconfig_url?: string;
  manual: { server: string; username: string };
}

interface Link {
  id: number;
  label: string;
  calendars: string[];
  created_at: string;
  last_used_at?: string;
  url?: string;
}

interface Session {
  id: number;
  user_agent: string;
  app: boolean;
  seen_at: string;
  current: boolean;
}

const KINDS: Record<string, string> = { davx5: 'DAVx5 (Android)', ios: 'iPhone, iPad, Mac', thunderbird: 'Thunderbird', other: 'Autre', app: "L'app agendouille" };

/** Appareils (PLAN.md 9.3, 8.4): phones and computers that sync, links, browsers. */
export class DevicesPage extends AppPage {
  protected async render(): Promise<Child[]> {
    const [devices, links, sessions, cals] = await Promise.all([
      this.api.get<{ devices: Device[] }>('/devices'), this.api.get<{ links: Link[] }>('/links'),
      this.api.get<{ sessions: Session[] }>('/sessions'), this.api.get<Calendars>('/calendars'),
    ]);
    const name = (ref: string): string => cals.calendars.find((c) => c.ref === ref)?.name ?? ref;
    return [
      section('Synchroniser un appareil',
        h('p', {}, 'Chaque appareil a son propre mot de passe, que vous pouvez retirer seul.'),
        h('div', { class: 'card-actions' },
          action('Android (DAVx5)', () => this.add('davx5'), 'filled'),
          action('iPhone, iPad, Mac', () => this.add('ios')),
          action('Thunderbird', () => this.add('thunderbird')),
          action('Autre', () => this.add('other'), 'text')),
        table(['Appareil', 'Accès', 'Dernière synchro', ''], devices.devices.map((d) => [
          h('span', {}, h('strong', {}, d.label), h('br', {}), h('span', { class: 'muted' }, KINDS[d.kind] ?? d.kind)),
          d.cap === 'read' ? 'lecture seule' : 'lecture et écriture',
          d.last_used_at ? ago(d.last_used_at) : 'jamais',
          action('Retirer', () => this.revoke(`/devices/${d.id}`, `Retirer « ${d.label} » ?`, 'Il ne pourra plus synchroniser.'), 'danger'),
        ]), "Aucun appareil pour l'instant.")),
      section("Liens d'abonnement",
        h('p', {}, 'Une adresse .ics en lecture seule, pour une télé ou un agenda qui ne sait pas CalDAV. Qui a le lien voit les calendriers choisis.'),
        h('div', { class: 'card-actions' }, action('+ Nouveau lien', () => this.addLink(cals.calendars))),
        table(['Lien', 'Calendriers', 'Dernière lecture', ''], links.links.map((l) => [
          l.label, l.calendars.map(name).join(', '), l.last_used_at ? ago(l.last_used_at) : 'jamais',
          action('Retirer', () => this.revoke(`/links/${l.id}`, `Retirer le lien « ${l.label} » ?`, "L'adresse cesse de marcher."), 'danger'),
        ]), 'Aucun lien.')),
      section('Navigateurs connectés',
        table(['Navigateur', 'Vu', ''], sessions.sessions.map((s) => [
          h('span', {}, s.app ? "L'app agendouille" : browserName(s.user_agent), s.current ? h('span', { class: 'chip' }, 'celui-ci') : null),
          ago(s.seen_at),
          s.current ? null : action('Déconnecter', () => this.revoke(`/sessions/${s.id}`, 'Déconnecter ce navigateur ?', 'Il faudra le reconnecter.'), 'danger'),
        ]))),
    ];
  }

  private async revoke(path: string, title: string, text: string): Promise<void> {
    if (!(await confirmDialog(this.doc, title, text, 'Retirer', true))) return;
    await this.act(() => this.api.delete(path), 'Retiré.');
  }

  private async add(kind: string): Promise<void> {
    const label = h('input', { required: true, value: kind === 'ios' ? 'iPhone' : kind === 'davx5' ? 'Téléphone' : 'Ordinateur' });
    const cap = h('select', {}, h('option', { value: 'readwrite' }, 'Lecture et écriture'), h('option', { value: 'read' }, 'Lecture seule'));
    const v = await dialog(this.doc, `Ajouter : ${KINDS[kind] ?? kind}`, [field('Nom', label), field('Accès', cap)],
      [{ label: 'Annuler', value: '' }, { label: 'Ajouter', value: 'ok', kind: 'filled' }]);
    if (v !== 'ok') return;
    let d: NewDevice;
    try {
      d = await this.api.post<NewDevice>('/devices', { kind, label: label.value.trim(), cap: cap.value });
    } catch (e) {
      this.toasts.error(e);
      return;
    }
    const manual = h('dl', { class: 'manual' },
      h('dt', {}, 'Serveur'), h('dd', {}, h('code', {}, d.manual.server)),
      h('dt', {}, 'Utilisateur'), h('dd', {}, h('code', {}, d.username)),
      h('dt', {}, 'Mot de passe'), h('dd', {}, h('code', { class: 'secret' }, d.secret)));
    const body: Child[] = [];
    if (d.davx5_uri !== undefined) {
      body.push(h('p', {}, 'Sur le téléphone, installez DAVx5, puis scannez ce code avec l’appareil photo, ou touchez le lien depuis le téléphone.'));
      if (d.qr_svg !== undefined) body.push(h('img', { class: 'qr', src: svgDataUrl(d.qr_svg), alt: 'QR code pour DAVx5', width: 240, height: 240 }));
      body.push(h('p', {}, h('a', { href: d.davx5_uri }, 'Ouvrir dans DAVx5')));
    }
    if (d.mobileconfig_url !== undefined) {
      body.push(h('p', {}, 'Ouvrez cette page sur l’iPhone, touchez le bouton, puis Réglages, Profil téléchargé, Installer. Le lien ne marche qu’une fois, dix minutes.'),
        h('p', {}, h('a', { class: 'btn filled', href: d.mobileconfig_url }, 'Installer le profil')));
    }
    body.push(h('p', { class: 'muted' }, 'Ou à la main, dans un compte CalDAV :'), manual,
      h('p', { class: 'banner' }, 'Le mot de passe ne sera plus affiché. Retirez l’appareil pour en refaire un.'));
    await dialog(this.doc, `« ${d.label} » ajouté`, body, [{ label: "C'est fait", value: 'ok', kind: 'filled' }]);
    await this.refresh();
  }

  private async addLink(cals: Calendar[]): Promise<void> {
    const label = h('input', { required: true, value: 'Télé du salon' });
    const boxes = cals.map((c) => ({ c, box: h('input', { type: 'checkbox', checked: c.shown }) }));
    const v = await dialog(this.doc, "Nouveau lien d'abonnement", [field('Nom', label),
      h('div', { class: 'cal-picker' }, ...boxes.map(({ c, box }) => h('label', { class: 'checks' }, box, swatch(c.color), c.name)))],
    [{ label: 'Annuler', value: '' }, { label: 'Créer', value: 'ok', kind: 'filled' }]);
    const chosen = boxes.filter((b) => b.box.checked).map((b) => b.c.ref);
    if (v !== 'ok' || chosen.length === 0) return;
    try {
      const l = await this.api.post<Link>('/links', { label: label.value.trim(), calendars: chosen });
      await dialog(this.doc, 'Lien créé', [h('p', {}, "Copiez l'adresse maintenant : elle ne sera plus affichée."), h('p', {}, h('code', { class: 'secret' }, l.url ?? ''))],
        [{ label: "C'est copié", value: 'ok', kind: 'filled' }]);
      await this.refresh();
    } catch (e) {
      this.toasts.error(e);
    }
  }
}

/** A browser's name from its user agent, roughly. */
export function browserName(ua: string): string {
  const os = /Android/.test(ua) ? 'Android' : /iPhone|iPad/.test(ua) ? 'iOS' : /Mac OS X/.test(ua) ? 'Mac' : /Windows/.test(ua) ? 'Windows' : /Linux/.test(ua) ? 'Linux' : '';
  const b = /Edg\//.test(ua) ? 'Edge' : /Firefox\//.test(ua) ? 'Firefox' : /Chrome\//.test(ua) ? 'Chrome' : /Safari\//.test(ua) ? 'Safari' : 'Navigateur';
  return os === '' ? b : `${b}, ${os}`;
}

/** Moi (PLAN.md 9.3): my name and colour, my groups, enrol another phone. */
export class MePage extends AppPage {
  protected async render(): Promise<Child[]> {
    const me = await this.api.get<Me>('/me');
    const name = h('input', { value: me.display_name, required: true });
    const color = h('input', { type: 'color', value: /^#[0-9a-f]{6}$/i.test(me.color) ? me.color : '#1e88e5' });
    return [
      section('Moi',
        field('Nom affiché', name), field('Couleur', color),
        h('div', { class: 'card-actions' }, action('Enregistrer', () => this.act(() => this.api.patch('/me', { display_name: name.value.trim(), color: color.value }), 'Enregistré.'), 'filled'))),
      section('Mes groupes',
        me.groups.length === 0 ? h('p', { class: 'empty' }, "Aucun groupe : l'administrateur vous en ajoute.") : h('p', {}, me.groups.map((g) => g.display_name || g.name).join(', ')),
        h('p', { class: 'muted' }, 'Vous voyez les gens de vos groupes, et pouvez partager vos calendriers avec eux.')),
      section('Connecter un autre téléphone',
        h('p', {}, "Un code valable dix minutes, à scanner avec l'appareil photo du nouveau téléphone."),
        action('Afficher le code', () => this.enrol())),
    ];
  }

  private async enrol(): Promise<void> {
    try {
      const e = await this.api.post<{ url: string; qr_svg: string }>('/me/enrolment');
      await dialog(this.doc, 'Scannez avec le nouveau téléphone', [
        h('img', { class: 'qr', src: svgDataUrl(e.qr_svg), alt: 'QR code de connexion', width: 240, height: 240 }),
        h('p', { class: 'muted' }, 'Valable dix minutes, une seule fois.'),
      ], [{ label: 'Fermer', value: '', kind: 'filled' }]);
    } catch (e) {
      this.toasts.error(e);
    }
  }
}
