import { avatar, calendarMark, entries, groupMark, iconMark } from '@/admin/entries.ts';
import { icon } from '@/shared/components/icons.ts';
import { ui } from '@/shared/components/recipes.ts';
import { seg } from '@/shared/core/Api.ts';
import { h, svgDataUrl, type Child } from '@/shared/core/Dom.ts';
import { action, card, confirmDialog, dialog, field, formDialog, section } from '@/shared/components/Ui.ts';
import { ago, bytes, day, when } from '@/shared/render/format.ts';
import { AdminPage } from '@/admin/pages/base.ts';
import { accessLabel, tileLink } from '@/admin/pages/More.ts';
import { ActivityLog } from '@/admin/ActivityLog.ts';
import { t } from '@/admin/pages/Users.msg.ts';
import { pick } from '@/admin/Picker.ts';

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
  color: string;
}

export interface Enrolment {
  id: number;
  kind?: 'phone' | 'browser';
  state?: 'pending' | 'used' | 'expired';
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
  id: string;
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
  calendars_shared: { id: string; ref: string; name: string; access: string; via: string }[];
}

/** "vu il y a 3 jours", or "jamais vu". */
const seen = (iso: string | undefined): string => iso === undefined || iso === '' ? t('neverSeen') : t('seenAgo', { ago: ago(iso) });

/** Shows a fresh enrolment's QR code and link, once (PLAN.md 7.2). */
export async function showEnrolment(doc: Document, who: string, e: Enrolment): Promise<void> {
  if (e.kind === 'browser') {
    await dialog(doc, t('browserTitle', { name: who }), [
      h('p', {}, t('browserBody', { name: who, until: when(e.expires_at) })),
      h('p', { class: ui.secret }, e.url ?? ''),
      h('p', {}, h('a', { class: ui.btn('filled'), href: e.url ?? '#', target: '_blank', rel: 'noopener noreferrer' }, icon('link'), t('openInNewTab'))),
      h('p', { class: ui.notice }, icon('warn'), t('browserWarn', { name: who })),
    ], [{ label: t('close'), value: 'close', kind: 'text' }]);
    return;
  }
  const body: Child[] = [
    h('p', {}, t('phoneBody', { name: who, until: when(e.expires_at) })),
    e.qr_svg !== undefined ? h('img', { class: 'qr mx-auto rounded-md3-l p-2', src: svgDataUrl(e.qr_svg), alt: t('qrAlt'), width: 240, height: 240 }) : null,
    h('p', { class: 'text-xs text-on-surface-variant' }, t('orSendLink')),
    h('p', { class: ui.secret }, e.url ?? ''),
  ];
  await dialog(doc, t('firstSignin'), body, [{ label: t('close'), value: 'close', kind: 'filled' }]);
}

/** The users, and a form to add one. */
export class UsersPage extends AdminPage {
  protected async render(): Promise<Child[]> {
    const [{ users }, { groups }] = await Promise.all([
      this.api.get<{ users: UserRow[] }>('/users'),
      this.api.get<{ groups: Group[] }>('/groups'),
    ]);
    return [
      card(t('users'), [action(t('addUser'), () => this.create(groups), 'filled', 'plus')],
        entries(users.map((u) => ({
          href: `/users/${seg(u.name)}`,
          lead: avatar(u.display_name || u.name, u.color),
          title: u.display_name || u.name,
          sub: [u.name, u.groups.length > 0 ? u.groups.join(', ') : t('noGroup'), seen(u.last_seen_at)].join(' · '),
          tags: u.disabled ? [h('span', { class: ui.pill.err }, t('disabled'))] : [],
          stats: [{ n: String(u.devices), label: t('devicesStat', { count: u.devices }) }, { n: String(u.calendars_owned), label: t('calendarsStat', { count: u.calendars_owned }) }],
        })), t('noUsers'))),
    ];
  }

