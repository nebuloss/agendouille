import { avatar, calendarMark, entries, groupMark } from '@/admin/entries.ts';
import { ui } from '@/shared/components/recipes.ts';
import { icon } from '@/shared/components/icons.ts';
import { ActivityLog } from '@/admin/ActivityLog.ts';
import { ApiError, seg } from '@/shared/core/Api.ts';
import { h, type Child } from '@/shared/core/Dom.ts';
import { action, card, confirmDialog, field, formDialog, section, swatch, table } from '@/shared/components/Ui.ts';
import { bytes, when } from '@/shared/render/format.ts';
import { AdminPage } from '@/admin/pages/base.ts';
import { t } from '@/admin/pages/More.msg.ts';
import { pick } from '@/shared/components/Picker.ts';

type Key = Parameters<typeof t>[0];

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
      card(t('groups'), [action(t('newGroup'), () => this.create(), 'filled', 'plus')],
        h('p', { class: 'text-sm text-on-surface-variant' }, t('groupsIntro')),
        entries(groups.map((g) => ({
          href: `/groups/${seg(g.name)}`,
          lead: groupMark(g.color),
          title: g.display_name || g.name,
          sub: `${g.name} · ${g.members.length === 0 ? t('noMembers') : g.members.map((m) => users.find((u) => u.name === m)?.display_name || m).join(', ')}`,
          stats: [{ n: String(g.members.length), label: t('statMembers', { count: g.members.length }) }, { n: String(g.shares), label: t('statShares', { count: g.shares }) }],
        })), t('noGroups'))),
    ];
  }

  /** "Nouveau groupe": the form in a modal. */
  private async create(): Promise<void> {
    const name = h('input', { required: true, pattern: '[a-z][a-z0-9-]{0,31}', placeholder: t('idPlaceholder'), autocomplete: 'off' });
    const display = h('input', { placeholder: t('displayPlaceholder') });
    const ok = await formDialog(this.doc, t('newGroup'), [
      field(t('identifier'), name, t('identifierHelp')),
      field(t('displayName'), display),
    ], t('createGroup'), async () => {
      await this.api.post('/groups', { name: name.value.trim(), display_name: display.value.trim() });
    });
    if (!ok) return;
    this.toasts.show(t('groupCreated'));
    this.doc.dispatchEvent(new Event('agd:changed'));
    await this.refresh();
  }


}

