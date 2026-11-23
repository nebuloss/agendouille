import { avatar, calendarMark, entries, groupMark } from '@/admin/entries.ts';
import { seg } from '@/shared/core/Api.ts';
import { h, type Child } from '@/shared/core/Dom.ts';
import { action, card, dialog, field, formDialog, section, styleControl, swatch, table } from '@/shared/components/Ui.ts';
import { ui } from '@/shared/components/recipes.ts';
import { icon } from '@/shared/components/icons.ts';
import { ago, bytes, day, when } from '@/shared/render/format.ts';
import { AdminPage } from '@/admin/pages/base.ts';
import { tileLink } from '@/admin/pages/More.ts';
import { ActivityLog } from '@/admin/ActivityLog.ts';
import { barList, dailyChart, type DayCount } from '@/admin/charts.ts';

/**
 * One calendar, as the admin sees it (PLAN.md 10: metadata, never an
 * event): its overview is its stats, at /calendars/<owner~slug>, and its
 * pages beside it: Partages, Activité. Reached from its owner; the trail
 * goes through them.
 */

export interface CalendarStats {
  ref: string;
  owner: string;
  name: string;
  color: string;
  timezone: string;
  description: string;
  events: number;
  bytes: number;
  recurring: number;
  all_day: number;
  changes: number;
  last_change?: string;
  created_at: string;
  shares: { target: string; access: string }[];
  days: number;
  daily: DayCount[];
  by_user: { user: string; changes: number; last: string }[];
  by_via: { via: string; changes: number }[];
  by_device: { label: string; user: string; kind: string; changes: number; last: string }[];
}

const VIA: Record<string, string> = { caldav: 'CalDAV (téléphones, ordinateurs)', dav: 'CalDAV (téléphones, ordinateurs)', web: 'application web', app: "l'app agendouille", import: 'import .ics', cli: 'console', test: 'test' };
const KIND: Record<string, string> = { davx5: 'DAVx5', ios: 'iPhone, iPad, Mac', thunderbird: 'Thunderbird', app: "l'app", other: 'autre' };

export class CalendarPage extends AdminPage {
  private get sub(): string {
    return this.root.dataset['sub'] ?? '';
  }

  private get ref(): string {
    return this.name;
  }

  protected async render(): Promise<Child[]> {
    const st = await this.api.get<CalendarStats>(`/calendars/${seg(this.ref)}/stats`);
    for (const el of this.doc.querySelectorAll<HTMLElement>('[data-item-name] span')) el.textContent = st.name;
    this.doc.title = `${st.name} · agendouille`;
    switch (this.sub) {
      case 'shares':
        return [await this.members(st)];
      case 'activity': {
        const log = new ActivityLog(this.doc, this.api, { calendar: st.ref });
        this.bag.defer(() => log.dispose());
        await log.load();
        return [section('Activité', log.root)];
      }
      default:
        return this.overview(st);
    }
  }