  /** "Ajouter un utilisateur": the form in a modal, then the sign-in QR. */
  private async create(groups: Group[]): Promise<void> {
    const name = h('input', { name: 'name', required: true, autocomplete: 'off', pattern: '[a-z][a-z0-9-]{0,31}', placeholder: 'lea' });
    const display = h('input', { name: 'display_name', placeholder: 'Léa' });
    const checks = groups.map((g) => h('input', { type: 'checkbox', value: g.name }));
    let made: { user: User; enrolment?: Enrolment } | null = null;
    const ok = await formDialog(this.doc, t('newUser'), [
      field(t('username'), name, t('usernameHelp')),
      field(t('displayName'), display),
      groups.length > 0
        ? h('div', { class: ui.field }, h('span', { class: ui.label }, t('groups')),
          h('div', { class: 'add-user-groups flex flex-wrap gap-x-5 gap-y-2' }, ...groups.map((g, i) => h('label', { class: ui.check }, checks[i]!, g.display_name))))
        : h('p', { class: 'text-sm' }, t('noGroupsYet')),
    ], t('add'), async () => {
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
    else this.toasts.show(t('addedNoQr'));
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
      case 'devices':
        // How this person gets in, on one page: the links that sign a
        // device in, the devices that sync, the browsers signed in.
        return [
          this.signin(u, d.enrolments),
          card(t('syncedDevices', { count: d.devices.length }), [],
            h('p', { class: 'text-sm text-on-surface-variant' }, t('devicesHelp')),
            entries(d.devices.map((x) => ({
              lead: iconMark(x.kind === 'thunderbird' ? 'laptop' : 'phone'),
              title: x.label,
              sub: [deviceKind(x.kind), x.cap === 'read' ? t('readOnly') : t('readWrite'),
                ...(x.last_used_at !== undefined ? [t('syncedAgo', { ago: ago(x.last_used_at) })] : []),
                ...(x.client ? [x.client] : [])].join(' · '),
              tags: x.last_used_at === undefined ? [h('span', { class: ui.pill.muted }, t('neverSynced'))] : [],
              actions: [action(t('revoke'), () => this.act(() => this.api.delete(`${this.path()}/devices/${x.id}`), t('deviceRevoked')), 'danger-text', 'trash', 'sm')],
            })), t('noDevices'))),
          card(t('browsers', { count: d.sessions.length }), [],
            entries(d.sessions.map((x) => ({
              lead: iconMark(x.app ? 'phone' : 'laptop'),
              title: x.app ? t('theApp') : browserOf(x.user_agent),
              sub: `${seen(x.seen_at)} · ${x.user_agent}`,
              actions: [action(t('signOut'), () => this.act(() => this.api.delete(`${this.path()}/sessions/${x.id}`), t('signedOut')), 'danger-text', 'logout', 'sm')],
            })), t('noBrowsers'))),
        ];
      case 'calendars':
        return [this.calendarsCard(d)];
      case 'groups':
        return [this.groupsCard(d.groups, groups)];
      case 'activity': {
        // The log of the activity page, this user's: the user is the page's
        // scope, the other columns still narrow it.
        const log = new ActivityLog(this.doc, this.api, { user: u.name });
        this.bag.defer(() => log.dispose());
        await log.load();
        return [section(t('activity'), log.root)];
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
          h('span', { class: 'text-sm text-on-surface-variant' }, [t('usernameIs', { name: u.name }), t('createdOn', { date: day(u.created_at) }), seen(u.last_seen_at)].join(' · ')),
          d.groups.length > 0 ? h('div', { class: 'flex flex-wrap gap-1.5 pt-1' }, ...d.groups.map((g) => h('a', { href: `/groups/${seg(g)}`, class: 'inline-flex h-6 items-center rounded-md3-s bg-surface-high px-2 text-xs font-semibold text-on-surface no-underline' }, g))) : null),
        u.disabled ? h('span', { class: ui.pill.err }, t('disabled')) : null),
      u.disabled ? h('p', { class: ui.error }, t('disabledNote')) : null,
      h('div', { class: 'grid gap-3 [grid-template-columns:repeat(auto-fill,minmax(150px,1fr))]' },
        tileLink(String(d.devices.length), t('devicesTile', { count: d.devices.length }), `${base}/devices`),
        tileLink(String(d.calendars_owned.length), t('ownedTile', { count: d.calendars_owned.length }), `${base}/calendars`),
        tileLink(String(d.calendars_shared.length), t('sharedTile', { count: d.calendars_shared.length }), `${base}/calendars`),
        tileLink(String(d.sessions.length), t('browsersTile', { count: d.sessions.length }), `${base}/devices`)),
      this.profile(u),
      section(t('account'),
        h('div', { class: 'flex flex-wrap items-center gap-2' },
          u.disabled
            ? action(t('enable'), () => this.act(() => this.api.patch(this.path(), { disabled: false }), t('enabled')))
            : action(t('disable'), () => this.act(() => this.api.patch(this.path(), { disabled: true }), t('disabledDone'))),
          action(t('deleteUser'), () => this.remove(u, d.calendars_owned, users), 'danger'),
        )),
    ];
  }