interface CalendarShared {
  id: string;
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
    if (g === undefined) return [h('p', { class: ui.error, role: 'alert' }, t('groupNotFound', { name: this.name }))];
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
      h('div', { class: 'grid gap-4 [grid-template-columns:repeat(auto-fill,minmax(min(220px,100%),1fr))]' }, field(t('displayName'), display), field(t('colour'), color)),
      h('button', { class: ui.btn('tonal'), type: 'submit' }, t('save')));
    form.addEventListener('submit', (ev) => {
      ev.preventDefault();
      void this.act(() => this.api.patch(this.path(), { display_name: display.value.trim(), color: color.value }), t('saved'));
    });
    return [
      h('section', { class: `${ui.card} flex-row items-center gap-4` },
        swatch(g.color, 'md'),
        h('div', { class: 'flex min-w-0 flex-1 flex-col' }, h('h2', { class: 'truncate text-2xl font-normal' }, g.display_name || g.name),
          h('span', { class: 'text-sm text-on-surface-variant' }, t('groupIdLine', { name: g.name })))),
      h('div', { class: 'grid gap-3 [grid-template-columns:repeat(auto-fill,minmax(150px,1fr))]' },
        tileLink(String(g.members.length), t('tileMembers', { count: g.members.length }), `${this.path()}/members`),
        tileLink(String(g.shares), t('tileShares', { count: g.shares }), `${this.path()}/calendars`)),
      this.members(g, users),
      section(t('profile'), form),
      section(t('deleteGroup'),
        h('p', { class: 'text-sm text-on-surface-variant' }, t('deleteConsequence', { count: g.shares })),
        action(t('deleteEllipsis'), () => this.remove(g), 'danger', 'trash')),
    ];
  }

  private members(g: Group, users: UserLite[]): HTMLElement {
    const set = (members: string[], done: string): Promise<void> => this.act(() => this.api.patch(this.path(), { members }), done);
    const byName = new Map(users.map((u) => [u.name, u]));
    return card(t('members'), [action(t('addMember'), () => this.addMember(g, users), 'filled', 'plus')],
      h('p', { class: 'text-sm text-on-surface-variant' }, t('membersIntro')),
      entries(g.members.map((m) => {
        const u = byName.get(m);
        return {
          href: `/users/${seg(m)}`,
          lead: avatar(u?.display_name || m, u?.color ?? ''),
          title: u?.display_name || m,
          sub: m,
          actions: [action(t('remove'), () => set(g.members.filter((x) => x !== m), t('removedFromGroup')), 'danger-text', 'close', 'sm')],
        };
      }), t('noMembersYet')));
  }

  private async addMember(g: Group, users: UserLite[]): Promise<void> {
    const others = users.filter((u) => !g.members.includes(u.name));
    if (others.length === 0) {
      this.toasts.show(t('everyoneIsMember'));
      return;
    }
    const picked = await pick(this.doc, {
      title: t('addMemberTo', { name: g.display_name || g.name }), multiple: true, confirm: t('addToGroup'),
      items: others.map((u) => ({ value: u.name, label: u.display_name || u.name, sub: u.name, lead: avatar(u.display_name || u.name, u.color) })),
    });
    if (picked === null || picked.length === 0) return;
    await this.act(() => this.api.patch(this.path(), { members: [...g.members, ...picked] }), t('addedToGroupN', { count: picked.length }));
  }



  private async calendars(g: Group): Promise<HTMLElement> {
    const { calendars } = await this.api.get<{ calendars: CalendarShared[] }>('/calendars');
    const target = `g:${g.name}`;
    const shared = calendars.filter((c) => c.shares.some((s) => s.target === target));
    return section(t('sharedWithGroup'),
      h('p', { class: 'text-sm text-on-surface-variant' }, t('adminSeesCalendars')),
      entries(shared.map((c) => {
        const s = c.shares.find((x) => x.target === target)!;
        return {
          href: `/calendars/${seg(c.id)}`,
          lead: calendarMark(c.color),
          title: c.name,
          sub: t('ownerEvents', { owner: c.owner, count: c.events }),
          tags: [h('span', { class: s.access === 'read' ? ui.pill.muted : ui.pill.ok }, accessLabel(s.access))],
          actions: [action(t('remove'), () => this.act(() => this.api.delete(`/calendars/${seg(c.id)}/shares/${seg(target)}`), t('shareRemoved')), 'danger-text', 'close', 'sm')],
        };
      }), t('noSharedCalendars')));
  }


  private async remove(g: Group): Promise<void> {
    const text = `${t('deleteConsequence', { count: g.shares })} ${t('irreversible')}`;
    if (!(await confirmDialog(this.doc, t('deleteConfirm', { name: g.display_name || g.name }), text, t('deleteGroup'), true))) return;
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
    h('span', { class: 'text-[32px] leading-10 font-semibold text-primary' }, n), h('span', { class: 'text-sm text-on-surface-variant' }, label));
}

const ACCESS: Record<string, Key> = { read: 'accessRead', readwrite: 'accessReadwrite', owner: 'accessOwner' };

export const accessLabel = (a: string): string => {
  const k = ACCESS[a];
  return k === undefined ? a : t(k);
};

/** Who did what, never what an event said: claudication's request log, for the activity (ActivityLog). */
export class ActivityPage extends AdminPage {
  private log: ActivityLog | null = null;