  private overview(st: CalendarStats): Child[] {
    const base = `/calendars/${seg(st.ref)}`;
    const share = st.events === 0 ? 0 : Math.round((st.recurring / st.events) * 100);
    return [
      h('section', { class: `${ui.card} flex-row flex-wrap items-center gap-4` },
        swatch(st.color, 'md'),
        h('div', { class: 'flex min-w-0 flex-1 flex-col gap-1' },
          h('h2', { class: 'truncate text-2xl font-normal' }, st.name),
          h('span', { class: 'text-sm text-on-surface-variant' },
            'À ', h('a', { href: `/users/${seg(st.owner)}`, class: ui.link }, st.owner),
            ` · ${st.ref} · ${st.timezone} · créé le ${day(st.created_at)}`),
          st.description !== '' ? h('p', { class: 'text-sm' }, st.description) : null),
        h('a', { class: ui.btn('tonal', 'sm'), href: this.api.url(`/calendars/${seg(st.ref)}/export`), download: `${st.ref}.ics` }, icon('download'), 'Télécharger en .ics')),
      h('div', { class: 'grid gap-3 [grid-template-columns:repeat(auto-fill,minmax(150px,1fr))]' },
        tile(String(st.events), 'événements', `dont ${st.recurring} récurrents (${share} %), ${st.all_day} sur la journée`),
        tile(bytes(st.bytes), 'de données', `soit ${st.events === 0 ? 0 : Math.round(st.bytes / st.events)} o par événement`),
        tile(String(st.changes), 'modifications', 'depuis sa création, gardées selon la durée de rétention'),
        tile(st.last_change !== undefined ? ago(st.last_change) : 'jamais', 'dernière modification', st.last_change !== undefined ? when(st.last_change) : ''),
        tileLink(String(st.shares.length), st.shares.length === 1 ? 'membre' : 'membres', `${base}/shares`)),
      section(`Modifications des ${st.days} derniers jours`, dailyChart(st.daily)),
      h('div', { class: ui.cards },
        section('Qui le modifie', barList(st.by_user.map((u) => ({ label: u.user || 'console', value: u.changes, sub: `dernière ${ago(u.last)}`, href: u.user ? `/users/${seg(u.user)}` : undefined })),
          'Personne ne l’a encore modifié.')),
        section('Par où', barList(st.by_via.map((v) => ({ label: VIA[v.via] ?? v.via, value: v.changes })), 'Aucune modification.'))),
      section('Appareils qui y écrivent',
        table(['Appareil', 'De', 'Type', 'Modifications', 'Dernière'],
          st.by_device.map((d) => [d.label, h('a', { href: `/users/${seg(d.user)}/devices`, class: ui.link }, d.user), KIND[d.kind] ?? d.kind, String(d.changes), ago(d.last)]),
          'Aucun appareil n’y a encore écrit.')),
      h('div', { class: ui.cards },
        section('Propriétaire',
          h('p', { class: 'flex items-center gap-3' },
            h('span', { class: 'grid size-10 place-items-center rounded-full bg-primary-container font-bold text-on-primary-container' }, st.owner.slice(0, 1).toUpperCase()),
            h('span', { class: 'flex flex-col' }, h('a', { href: `/users/${seg(st.owner)}`, class: ui.link }, st.owner),
              h('span', { class: 'text-xs text-on-surface-variant' }, 'le seul à pouvoir le partager, le renommer ou le supprimer depuis son application'))),
          action('Changer de propriétaire…', () => this.transfer(st), 'tonal', 'give')),
        section('Supprimer le calendrier',
          h('p', { class: 'text-sm text-on-surface-variant' }, 'Ses événements disparaissent pour tout le monde, sur tous les appareils.'),
          action('Supprimer…', () => this.remove(st), 'danger', 'trash'))),
    ];
  }