  /** The user's calendars as rows, each opening its page (stats, members, activity). */
  private calendarsCard(d: UserDetail): HTMLElement {
    const membersLine = (c: CalendarMeta): string => c.shares.length === 0 ? t('noMembers')
      : c.shares.map((s) => t(s.target.startsWith('g:') ? 'memberGroup' : 'memberUser', { name: s.target.slice(2), access: accessLabel(s.access) })).join(', ');
    return h('div', { class: 'flex flex-col gap-4' },
      section(t('ownCalendars', { count: d.calendars_owned.length }),
        h('p', { class: 'text-sm text-on-surface-variant' }, t('adminNoEvents')),
        entries(d.calendars_owned.map((c) => ({
          href: `/calendars/${seg(c.id)}`, lead: calendarMark(c.color), title: c.name, sub: `${c.ref} · ${membersLine(c)}`,
          stats: [{ n: String(c.events), label: t('eventsStat', { count: c.events }) }, { n: bytes(c.bytes), label: t('size') }],
        })), t('noOwnCalendars'))),
      section(t('sharedCalendars', { count: d.calendars_shared.length }),
        entries(d.calendars_shared.map((c) => ({
          href: `/calendars/${seg(c.id)}`, lead: calendarMark(''), title: c.name,
          sub: `${t('from', { owner: c.ref.split('~')[0] ?? '' })} · ${c.via.startsWith('g:') ? t('viaGroup', { group: c.via.slice(2) }) : t('viaDirect')}`,
          tags: [h('span', { class: c.access === 'read' ? ui.pill.muted : ui.pill.ok }, accessLabel(c.access))],
        })), t('noShared'))));
  }



  private profile(u: User): HTMLElement {
    const display = h('input', { value: u.display_name });
    const color = h('input', { type: 'color', value: u.color });
    const form = h('form', { class: 'flex flex-col items-start gap-4 [&>div]:w-full' }, h('div', { class: 'grid gap-4 [grid-template-columns:repeat(auto-fill,minmax(min(220px,100%),1fr))]' }, field(t('displayName'), display), field(t('colour'), color)),
      h('button', { class: ui.btn('tonal'), type: 'submit' }, t('save')));
    form.addEventListener('submit', (ev) => {
      ev.preventDefault();
      void this.act(() => this.api.patch(this.path(), { display_name: display.value.trim(), color: color.value }), t('profileSaved'));
    });
    return section(t('profile'), form);
  }

  /** The user's groups, a row each (opening the group), with the way out and the way in. */
  private groupsCard(mine: string[], all: Group[]): HTMLElement {
    const set = (groups: string[], done: string): Promise<void> => this.act(() => this.api.patch(this.path(), { groups }), done);
    const others = all.filter((g) => !mine.includes(g.name));
    return card(t('groups'), [action(t('addToGroup'), () => this.joinGroup(mine, others), 'filled', 'plus')],
      h('p', { class: 'text-sm text-on-surface-variant' }, t('groupsHelp')),
      entries(all.filter((g) => mine.includes(g.name)).map((g) => ({
        href: `/groups/${seg(g.name)}`,
        lead: groupMark(g.color),
        title: g.display_name || g.name,
        sub: g.name,
        actions: [action(t('remove'), () => set(mine.filter((x) => x !== g.name), t('removedFromGroup')), 'danger-text', 'close', 'sm')],
      })), t('inNoGroup')));
  }

