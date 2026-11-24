import { seg, type Api } from '@/shared/core/Api.ts';
import { h, svgDataUrl, type Child } from '@/shared/core/Dom.ts';
import { icon } from '@/shared/components/icons.ts';
import { ui } from '@/shared/components/recipes.ts';
import { ApiPage } from '@/shared/components/ApiPage.ts';
import {
  action, card, check, chip, confirmDialog, dialog, field, langChoice, secretField, section, segmented, styleControl, swatch, table, themeToggle,
} from '@/shared/components/Ui.ts';
import { ago } from '@/shared/render/format.ts';
import { pick, type PickItem } from '@/shared/components/Picker.ts';
import { recencyTone } from '@/shared/components/tone.ts';
import { appApi, type Calendar, type Calendars, type Me, type Person, type Share } from '@/app/api.ts';
import { t } from '@/app/pages/Manage.msg.ts';

/** A page of the family web app that renders from the API into #app-content. */
abstract class AppPage extends ApiPage {
  constructor(doc: Document, api: Api = appApi()) {
    super(doc, api, 'app-content');
  }

  /** Asks first (danger style), then runs a deletion and says it is done. */
  protected async confirmed(title: string, text: string, button: string, run: () => Promise<unknown>, done: string): Promise<void> {
    if (!(await confirmDialog(this.doc, title, text, button, true))) return;
    await this.act(run, done);
  }
}

const accessLabel = (access: string): string =>
  access === 'owner' ? t('accessOwner') : access === 'readwrite' ? t('accessReadwrite') : access === 'read' ? t('accessRead') : access;
const COLORS = ['#1e88e5', '#43a047', '#e53935', '#fb8c00', '#8e24aa', '#00897b', '#6d4c41', '#3949ab'];

export interface People {
  users: Person[];
  groups: Person[];
}

/** Calendriers (PLAN.md 9.3): mine, shared with me; create, share, hide. */
export class CalendarsPage extends AppPage {
  /** Export and import (.ics files): the Android app has no use for them. */
  protected readonly files: boolean = true;

  protected async render(): Promise<Child[]> {
    const [cals, people] = await Promise.all([this.api.get<Calendars>('/calendars'), this.api.get<People>('/people')]);
    const me = this.doc.querySelector<HTMLElement>('.app')?.dataset['user'] ?? '';
    const mine = cals.calendars.filter((c) => c.owner === me);
    const others = cals.calendars.filter((c) => c.owner !== me);
    const newBtn = action(t('createCalendar'), () => this.create(), 'filled', 'plus');
    return [
      card(t('myCalendars'), [newBtn],
        mine.length === 0 ? h('p', { class: ui.empty }, t('noOwnCalendar')) : null,
        ...mine.map((c) => this.ownedCard(c, people))),
      section(t('sharedWithMe'),
        others.length === 0 ? h('p', { class: ui.empty }, t('noSharedCalendar')) : null,
        ...others.map((c) => this.sharedCard(c))),
    ];
  }

  protected nameOf(people: People, target: string): string {
    const [kind, name] = [target.slice(0, 1), target.slice(2)];
    const list = kind === 'g' ? people.groups : people.users;
    const p = list.find((x) => x.name === name);
    const shown = p?.display_name || name;
    return kind === 'g' ? t('groupInline', { name: shown }) : shown;
  }