  /**
   * Who is in the calendar: its owner, then its members (users and groups
   * it is shared with). The admin adds a member, changes their access, or
   * takes them out; the owner changes with "Changer de propriétaire".
   */
  private async members(st: CalendarStats): Promise<HTMLElement> {
    const [{ users }, { groups }] = await Promise.all([
      this.api.get<{ users: { name: string; display_name: string; color: string }[] }>('/users'),
      this.api.get<{ groups: { name: string; display_name: string; color: string; members: string[] }[] }>('/groups'),
    ]);
    const accessSelect = (t: string, current: string, label: string): HTMLSelectElement => {
      const sel = styleControl(h('select', { 'aria-label': `Accès de ${label}` },
        h('option', { value: 'read', selected: current === 'read' }, 'lecture'),
        h('option', { value: 'readwrite', selected: current === 'readwrite' }, 'lecture et écriture')));
      sel.classList.add('!w-44', '!min-h-9', '!py-1');
      sel.addEventListener('change', () => void this.act(() => this.api.put(`/calendars/${seg(st.ref)}/shares/${seg(t)}`, { access: sel.value }), 'Accès changé.'));
      return sel;
    };
    const owner = users.find((u) => u.name === st.owner);
    const list = [
      {
        href: `/users/${seg(st.owner)}`,
        lead: avatar(owner?.display_name || st.owner, owner?.color ?? ''),
        title: owner?.display_name || st.owner,
        sub: `${st.owner} · peut tout faire, dont le partager et le supprimer`,
        tags: [h('span', { class: ui.pill.ok }, 'propriétaire')],
        actions: [action('Changer…', () => this.transfer(st), 'text', 'give', 'sm')],
      },
      ...st.shares.map((m) => {
        const isGroup = m.target.startsWith('g:');
        const name = m.target.slice(2);
        const g = isGroup ? groups.find((x) => x.name === name) : undefined;
        const u = isGroup ? undefined : users.find((x) => x.name === name);
        const title = isGroup ? `Groupe ${g?.display_name || name}` : u?.display_name || name;
        return {
          href: isGroup ? `/groups/${seg(name)}` : `/users/${seg(name)}`,
          lead: isGroup ? groupMark(g?.color ?? '') : avatar(u?.display_name || name, u?.color ?? ''),
          title,
          sub: isGroup ? `${name} · ${g === undefined || g.members.length === 0 ? 'personne dedans' : g.members.join(', ')}` : name,
          actions: [accessSelect(m.target, m.access, title),
            action('Retirer', () => this.act(() => this.api.delete(`/calendars/${seg(st.ref)}/shares/${seg(m.target)}`), 'Membre retiré.'), 'danger-text', 'close', 'sm')],
        };
      }),
    ];
    return card('Membres', [action('Ajouter un membre', () => this.addMember(st, users, groups), 'filled', 'plus')],
      h('p', { class: 'text-sm text-on-surface-variant' }, "Un membre voit le calendrier sur ses appareils ; en lecture et écriture, il y ajoute et modifie des événements. Un groupe, c'est chacun de ses membres."),
      entries(list, ''));
  }


  private async addMember(st: CalendarStats, users: { name: string; display_name: string }[], groups: { name: string; display_name: string }[]): Promise<void> {
    const taken = new Set(st.shares.map((m) => m.target));
    const options = [
      ...groups.filter((g) => !taken.has(`g:${g.name}`)).map((g) => h('option', { value: `g:${g.name}` }, `Groupe ${g.display_name || g.name}`)),
      ...users.filter((u) => u.name !== st.owner && !taken.has(`u:${u.name}`)).map((u) => h('option', { value: `u:${u.name}` }, u.display_name || u.name)),
    ];
    if (options.length === 0) {
      this.toasts.show('Tout le monde en est déjà membre.');
      return;
    }
    const who = h('select', { required: true }, ...options);
    const access = h('select', {}, h('option', { value: 'read' }, 'Lecture seule'), h('option', { value: 'readwrite' }, 'Lecture et écriture'));
    const ok = await formDialog(this.doc, `Ajouter un membre à ${st.name}`, [field('Qui', who), field('Accès', access)], 'Ajouter',
      () => this.api.put(`/calendars/${seg(st.ref)}/shares/${seg(who.value)}`, { access: access.value }));
    if (!ok) return;
    this.toasts.show('Membre ajouté.');
    this.doc.dispatchEvent(new Event('agd:changed'));
    await this.refresh();
  }

  private async transfer(st: CalendarStats): Promise<void> {
    const { users } = await this.api.get<{ users: { name: string; display_name: string }[] }>('/users');
    const select = h('select', {}, ...users.filter((u) => u.name !== st.owner).map((u) => h('option', { value: u.name }, u.display_name || u.name)));
    const keep = h('input', { type: 'checkbox', checked: true });
    const v = await dialog(this.doc, `Changer le propriétaire de ${st.name}`, [
      h('p', {}, "Le calendrier passe à un autre compte, avec ses événements et ses membres. Son adresse change : les téléphones le retrouvent à la synchronisation suivante."),
      field('Nouveau propriétaire', select), h('label', { class: ui.check }, keep, `${st.owner} reste membre, en lecture et écriture`)],
    [{ label: 'Annuler', value: 'no' }, { label: 'Changer', value: 'yes', kind: 'filled' }]);
    if (v !== 'yes' || select.value === '') return;
    try {
      const r = await this.api.post<{ ref?: string }>(`/calendars/${seg(st.ref)}/transfer`, { to: select.value, keep: keep.checked });
      this.doc.location.assign(r.ref !== undefined ? `/calendars/${seg(r.ref)}` : `/users/${seg(select.value)}/calendars`);
    } catch (e) {
      this.toasts.error(e);
    }
  }