  private async joinGroup(mine: string[], others: Group[]): Promise<void> {
    if (others.length === 0) {
      this.toasts.show(mine.length === 0 ? t('noGroupsAtAll') : t('inAllGroups'));
      return;
    }
    const picked = await pick(this.doc, {
      title: t('addToGroup'), multiple: true, confirm: t('add'),
      items: others.map((g) => ({ value: g.name, label: g.display_name || g.name, sub: g.name, lead: groupMark(g.color) })),
    });
    if (picked === null || picked.length === 0) return;
    await this.act(() => this.api.patch(this.path(), { groups: [...mine, ...picked] }), t('addedToGroupsN', { count: picked.length }));
  }



  /**
   * Its sign-in links, a row each: a QR for a phone, or a link to sign a
   * browser in (for testing). A pending one opens again, as often as
   * needed, until it is used, cancelled or expired.
   */
  private signin(u: User, enrolments: Enrolment[]): HTMLElement {
    const who = u.display_name || u.name;
    const make = async (kind: 'phone' | 'browser'): Promise<void> => {
      try {
        const e = await this.api.post<Enrolment>(`${this.path()}/enrolments`, { kind });
        await showEnrolment(this.doc, who, e);
        await this.refresh();
      } catch (err) {
        this.toasts.error(err);
      }
    };
    const show = async (e: Enrolment): Promise<void> => {
      try {
        await showEnrolment(this.doc, who, await this.api.get<Enrolment>(`${this.path()}/enrolments/${e.id}`));
      } catch (err) {
        this.toasts.error(err);
        await this.refresh();
      }
    };
    const STATE: Record<string, [string, string]> = { pending: [t('pending'), ui.pill.ok], used: [t('used'), ui.pill.muted], expired: [t('expired'), ui.pill.muted] };
    return card(t('signinLinks'), [
      action(t('makeQr'), () => make('phone'), 'filled', 'qr'),
      action(t('makeBrowserLink'), () => make('browser'), 'tonal', 'laptop'),
    ],
    h('p', { class: 'text-sm text-on-surface-variant' }, t('signinHelp')),
    entries(enrolments.map((e) => {
      const state = e.state ?? (e.used_at !== undefined ? 'used' : 'pending');
      const [label, cls] = STATE[state] ?? STATE['pending']!;
      return {
        open: state === 'pending' ? () => void show(e) : undefined,
        lead: iconMark(e.kind === 'browser' ? 'laptop' : 'qr'),
        title: e.kind === 'browser' ? t('browserLinkTitle') : t('qrTitle'),
        sub: state === 'used' && e.used_at !== undefined ? `${t('usedAgo', { ago: ago(e.used_at) })} · ${t('createdOn', { date: when(e.created_at) })}`
          : state === 'expired' ? t('expiredOn', { date: when(e.expires_at) }) : `${t('createdOn', { date: when(e.created_at) })} · ${t('expiresOn', { date: when(e.expires_at) })}`,
        tags: [h('span', { class: cls }, label)],
        actions: state === 'pending' ? [action(t('cancelLink'), () => this.act(() => this.api.delete(`${this.path()}/enrolments/${e.id}`), t('linkCancelled')), 'danger-text', 'close', 'sm')] : [],
      };
    }), t('noLinks')));
  }