  protected ownedCard(c: Calendar, people: People): HTMLElement {
    const shares = c.shares ?? [];
    const path = `/calendars/${seg(c.ref)}`;
    // Each share: who, a button to take it off, and its access, changed in place.
    const shareRows = shares.map((s: Share) => {
      const who = this.nameOf(people, s.target);
      const access = segmented(this.doc, t('access'), [{ value: 'read', label: t('accessReadShort') }, { value: 'readwrite', label: t('accessReadwriteShort') }],
        s.access === 'readwrite' ? 'readwrite' : 'read',
        (v) => void this.act(() => this.api.put(`${path}/shares/${seg(s.target)}`, { access: v }), t('accessChanged', { name: who })));
      access.el.classList.add('self-start');
      const off = h('button', { type: 'button', class: 'state-layer grid size-9 shrink-0 place-items-center rounded-full text-on-surface-variant',
        title: t('removeShare'), 'aria-label': `${t('removeShare')} ${who}` }, icon('close'));
      off.addEventListener('click', () => void this.confirmed(t('removeShareTitle', { calendar: c.name, name: who }), t('removeShareText', { name: who }),
        t('removeShareButton'), () => this.api.delete(`${path}/shares/${seg(s.target)}`), t('shareRemoved')));
      return h('div', { class: 'flex flex-col gap-2 px-3 py-3' },
        h('div', { class: 'flex items-center gap-3' }, this.mark(s.target.startsWith('g:') ? 'group' : 'person'),
          h('span', { class: 'min-w-0 flex-1 truncate font-medium' }, who), off),
        access.el);
    });
    return h('div', { class: 'flex flex-col gap-3 rounded-md3-l bg-surface-lowest p-4' },
      h('div', { class: 'flex flex-wrap items-center gap-3' }, swatch(c.color, 'md'),
        h('div', { class: 'flex min-w-0 flex-1 flex-col gap-1' }, h('strong', { class: 'truncate text-base' }, c.name),
          h('span', { class: 'flex flex-wrap items-center gap-1.5' },
            chip(t('sharedCount', { count: shares.length }), shares.length > 0 ? 'accent' : 'neutral', shares.length > 0 ? 'share' : 'lock', this.doc),
            c.hidden ? chip(t('hidden'), 'warn', 'hide', this.doc) : null))),
      c.description !== '' ? h('p', { class: 'text-sm text-on-surface-variant' }, c.description) : null,
      shares.length > 0
        ? h('div', { class: 'flex flex-col gap-1' }, h('span', { class: 'text-xs font-semibold text-on-surface-variant' }, t('sharedWith')),
          h('div', { class: 'flex flex-col divide-y divide-outline-variant rounded-md3-l border border-outline-variant' }, ...shareRows))
        : null,
      h('div', { class: 'flex flex-wrap items-center gap-1' },
        action(t('share'), () => this.share(c, people), 'tonal', 'share', 'sm'),
        action(t('edit'), () => this.editProps(c), 'text', 'edit', 'sm'),
        this.files ? h('a', { class: ui.btn('text', 'sm'), href: this.api.url(`${path}/export`), download: `${c.ref}.ics` }, icon('download'), t('export')) : null,
        this.files ? action(t('import'), () => this.importInto(c), 'text', 'upload', 'sm') : null,
        action(t('give'), () => this.transfer(c, people), 'text', 'give', 'sm'),
        h('span', { class: 'ml-auto' }),
        action(t('delete'), () => this.remove(c), 'danger-text', 'trash', 'sm')));
  }

  protected sharedCard(c: Calendar): HTMLElement {
    const path = `/calendars/${seg(c.ref)}`;
    return h('div', { class: 'flex flex-col gap-3 rounded-md3-l bg-surface-lowest p-4' },
      h('div', { class: 'flex flex-wrap items-center gap-3' }, swatch(c.color, 'md'),
        h('div', { class: 'flex min-w-0 flex-1 flex-col' }, h('strong', { class: 'truncate text-base' }, c.name),
          h('span', { class: 'text-xs text-on-surface-variant' }, t('ownerIs', { name: c.owner }))),
        chip(accessLabel(c.access), c.access === 'read' ? 'neutral' : 'accent', c.access === 'read' ? 'show' : 'edit', this.doc)),
      h('div', { class: 'flex flex-wrap items-center gap-1' },
        c.hidden
          ? action(t('showAgain'), () => this.act(() => this.api.patch(path, { hidden: false }), t('shownAgain')), 'tonal', 'show', 'sm')
          : action(t('hide'), () => this.act(() => this.api.patch(path, { hidden: true }), t('hiddenDone')), 'text', 'hide', 'sm'),
        action(t('customise'), () => this.editProps(c), 'text', 'palette', 'sm')));
  }