  private async remove(st: CalendarStats): Promise<void> {
    const typed = h('input', { autocomplete: 'off' });
    const v = await dialog(this.doc, `Supprimer ${st.name} ?`, [
      h('p', {}, `Ses ${st.events} événement(s) disparaissent pour tout le monde. Seule une sauvegarde peut les rendre.`),
      field(`Tapez « ${st.name} » pour confirmer`, typed),
    ], [{ label: 'Annuler', value: 'no' }, { label: 'Supprimer', value: 'yes', kind: 'danger' }]);
    if (v !== 'yes') return;
    try {
      await this.api.delete(`/calendars/${seg(st.ref)}`, { confirm: typed.value });
      this.doc.location.assign('/calendars');
    } catch (e) {
      this.toasts.error(e);
    }
  }
}

/** A stat tile with a line under it. */
function tile(n: string, label: string, sub: string): HTMLElement {
  return h('div', { class: 'flex flex-col gap-1 rounded-md3-xl bg-surface-container p-4' },
    h('span', { class: n.length > 8 ? 'text-xl leading-9 font-semibold' : 'text-[28px] leading-9 font-semibold' }, n), h('span', { class: 'text-sm' }, label),
    sub !== '' ? h('span', { class: 'text-xs text-on-surface-variant' }, sub) : null);
}

interface CalendarRow {
  ref: string;
  owner: string;
  name: string;
  color: string;
  events: number;
  bytes: number;
  shares: { target: string; access: string }[];
  created_at: string;
}

/**
 * Calendriers: every calendar, as a list that opens each one (its stats,
 * its members, its activity), with a search over name, owner and member.
 */
export class CalendarsPage extends AdminPage {
  private query = '';

  protected async render(): Promise<Child[]> {
    const { calendars } = await this.api.get<{ calendars: CalendarRow[] }>('/calendars');
    const search = h('input', { type: 'search', value: this.query, placeholder: 'Nom, propriétaire, membre', 'aria-label': 'Chercher un calendrier',
      class: 'min-w-0 flex-1 border-0 bg-transparent text-sm text-on-surface outline-none' });
    const list = h('div', {});
    const count = h('span', { class: 'text-xs text-on-surface-variant' });
    const draw = (): void => {
      const q = this.query.trim().toLowerCase();
      const shown = calendars.filter((c) => q === '' || [c.name, c.ref, c.owner, ...c.shares.map((s) => s.target)].some((x) => x.toLowerCase().includes(q)));
      count.textContent = `${shown.length} sur ${calendars.length}`;
      list.replaceChildren(entries(shown.map((c) => ({
        href: `/calendars/${seg(c.ref)}`,
        lead: calendarMark(c.color),
        title: c.name,
        sub: `à ${c.owner} · ${c.shares.length === 0 ? 'aucun membre' : c.shares.map((s) => `${s.target.startsWith('g:') ? 'groupe ' : ''}${s.target.slice(2)}`).join(', ')}`,
        stats: [{ n: String(c.events), label: 'événements' }, { n: bytes(c.bytes), label: 'taille' }],
      })), calendars.length === 0 ? "Aucun calendrier : les gens en créent depuis l'application ou leur téléphone." : 'Aucun ne correspond.'));
    };
    search.addEventListener('input', () => {
      this.query = search.value;
      draw();
    });
    draw();
    return [card('Calendriers', [h('label', { class: 'flex h-9 min-w-56 items-center gap-2 rounded-md3-full bg-surface-lowest px-3 text-on-surface-variant' }, icon('search'), search), count],
      h('p', { class: 'text-sm text-on-surface-variant' }, "L'administration voit les calendriers, jamais leurs événements. Ouvrez-en un pour ses statistiques, son propriétaire et ses membres."),
      list)];
  }
}