  protected async render(): Promise<Child[]> {
    this.log ??= new ActivityLog(this.doc, this.api);
    await this.log.load();
    return [card(t('activity'), [],
      h('p', { class: 'text-sm text-on-surface-variant' }, t('activityIntro')),
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
  /** In the reader's language, from the server's catalog. */
  help?: string;
}

const GROUP_NAMES: Record<string, Key> = { network: 'groupNetwork', storage: 'groupStorage', limits: 'groupLimits', defaults: 'groupDefaults' };

function groupName(g: string): string {
  const k = GROUP_NAMES[g];
  return k === undefined ? g : t(k);
}

const ORIGINS: Record<string, { label: Key; cls: string }> = {
  file: { label: 'originFile', cls: ui.pill.ok },
  env: { label: 'originEnv', cls: ui.pill.warn },
  flag: { label: 'originFlag', cls: ui.pill.warn },
  default: { label: 'originDefault', cls: ui.pill.muted },
};

const shown = (v: unknown): string => (Array.isArray(v) ? (v.length === 0 ? '—' : v.join(', ')) : v === '' || v === null || v === undefined ? '—' : String(v));

/**
 * The settings, as cafard and claudication show theirs: read from the
 * configuration file (and the environment), each with where its value
 * came from. Edited in the file; the server applies an edit within
 * seconds (DECISIONS: settings in the configuration file).
 */
export class SettingsPage extends AdminPage {
  protected async render(): Promise<Child[]> {
    const { settings, path, exists } = await this.api.get<{ settings: Setting[]; path: string; exists: boolean }>('/settings');
    const groups = new Map<string, Setting[]>();
    for (const st of settings) groups.set(st.group, [...(groups.get(st.group) ?? []), st]);
    const how = card(t('configFile'), [h('code', { class: 'rounded-md3-s bg-surface-lowest px-2 py-1 font-mono text-[13px]' }, path)],
      h('p', { class: 'text-sm text-on-surface-variant' }, exists ? t('fileExists') : t('fileMissing')),
      h('pre', { class: 'overflow-x-auto rounded-md3-m bg-surface-lowest px-4 py-3 font-mono text-[12.5px] leading-relaxed text-on-surface' },
        t('fileExample', { path })),
      h('p', { class: 'text-xs text-on-surface-variant' }, t('envNote')));
    const cards = ['network', 'storage', 'limits', 'defaults'].map((g) => section(groupName(g),
      h('dl', { class: 'm-0 flex flex-col divide-y divide-outline-variant/50 rounded-md3-m bg-surface-lowest' }, ...(groups.get(g) ?? []).map((st) => {
        const o = ORIGINS[st.origin] ?? ORIGINS['default']!;
        const changed = JSON.stringify(st.value) !== JSON.stringify(st.default);
        return h('div', { class: 'flex flex-col gap-1 px-4 py-3' },
          h('dt', { class: 'flex flex-wrap items-center gap-2' },
            h('code', { class: 'font-mono text-[13px] font-semibold' }, st.key),
            h('span', { class: o.cls, title: st.origin === 'env' ? st.env : undefined }, t(o.label))),
          h('dd', { class: 'm-0 flex flex-col gap-0.5' },
            h('span', { class: 'font-mono text-[13px] break-all text-on-surface' }, shown(st.value)),
            st.help !== undefined && st.help !== '' ? h('span', { class: 'text-xs text-on-surface-variant' }, st.help) : null,
            changed ? h('span', { class: 'text-xs text-on-surface-variant' }, t('defaultValue', { value: shown(st.default), env: st.env }))
              : h('span', { class: 'text-xs text-on-surface-variant' }, st.env)));
      }))));
    return [how, h('div', { class: ui.cards }, ...cards)];
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
    const { calendars } = await this.api.get<{ calendars: { id: string; ref: string; name: string; owner: string; color: string; events: number }[] }>('/calendars');
    const icsCard = card(t('icsTitle'), [
      h('a', { class: ui.btn('tonal'), href: this.api.url('/calendars/export.zip'), download: '' }, icon('download'), t('downloadAllZip')),
    ],
    h('p', { class: 'text-sm text-on-surface-variant' }, t('icsIntro')),
    calendars.length === 0 ? h('p', { class: ui.empty }, t('noCalendars'))
      : h('div', { class: ui.well }, ...calendars.map((c) => h('div', { class: 'flex min-h-14 items-center gap-3 border-b border-outline-variant/50 px-4 py-2.5 last:border-0' },
        swatch(c.color, 'md'),
        h('span', { class: 'flex min-w-0 flex-1 flex-col' }, h('a', { href: `/calendars/${seg(c.id)}`, class: 'truncate font-semibold text-on-surface no-underline hover:underline' }, c.name),
          h('span', { class: 'text-xs text-on-surface-variant' }, t('ownerEvents', { owner: c.owner, count: c.events }))),
        h('a', { class: ui.btn('text', 'sm'), href: this.api.url(`/calendars/${seg(c.id)}/export`), download: `${c.ref}.ics` }, icon('download'), '.ics')))));
    return [
      card(t('database'), [action(t('backUpNow'), () => this.act(() => this.api.post('/backups'), t('backupDone')), 'filled', 'disk')],
      h('p', { class: 'text-sm text-on-surface-variant' }, t('databaseIntro')),
      table([t('colBackup'), t('colMade'), t('colSize'), ''],
        backups.map((b) => [
          h('span', { class: 'font-mono text-[13px]' }, b.name, b.before_restore ? h('span', { class: 'mr-1 inline-flex h-6 items-center gap-1 rounded-md3-s bg-surface-high px-2 text-xs font-semibold text-on-surface' }, t('beforeRestore')) : null),
          when(b.made),
          bytes(b.size),
          h('span', { class: 'flex flex-wrap items-center gap-2' },
            h('a', { class: ui.btn('text'), href: this.api.url(`/backups/${seg(b.name)}`), download: b.name }, t('download')),
            action(t('restoreEllipsis'), () => this.restore(b), 'text')),
        ]), t('noBackups'))),
      icsCard,
    ];
  }

  private async restore(b: Backup): Promise<void> {
    const ok = await confirmDialog(this.doc, t('restoreConfirm'), t('restoreConsequence', { date: when(b.made) }), t('restore'), true);
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