  /** Removing asks, calendar by calendar, whom to give it to or to delete it (PLAN.md 6). */
  private async remove(u: User, owned: CalendarMeta[], users: UserRow[]): Promise<void> {
    const others = users.filter((x) => x.name !== u.name && !x.disabled);
    const label = (name: string): string => others.find((o) => o.name === name)?.display_name || name;
    const choices = owned.map((c) => {
      const state: { to: string; drop: boolean } = { to: others.length === 1 ? others[0]!.name : '', drop: others.length === 0 };
      const status = h('span', { class: 'text-sm text-on-surface-variant' });
      const choose = h('button', { type: 'button', class: ui.btn('tonal', 'sm') }, icon('give'));
      const drop = h('input', { type: 'checkbox', checked: state.drop, disabled: others.length === 0 });
      const draw = (): void => {
        status.textContent = state.drop ? t('willBeDeleted') : state.to !== '' ? t('willGoTo', { name: label(state.to) }) : t('noOwnerYet');
        status.className = !state.drop && state.to === '' ? 'text-sm font-semibold text-error' : 'text-sm text-on-surface-variant';
        choose.replaceChildren(icon('give'), state.to === '' ? t('chooseOwner') : t('changeOwner'));
        choose.disabled = state.drop;
      };
      choose.addEventListener('click', () => void pick(this.doc, {
        title: t('pickOwnerTitle', { name: c.name }),
        items: others.map((o) => ({ value: o.name, label: o.display_name || o.name, sub: o.name, lead: avatar(o.display_name || o.name, o.color) })),
        selected: state.to !== '' ? [state.to] : [],
      }).then((p) => {
        if (p !== null && p.length > 0) state.to = p[0]!;
        draw();
      }));
      drop.addEventListener('change', () => {
        state.drop = drop.checked;
        draw();
      });
      draw();
      const row = h('div', { class: 'flex flex-col gap-2 rounded-md3-l bg-surface-lowest p-3' },
        h('div', { class: 'flex items-center gap-3' }, calendarMark(c.color),
          h('span', { class: 'flex min-w-0 flex-1 flex-col' }, h('strong', { class: 'truncate' }, c.name), status), choose),
        h('label', { class: `${ui.check} text-sm` }, drop, t('deleteInstead')));
      return { c, state, row };
    });
    const body: Child[] = [
      h('p', {}, t('deleteBody', { name: u.display_name })),
      ...(owned.length > 0 ? [h('p', { class: 'text-sm' }, others.length === 0 ? t('theirCalendarsNobody') : t('theirCalendars')), ...choices.map((c) => c.row)] : []),
    ];
    const done = await formDialog(this.doc, t('deleteTitle', { name: u.display_name }), body, t('deleteUserNow'), async () => {
      const missing = choices.find((c) => !c.state.drop && c.state.to === '');
      if (missing !== undefined) throw new Error(t('mustChoose', { name: missing.c.name }));
      const calendars: Record<string, { transfer_to?: string; delete?: boolean }> = {};
      for (const c of choices) calendars[c.c.ref] = c.state.drop ? { delete: true } : { transfer_to: c.state.to };
      await this.api.post(`${this.path()}/remove`, { calendars });
    }, 'danger');
    if (done) this.doc.location.assign('/users');
  }

}

export { confirmDialog };

/** A small round icon button inside a chip. */
export function iconButtonSm(ic: 'close', label: string, run: () => Promise<void>): HTMLButtonElement {
  const b = h('button', { type: 'button', class: 'state-layer grid size-7 place-items-center rounded-full [&_.ic]:size-4', 'aria-label': label, title: label }, icon(ic));
  b.addEventListener('click', () => void run());
  return b;
}

const KIND = {
  davx5: 'kindDavx5', ios: 'kindIos', thunderbird: 'kindThunderbird', app: 'kindApp', other: 'kindOther',
} as const satisfies Record<string, Parameters<typeof t>[0]>;

/** A device's kind as a person reads it ("DAVx5", "Application agendouille"). */
export function deviceKind(kind: string): string {
  const key = (KIND as Record<string, Parameters<typeof t>[0]>)[kind];
  return key === undefined ? kind : t(key);
}

/** A browser's name from its user agent, roughly. */
function browserOf(ua: string): string {
  const os = /Android/.test(ua) ? 'Android' : /iPhone|iPad/.test(ua) ? 'iOS' : /Mac OS X/.test(ua) ? 'Mac' : /Windows/.test(ua) ? 'Windows' : /Linux/.test(ua) ? 'Linux' : '';
  const b = /Edg\//.test(ua) ? 'Edge' : /Firefox\//.test(ua) ? 'Firefox' : /Chrome\//.test(ua) ? 'Chrome' : /Safari\//.test(ua) ? 'Safari' : t('browser');
  return os === '' ? b : `${b}, ${os}`;
}
