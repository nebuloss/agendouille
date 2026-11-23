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
import { t } from '@/admin/pages/Calendar.msg.ts';

/**
 * One calendar, as the admin sees it (PLAN.md 10: metadata, never an
 * event): its overview is its stats, at /calendars/<owner~slug>, and its
 * pages beside it: Membres, Activité. Reached from its owner; the trail
 * goes through them.
 */

export interface CalendarStats {
  id: string;
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

type Key = Parameters<typeof t>[0];
const VIA: Readonly<Record<string, Key>> = { caldav: 'viaCaldav', dav: 'viaCaldav', web: 'viaWeb', app: 'viaApp', import: 'viaImport', cli: 'viaCli', test: 'viaTest' };
const KIND: Readonly<Record<string, Key>> = { davx5: 'kindDavx5', ios: 'kindIos', thunderbird: 'kindThunderbird', app: 'kindApp', other: 'kindOther' };

/** A code's label from a table of keys, or the code itself. */
const labelOf = (table: Readonly<Record<string, Key>>, code: string): string => {
  const key = table[code];
  return key === undefined ? code : t(key);
};

/**
 * A message with one element in it (a link): the message is rendered with
 * a marker in the element's place, then split there, so the words keep the
 * language's own order.
 */
function withElement(key: Key, param: string, el: HTMLElement, params: Record<string, string | number> = {}): Child[] {
  const mark = '\u0000';
  const [before = '', after = ''] = t(key, { ...params, [param]: mark }).split(mark);
  return [before, el, after];
}

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
        return [section(t('activity'), log.root)];
      }
      default:
        return this.overview(st);
    }
  }

  private overview(st: CalendarStats): Child[] {
    const base = `/calendars/${seg(st.id)}`;
    const share = st.events === 0 ? 0 : Math.round((st.recurring / st.events) * 100);
    return [
      h('section', { class: `${ui.card} flex-row flex-wrap items-center gap-4` },
        swatch(st.color, 'md'),
        h('div', { class: 'flex min-w-0 flex-1 flex-col gap-1' },
          h('h2', { class: 'truncate text-2xl font-normal' }, st.name),
          h('span', { class: 'text-sm text-on-surface-variant' },
            ...withElement('ownerIs', 'owner', h('a', { href: `/users/${seg(st.owner)}`, class: ui.link }, st.owner)),
            ` · ${st.ref} · ${st.timezone} · ${t('createdOn', { date: day(st.created_at) })}`),
          st.description !== '' ? h('p', { class: 'text-sm' }, st.description) : null),
        h('a', { class: ui.btn('tonal', 'sm'), href: this.api.url(`/calendars/${seg(st.id)}/export`), download: `${st.ref}.ics` }, icon('download'), t('download'))),
      h('div', { class: 'grid gap-3 [grid-template-columns:repeat(auto-fill,minmax(150px,1fr))]' },
        tile(String(st.events), t('eventsTile', { count: st.events }), t('eventsSub', { recurring: st.recurring, share, allDay: st.all_day })),
        tile(bytes(st.bytes), t('dataTile'), t('dataSub', { size: bytes(st.events === 0 ? 0 : Math.round(st.bytes / st.events)) })),
        tile(String(st.changes), t('changesTile', { count: st.changes }), t('changesSub')),
        tile(ago(st.last_change), t('lastChangeTile'), st.last_change !== undefined ? when(st.last_change) : ''),
        tileLink(String(st.shares.length), t('membersTile', { count: st.shares.length }), `${base}/shares`)),
      section(t('recentChanges', { count: st.days }), dailyChart(st.daily)),
      h('div', { class: ui.cards },
        section(t('whoChanges'), barList(st.by_user.map((u) => ({ label: u.user || t('viaCli'), value: u.changes, sub: t('lastChangeAgo', { ago: ago(u.last) }), href: u.user ? `/users/${seg(u.user)}` : undefined })),
          t('nobodyChanged'))),
        section(t('whereFrom'), barList(st.by_via.map((v) => ({ label: labelOf(VIA, v.via), value: v.changes })), t('noChanges')))),
      section(t('writingDevices'),
        table([t('device'), t('user'), t('kind'), t('changes'), t('lastChange')],
          st.by_device.map((d) => [d.label, h('a', { href: `/users/${seg(d.user)}/devices`, class: ui.link }, d.user), labelOf(KIND, d.kind), String(d.changes), ago(d.last)]),
          t('noWritingDevice'))),
      h('div', { class: ui.cards },
        section(t('owner'),
          h('p', { class: 'flex items-center gap-3' },
            h('span', { class: 'grid size-10 place-items-center rounded-full bg-primary-container font-bold text-on-primary-container' }, st.owner.slice(0, 1).toUpperCase()),
            h('span', { class: 'flex flex-col' }, h('a', { href: `/users/${seg(st.owner)}`, class: ui.link }, st.owner),
              h('span', { class: 'text-xs text-on-surface-variant' }, t('ownerHelp')))),
          action(t('changeOwner'), () => this.transfer(st), 'tonal', 'give')),
        section(t('deleteSection'),
          h('p', { class: 'text-sm text-on-surface-variant' }, t('deleteHelp')),
          action(t('deleteCalendar'), () => this.remove(st), 'danger', 'trash'))),
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
    const accessSelect = (target: string, current: string, label: string): HTMLSelectElement => {
      const sel = styleControl(h('select', { 'aria-label': t('accessOf', { name: label }) },
        h('option', { value: 'read', selected: current === 'read' }, t('read')),
        h('option', { value: 'readwrite', selected: current === 'readwrite' }, t('readwrite'))));
      sel.classList.add('!w-44', '!min-h-9', '!py-1');
      sel.addEventListener('change', () => void this.act(() => this.api.put(`/calendars/${seg(st.id)}/shares/${seg(target)}`, { access: sel.value }), t('accessChanged')));
      return sel;
    };
    const owner = users.find((u) => u.name === st.owner);
    const list = [
      {
        href: `/users/${seg(st.owner)}`,
        lead: avatar(owner?.display_name || st.owner, owner?.color ?? ''),
        title: owner?.display_name || st.owner,
        sub: `${st.owner} · ${t('ownerSub')}`,
        tags: [h('span', { class: ui.pill.ok }, t('ownerTag'))],
        actions: [action(t('change'), () => this.transfer(st), 'text', 'give', 'sm')],
      },
      ...st.shares.map((m) => {
        const isGroup = m.target.startsWith('g:');
        const name = m.target.slice(2);
        const g = isGroup ? groups.find((x) => x.name === name) : undefined;
        const u = isGroup ? undefined : users.find((x) => x.name === name);
        const title = isGroup ? t('groupNamed', { name: g?.display_name || name }) : u?.display_name || name;
        return {
          href: isGroup ? `/groups/${seg(name)}` : `/users/${seg(name)}`,
          lead: isGroup ? groupMark(g?.color ?? '') : avatar(u?.display_name || name, u?.color ?? ''),
          title,
          sub: isGroup ? `${name} · ${g === undefined || g.members.length === 0 ? t('noMembers') : g.members.join(', ')}` : name,
          actions: [accessSelect(m.target, m.access, title),
            action(t('remove'), () => this.act(() => this.api.delete(`/calendars/${seg(st.id)}/shares/${seg(m.target)}`), t('memberRemoved')), 'danger-text', 'close', 'sm')],
        };
      }),
    ];
    return card(t('members'), [action(t('addMember'), () => this.addMember(st, users, groups), 'filled', 'plus')],
      h('p', { class: 'text-sm text-on-surface-variant' }, t('membersHelp')),
      entries(list, ''));
  }


  private async addMember(st: CalendarStats, users: { name: string; display_name: string }[], groups: { name: string; display_name: string }[]): Promise<void> {
    const taken = new Set(st.shares.map((m) => m.target));
    const options = [
      ...groups.filter((g) => !taken.has(`g:${g.name}`)).map((g) => h('option', { value: `g:${g.name}` }, t('groupNamed', { name: g.display_name || g.name }))),
      ...users.filter((u) => u.name !== st.owner && !taken.has(`u:${u.name}`)).map((u) => h('option', { value: `u:${u.name}` }, u.display_name || u.name)),
    ];
    if (options.length === 0) {
      this.toasts.show(t('everyoneIn'));
      return;
    }
    const who = h('select', { required: true }, ...options);
    const access = h('select', {}, h('option', { value: 'read' }, t('read')), h('option', { value: 'readwrite' }, t('readwrite')));
    const ok = await formDialog(this.doc, t('addMemberTo', { name: st.name }), [field(t('member'), who), field(t('access'), access)], t('add'),
      () => this.api.put(`/calendars/${seg(st.id)}/shares/${seg(who.value)}`, { access: access.value }));
    if (!ok) return;
    this.toasts.show(t('memberAdded'));
    this.doc.dispatchEvent(new Event('agd:changed'));
    await this.refresh();
  }

  private async transfer(st: CalendarStats): Promise<void> {
    const { users } = await this.api.get<{ users: { name: string; display_name: string }[] }>('/users');
    const select = h('select', {}, ...users.filter((u) => u.name !== st.owner).map((u) => h('option', { value: u.name }, u.display_name || u.name)));
    const keep = h('input', { type: 'checkbox', checked: true });
    const v = await dialog(this.doc, t('transferTitle', { name: st.name }), [
      h('p', {}, t('transferBody')),
      field(t('newOwner'), select), h('label', { class: ui.check }, keep, t('keepOwner', { owner: st.owner }))],
    [{ label: t('cancel'), value: 'no' }, { label: t('transfer'), value: 'yes', kind: 'filled' }]);
    if (v !== 'yes' || select.value === '') return;
    try {
      const r = await this.api.post<{ ref?: string; id?: string }>(`/calendars/${seg(st.id)}/transfer`, { to: select.value, keep: keep.checked });
      this.doc.location.assign(r.id !== undefined ? `/calendars/${seg(r.id)}` : `/users/${seg(select.value)}/calendars`);
    } catch (e) {
      this.toasts.error(e);
    }
  }

  private async remove(st: CalendarStats): Promise<void> {
    const typed = h('input', { autocomplete: 'off' });
    const v = await dialog(this.doc, t('deleteTitle', { name: st.name }), [
      h('p', {}, t('deleteBody', { count: st.events })),
      field(t('typeToConfirm', { name: st.name }), typed),
    ], [{ label: t('cancel'), value: 'no' }, { label: t('delete'), value: 'yes', kind: 'danger' }]);
    if (v !== 'yes') return;
    try {
      await this.api.delete(`/calendars/${seg(st.id)}`, { confirm: typed.value });
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
  id: string;
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
    const search = h('input', { type: 'search', value: this.query, placeholder: t('searchPlaceholder'), 'aria-label': t('search'),
      class: 'min-w-0 flex-1 border-0 bg-transparent text-sm text-on-surface outline-none' });
    const list = h('div', {});
    const count = h('span', { class: 'text-xs text-on-surface-variant' });
    const draw = (): void => {
      const q = this.query.trim().toLowerCase();
      const shown = calendars.filter((c) => q === '' || [c.name, c.ref, c.owner, ...c.shares.map((s) => s.target)].some((x) => x.toLowerCase().includes(q)));
      count.textContent = t('shownOf', { shown: shown.length, total: calendars.length });
      list.replaceChildren(entries(shown.map((c) => ({
        href: `/calendars/${seg(c.id)}`,
        lead: calendarMark(c.color),
        title: c.name,
        sub: `${t('ownedBy', { owner: c.owner })} · ${c.shares.length === 0 ? t('noMembers') : c.shares.map((s) => s.target.startsWith('g:') ? t('memberGroup', { name: s.target.slice(2) }) : s.target.slice(2)).join(', ')}`,
        stats: [{ n: String(c.events), label: t('eventsStat', { count: c.events }) }, { n: bytes(c.bytes), label: t('size') }],
      })), calendars.length === 0 ? t('noCalendars') : t('noMatch')));
    };
    search.addEventListener('input', () => {
      this.query = search.value;
      draw();
    });
    draw();
    return [card(t('calendars'), [h('label', { class: 'flex h-9 min-w-56 items-center gap-2 rounded-md3-full bg-surface-lowest px-3 text-on-surface-variant' }, icon('search'), search), count],
      h('p', { class: 'text-sm text-on-surface-variant' }, t('adminNoEvents')),
      list)];
  }
}