  private colorPicker(current: string): HTMLInputElement {
    return h('input', { type: 'color', value: /^#[0-9a-f]{6}$/i.test(current) ? current : COLORS[0]! });
  }

  protected async create(): Promise<void> {
    const name = h('input', { required: true, placeholder: t('namePlaceholder') });
    const color = this.colorPicker(COLORS[Math.floor(Math.random() * COLORS.length)]!);
    const v = await dialog(this.doc, t('newCalendar'), [field(t('name'), name), field(t('color'), color)],
      [{ label: t('cancel'), value: '' }, { label: t('createCalendarButton'), value: 'ok', kind: 'filled' }]);
    if (v !== 'ok' || name.value.trim() === '') return;
    await this.act(() => this.api.post('/calendars', { name: name.value.trim(), color: color.value }), t('calendarCreated'));
  }

  protected async editProps(c: Calendar): Promise<void> {
    const owner = c.access === 'owner';
    const name = h('input', { value: c.name, required: true });
    const color = this.colorPicker(c.color);
    const desc = h('textarea', { rows: 2 });
    desc.value = c.description;
    const v = await dialog(this.doc, owner ? t('editCalendar') : t('customiseCalendar'), [
      field(owner ? t('name') : t('nameForMe'), name), field(t('color'), color), owner ? field(t('description'), desc) : null,
    ], [{ label: t('cancel'), value: '' }, { label: t('save'), value: 'ok', kind: 'filled' }]);
    if (v !== 'ok') return;
    const body: Record<string, string> = { name: name.value.trim(), color: color.value };
    if (owner) body['description'] = desc.value;
    await this.act(() => this.api.patch(`/calendars/${seg(c.ref)}`, body), t('saved'));
  }

  /** The people and groups to choose from, groups first. */
  private shareItems(people: People): PickItem[] {
    return [
      ...people.groups.map((g) => ({ value: `g:${g.name}`, label: g.display_name || g.name, sub: t('groupSub'), section: t('groupsSection'), lead: this.mark('group') })),
      ...people.users.map((u) => ({ value: `u:${u.name}`, label: u.display_name || u.name, sub: u.name, section: t('peopleSection'), lead: this.mark('person') })),
    ];
  }

  protected mark(name: 'group' | 'person'): HTMLElement {
    return h('span', { class: 'grid size-9 shrink-0 place-items-center rounded-full bg-secondary-container text-on-secondary-container' }, icon(name));
  }

  /**
   * Sharing, in one picker: the groups and the people, those it is shared
   * with already ticked. Ticking adds (with the access chosen under the
   * list), unticking takes off, after a confirmation.
   */
  protected async share(c: Calendar, people: People): Promise<void> {
    const items = this.shareItems(people);
    if (items.length === 0) {
      this.toasts.show(t('nobodyToShare'), 'error');
      return;
    }
    const current = new Map((c.shares ?? []).map((s) => [s.target, s.access]));
    const access = segmented(this.doc, t('access'), [{ value: 'read', label: t('accessReadShort') }, { value: 'readwrite', label: t('accessReadwriteShort') }], 'read');
    const picked = await pick(this.doc, {
      title: t('shareTitle', { name: c.name }), items, multiple: true, allowNone: true,
      selected: [...current.keys()], confirm: t('shareSave'),
      extra: [h('div', { class: 'flex flex-col gap-2' }, h('span', { class: 'text-xs font-semibold text-on-surface-variant' }, t('accessForAdded')), access.el)],
    });
    if (picked === null) return;
    const added = picked.filter((v) => !current.has(v));
    const removed = [...current.keys()].filter((v) => !picked.includes(v));
    if (added.length === 0 && removed.length === 0) return;
    if (removed.length > 0) {
      const names = removed.map((v) => this.nameOf(people, v)).join(', ');
      if (!(await confirmDialog(this.doc, t('unshareTitle'), t('unshareText', { calendar: c.name, names }), t('unshareButton'), true))) return;
    }
    const path = `/calendars/${seg(c.ref)}/shares`;
    await this.act(async () => {
      for (const v of added) await this.api.put(`${path}/${seg(v)}`, { access: access.value });
      for (const v of removed) await this.api.delete(`${path}/${seg(v)}`);
    }, t('sharesSaved'));
  }

  /** Giving it away: one person, chosen in the picker. */
  protected async transfer(c: Calendar, people: People): Promise<void> {
    if (people.users.length === 0) return;
    const picked = await pick(this.doc, {
      title: t('giveTitle', { name: c.name }),
      items: people.users.map((u) => ({ value: u.name, label: u.display_name || u.name, sub: u.name, lead: this.mark('person') })),
      extra: [h('p', { class: 'text-sm text-on-surface-variant' }, t('giveText'))],
    });
    if (picked === null || picked.length === 0) return;
    const to = people.users.find((u) => u.name === picked[0]);
    if (!(await confirmDialog(this.doc, t('giveTitle', { name: c.name }), t('giveConfirm', { calendar: c.name, name: to?.display_name || picked[0]! }), t('giveButton'), false))) return;
    await this.act(() => this.api.post(`/calendars/${seg(c.ref)}/transfer`, { to: picked[0] }), t('given'));
  }

  private async importInto(c: Calendar): Promise<void> {
    const file = h('input', { type: 'file', accept: '.ics,text/calendar' });
    const replace = h('input', { type: 'checkbox' });
    const v = await dialog(this.doc, t('importTitle', { name: c.name }), [
      styleControl(file), check(replace, t('importReplace')),
    ], [{ label: t('cancel'), value: '' }, { label: t('importButton'), value: 'ok', kind: 'filled' }]);
    const f = file.files?.[0];
    if (v !== 'ok' || f === undefined) return;
    // Replacing overwrites events already there: ask first.
    if (replace.checked && !(await confirmDialog(this.doc, t('importReplaceTitle', { name: c.name }), t('importReplaceText'), t('importReplaceButton'), true))) return;
    try {
      const r = await this.api.upload<{ added: number; replaced: number; skipped: number; errors: string[] }>(
        `/calendars/${seg(c.ref)}/import`, f, { replace: replace.checked ? '1' : undefined });
      this.toasts.show(t('imported', { added: r.added, replaced: r.replaced, skipped: r.skipped }));
      await this.refresh();
    } catch (e) {
      this.toasts.error(e);
    }
  }

  protected async remove(c: Calendar): Promise<void> {
    await this.confirmed(t('deleteCalendarTitle', { name: c.name }), t('deleteCalendarText'), t('deleteCalendarButton'),
      () => this.api.delete(`/calendars/${seg(c.ref)}`), t('calendarDeleted'));
  }
}

export interface Device {
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

export interface Session {
  id: number;
  user_agent: string;
  app: boolean;
  seen_at: string;
  current: boolean;
}

/** A kind of device, as the person reads it (product names stay as they are). */
export function kindLabel(kind: string): string {
  switch (kind) {
    case 'davx5':
      return 'DAVx5 (Android)';
    case 'ios':
      return 'iPhone, iPad, Mac';
    case 'thunderbird':
      return 'Thunderbird';
    case 'other':
      return t('kindOther');
    case 'app':
      return t('kindApp');
    default:
      return kind;
  }
}

/** Appareils (PLAN.md 9.3, 8.4): phones and computers that sync, links, browsers. */
export class DevicesPage extends AppPage {
  protected async render(): Promise<Child[]> {
    const [devices, links, sessions, cals] = await Promise.all([
      this.api.get<{ devices: Device[] }>('/devices'), this.api.get<{ links: Link[] }>('/links'),
      this.api.get<{ sessions: Session[] }>('/sessions'), this.api.get<Calendars>('/calendars'),
    ]);
    const name = (ref: string): string => cals.calendars.find((c) => c.ref === ref)?.name ?? ref;
    return [
      section(t('syncDevice'),
        h('p', { class: 'text-sm text-on-surface-variant' }, t('modePassword')),
        h('div', { class: 'grid grid-cols-2 gap-2 sm:flex sm:flex-wrap' },
          action('Android (DAVx5)', () => this.add('davx5'), 'filled', 'phone'),
          action(kindLabel('ios'), () => this.add('ios'), 'tonal', 'phone'),
          action(kindLabel('thunderbird'), () => this.add('thunderbird'), 'tonal', 'laptop'),
          action(kindLabel('other'), () => this.add('other'), 'outlined', 'devices')),
        table([t('device'), t('access'), t('lastSync'), ''], devices.devices.map((d) => [
          h('span', { class: 'flex flex-col' }, h('strong', {}, d.label), h('span', { class: 'text-xs text-on-surface-variant' }, kindLabel(d.kind))),
          chip(d.cap === 'read' ? t('accessRead') : t('accessReadwrite'), d.cap === 'read' ? 'neutral' : 'accent', undefined, this.doc),
          chip(ago(d.last_used_at), recencyTone(d.last_used_at), 'clock', this.doc),
          h('span', { class: 'flex flex-wrap justify-end gap-1' },
            action(t('removeDevice'), () => this.confirmed(t('removeDeviceTitle', { name: d.label }), t('removeDeviceText'), t('removeDeviceButton'),
              () => this.api.delete(`/devices/${d.id}`), t('deviceRemoved')), 'danger-text', 'trash', 'sm')),
        ]), t('noDevice'))),
      card(t('subscriptionLinks'), [action(t('createLink'), () => this.addLink(cals.calendars), 'tonal', 'link', 'sm')],
        h('p', { class: 'text-sm text-on-surface-variant' }, t('linksIntro')),
        table([t('link'), t('calendars'), t('lastRead'), ''], links.links.map((l) => [
          l.label, l.calendars.map(name).join(', '), ago(l.last_used_at),
          action(t('removeLink'), () => this.confirmed(t('removeLinkTitle', { name: l.label }), t('removeLinkText'), t('removeLinkButton'),
            () => this.api.delete(`/links/${l.id}`), t('linkRemoved')), 'danger-text', 'trash', 'sm'),
        ]), t('noLink'))),
      card(t('browsers'),
        // Every other browser at once (this one, and the Android app, stay).
        sessions.sessions.filter((s) => !s.current && !s.app).length > 1
          ? [action(t('signOutOthers'), () => this.confirmed(t('signOutOthersTitle'), t('signOutOthersText'), t('signOutOthersButton'),
            () => this.api.delete('/sessions'), t('othersSignedOut')), 'danger-text', 'logout', 'sm')]
          : [],
        table([t('browser'), t('lastSeen'), ''], sessions.sessions.map((s) => {
          const label = s.app ? t('kindApp') : browserName(s.user_agent);
          return [
            h('span', { class: 'inline-flex flex-wrap items-center gap-2' }, label, s.current ? chip(t('thisBrowser'), 'accent', 'laptop', this.doc) : null),
            chip(ago(s.seen_at), recencyTone(s.seen_at), 'clock', this.doc),
            s.current ? null : action(t('signOutBrowser'), () => this.confirmed(t('signOutBrowserTitle', { name: label }), t('signOutBrowserText'),
              t('signOutBrowserButton'), () => this.api.delete(`/sessions/${s.id}`), t('browserSignedOut')), 'danger-text', 'logout', 'sm'),
          ];
        }))),
    ];
  }

  private async add(kind: string): Promise<void> {
    const label = h('input', { required: true, value: kind === 'ios' ? 'iPhone' : kind === 'davx5' ? t('defaultPhone') : t('defaultComputer') });
    const cap = segmented(this.doc, t('access'), [{ value: 'readwrite', label: t('accessReadwriteShort') }, { value: 'read', label: t('accessReadShort') }], 'readwrite');
    const v = await dialog(this.doc, t('addDeviceTitle', { kind: kindLabel(kind) }), [field(t('name'), label),
      h('div', { class: 'flex flex-col gap-1.5' }, h('span', { class: 'text-xs font-semibold text-on-surface-variant' }, t('access')), cap.el)],
      [{ label: t('cancel'), value: '' }, { label: t('addDeviceButton'), value: 'ok', kind: 'filled' }]);
    if (v !== 'ok') return;
    let d: NewDevice;
    try {
      d = await this.api.post<NewDevice>('/devices', { kind, label: label.value.trim(), cap: cap.value });
    } catch (e) {
      this.toasts.error(e);
      return;
    }
    const manual = h('dl', { class: ui.kv },
      h('dt', {}, t('server')), h('dd', {}, h('code', {}, d.manual.server)),
      h('dt', {}, t('username')), h('dd', {}, h('code', {}, d.username)),
      h('dt', {}, t('password')), h('dd', {}, secretField(d.secret, t('devicePassword'))));
    const body: Child[] = [];
    if (d.davx5_uri !== undefined) {
      body.push(h('p', {}, t('davx5Intro')));
      if (d.qr_svg !== undefined) body.push(h('img', { class: 'qr mx-auto rounded-md3-l p-2', src: svgDataUrl(d.qr_svg), alt: t('davx5QrAlt'), width: 240, height: 240 }));
      body.push(h('p', { class: 'flex justify-center' }, h('a', { href: d.davx5_uri, class: ui.btn('tonal') }, icon('phone'), t('openInDavx5'))));
    }
    if (d.mobileconfig_url !== undefined) {
      body.push(h('p', {}, t('iosIntro')),
        h('p', {}, h('a', { class: ui.btn('filled'), href: d.mobileconfig_url }, t('installProfile'))));
    }
    body.push(h('p', { class: 'text-sm' }, t('byHand')), manual,
      h('p', { class: ui.notice }, icon('warn'), t('passwordOnceRemove')));
    await dialog(this.doc, t('deviceAdded', { name: d.label }), body, [{ label: t('done'), value: 'ok', kind: 'filled' }]);
    await this.refresh();
  }

  private async addLink(cals: Calendar[]): Promise<void> {
    const label = h('input', { required: true, value: t('defaultLinkName') });
    const boxes = cals.map((c) => ({ c, box: h('input', { type: 'checkbox', checked: c.shown }) }));
    const v = await dialog(this.doc, t('newLinkTitle'), [field(t('name'), label),
      h('div', { class: 'flex flex-col gap-3' }, ...boxes.map(({ c, box }) => check(box, swatch(c.color), h('span', { class: 'text-on-surface' }, c.name))))],
    [{ label: t('cancel'), value: '' }, { label: t('createLinkButton'), value: 'ok', kind: 'filled' }]);
    const chosen = boxes.filter((b) => b.box.checked).map((b) => b.c.ref);
    if (v !== 'ok' || chosen.length === 0) return;
    try {
      const l = await this.api.post<Link>('/links', { label: label.value.trim(), calendars: chosen });
      await dialog(this.doc, t('linkCreated'), [h('p', {}, t('copyNow')), secretField(l.url ?? '', t('linkAddress'))],
        [{ label: t('done'), value: 'ok', kind: 'filled' }]);
      await this.refresh();
    } catch (e) {
      this.toasts.error(e);
    }
  }
}

/** A browser's name from its user agent, roughly. */
export function browserName(ua: string): string {
  const os = /Android/.test(ua) ? 'Android' : /iPhone|iPad/.test(ua) ? 'iOS' : /Mac OS X/.test(ua) ? 'Mac' : /Windows/.test(ua) ? 'Windows' : /Linux/.test(ua) ? 'Linux' : '';
  const b = /Edg\//.test(ua) ? 'Edge' : /Firefox\//.test(ua) ? 'Firefox' : /Chrome\//.test(ua) ? 'Chrome' : /Safari\//.test(ua) ? 'Safari' : t('browser');
  return os === '' ? b : `${b}, ${os}`;
}

/** Hidden from the sm breakpoint up, where the shell's header shows the same. */
function phoneOnly(el: HTMLElement): HTMLElement {
  el.classList.add('sm:hidden');
  return el;
}

/** Profil (PLAN.md 9.3): my name and colour, my groups, enroll another phone. */
export class MePage extends AppPage {
  protected async render(): Promise<Child[]> {
    const me = await this.api.get<Me>('/me');
    const name = h('input', { value: me.display_name, required: true });
    const color = h('input', { type: 'color', value: /^#[0-9a-f]{6}$/i.test(me.color) ? me.color : '#1e88e5' });
    const language = langChoice(this.doc);
    language.classList.add('self-start'); // as wide as its choices, not the field
    return [
      section(t('profile'),
        field(t('displayName'), name), field(t('color'), color),
        h('div', { class: 'flex justify-end' }, action(t('save'), () => this.act(() => this.api.patch('/me', { display_name: name.value.trim(), color: color.value }), t('saved')), 'filled'))),
      // On a wider screen the header holds them: here only on a phone.
      phoneOnly(section(t('appearance'),
        h('p', { class: 'text-sm text-on-surface-variant' }, t('appearanceHint')),
        h('div', {}, themeToggle(this.doc)),
        // Not a <label>: a tap on the word would press the first language.
        h('div', { class: ui.field }, h('span', { class: ui.label }, t('language')), language))),
      section(t('myGroups'),
        me.groups.length === 0 ? h('p', { class: ui.empty }, t('noGroup'))
          : h('div', { class: 'flex flex-wrap gap-2' }, ...me.groups.map((g) => chip(g.display_name || g.name, 'accent', 'group', this.doc))),
        h('p', { class: 'text-sm text-on-surface-variant' }, t('groupsHint'))),
      // The app's sign-in codes come from the admin only: a browser, which
      // the app approved, cannot mint the app's own access (DECISIONS).
      section(t('otherPhone'),
        h('p', {}, t('otherPhoneByAdmin'))),
    ];
  }


}
