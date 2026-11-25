/**
 * The Android app's screens (PLAN.md 7, DECISIONS: the app is a sync
 * app): a welcome that signs the phone in (the QR code of an enrollment
 * link, or the account by hand), then four tabs: Synchro (the sync and the
 * account), Calendriers (the list, then a calendar's page: the web app's
 * own card, and whether it syncs on this phone), Appareils (approving a
 * browser, the devices and browsers signed in, to disconnect them) and
 * Réglages (language, theme). No event is shown here: the phone's own
 * calendar app shows them; nothing opens the website either. Java does the
 * work, over the bridge.
 */
import { append, h, type Child } from '@/shared/core/Dom.ts';
import { icon } from '@/shared/components/icons.ts';
import { ui } from '@/shared/components/recipes.ts';
import { Toasts, card, chip, confirmDialog, dialog, field, formDialog, segmented, switchButton, stat, swatch, themeToggle } from '@/shared/components/Ui.ts';
import { recencyTone } from '@/shared/components/tone.ts';
import { flag } from '@/shared/components/flags.ts';
import { LANGS, lang, setLang, type Lang } from '@/shared/i18n/i18n.ts';
import { ApiPage } from '@/shared/components/ApiPage.ts';
import { currentTheme } from '@/shared/core/Theme.ts';
import type { Calendar, Calendars } from '@/app/api.ts';
import { scanQR } from '@/shared/components/Scanner.ts';
import { ago } from '@/shared/render/format.ts';
import { BridgeError, type Bridge } from '@/shared/core/Bridge.ts';
import { parseState, type AppState, type CalendarState, type UpdateOffer } from '@/android/state.ts';
import { bridgeApi } from '@/android/api.ts';
import { seg, type Api } from '@/shared/core/Api.ts';
import { CalendarsPage, browserName, kindLabel, type Device, type People, type Session } from '@/app/pages/Manage.ts';
import { CONFIRM_DELAY_MS, codeFromHash } from '@/app/pages/ApprovePage.ts';
import { t } from '@/android/App.msg.ts';

/** The bridge's error as a sentence: the app's message, else its code's. */
function reason(e: unknown): string {
  if (e instanceof BridgeError) return e.detail ?? t('failed');
  return e instanceof Error ? e.message : String(e);
}

type Tab = 'sync' | 'calendars' | 'devices' | 'settings';

/** Keeps a choice of the screens (language, theme) for the next start; main.ts reads them. */
export function remember(key: 'agd_lang' | 'agd_theme', value: string): void {
  try {
    globalThis.localStorage?.setItem(key, value);
  } catch {
    // no storage: the choice lasts until the app closes
  }
}

const heading = (text: string): HTMLElement => h('h1', { class: 'pt-4 text-[22px] font-normal' }, text);

/**
 * Calendriers: the names, then one calendar's page, the web app's own
 * card (share, edit, give, delete; hide for one shared with me) without
 * its files, and whether it syncs on this phone.
 */
class PhoneCalendars extends CalendarsPage {
  protected override readonly files = false;
  private selected: string | null = null;

  constructor(doc: Document, api: Api, private readonly phone: () => AppState | null, private readonly toggle: (c: CalendarState, on: boolean) => void,
    private readonly align: (c: CalendarState, on: boolean) => void = () => undefined) {
    super(doc, api);
  }

  protected override async render(): Promise<Child[]> {
    const [cals, people] = await Promise.all([this.api.get<Calendars>('/calendars'), this.api.get<People>('/people')]);
    const me = this.doc.querySelector<HTMLElement>('.app')?.dataset['user'] ?? '';
    const c = this.selected === null ? undefined : cals.calendars.find((x) => x.ref === this.selected);
    if (c === undefined) {
      this.selected = null;
      return this.list(cals.calendars, me);
    }
    return this.detail(c, me, people);
  }

  private open(ref: string | null): void {
    this.selected = ref;
    void this.refresh();
    this.doc.defaultView?.scrollTo(0, 0);
  }

  private list(all: Calendar[], me: string): Child[] {
    const row = (c: Calendar): HTMLElement => {
      const shares = c.shares ?? [];
      const phone = this.phone()?.calendars.find((x) => x.ref === c.ref);
      if (phone !== undefined && phone.sync !== c.shown) this.align(phone, c.shown);
      const b = h('button', { type: 'button', class: 'state-layer flex min-h-14 min-w-0 flex-1 items-center gap-4 py-2 pr-2 pl-4 text-left' },
        swatch(c.color, 'md'),
        h('span', { class: 'flex min-w-0 flex-1 flex-col gap-1' },
          h('span', { class: 'truncate text-base font-medium' }, c.name),
          h('span', { class: 'flex flex-wrap items-center gap-1.5' }, ...calendarChips(c, me, shares.length))),
        phone === undefined ? h('span', { class: 'text-on-surface-variant' }, icon('right')) : null);
      b.addEventListener('click', () => this.open(c.ref));
      // On the phone: synced here or not, in one tap.
      if (phone === undefined) return b;
      return h('div', { class: 'flex items-center' }, b,
        h('span', { class: 'flex shrink-0 items-center pr-4' },
          switchButton(this.doc, t('syncThis', { name: c.name }), c.shown, (on) => this.toggle(phone, on))));
    };
    const create = h('button', { type: 'button', class: ui.btn('filled', 'sm') }, icon('plus'), t('newCalendar'));
    create.addEventListener('click', () => void this.create());
    const mine = all.filter((c) => c.owner === me);
    const others = all.filter((c) => c.owner !== me);
    return [
      h('div', { class: 'flex items-center justify-between gap-3 pt-4' }, h('h1', { class: 'text-[22px] leading-7 font-normal' }, t('tabCalendars')), create),
      ...(all.length === 0 ? [h('p', { class: ui.empty }, t('noCalendar'))] : []),
      ...group(t('mine'), mine.map(row)),
      ...group(t('sharedWithMe'), others.map(row)),
    ];
  }

  /** One calendar: its name and colour, then grouped rows, Android's settings fashion. */
  private detail(c: Calendar, me: string, people: People): Child[] {
    const back = h('button', { type: 'button', class: `${ui.btn('text', 'sm')} -ml-3 self-start` }, icon('left'), t('tabCalendars'));
    back.addEventListener('click', () => this.open(null));
    const owned = c.owner === me;
    const shares = c.shares ?? [];
    const head = h('div', { class: 'flex items-center gap-3' }, swatch(c.color, 'md'),
      h('div', { class: 'flex min-w-0 flex-col' },
        h('h1', { class: 'truncate text-[22px] leading-7 font-normal' }, c.name),
        h('span', { class: 'flex flex-wrap items-center gap-1.5 pt-1' }, ...calendarChips(c, me, shares.length))));

    // Sur ce téléphone: whether it syncs here.
    const phone = this.phone()?.calendars.find((x) => x.ref === c.ref);
    let sync: HTMLElement;
    if (phone === undefined) {
      sync = h('p', { class: 'px-4 py-3 text-sm text-on-surface-variant' }, t('notOnPhoneYet'));
    } else {
      const box = switchButton(this.doc, t('syncThis', { name: c.name }), c.shown, (on) => this.toggle(phone, on));
      sync = h('label', { class: ROW },
        h('span', { class: 'flex min-w-0 flex-1 flex-col' },
          h('span', { class: 'text-base' }, t('syncOnPhone')),
          h('span', { class: 'text-sm text-on-surface-variant' }, phone.read_only ? t('readOnlyHere') : t('syncOnPhoneHelp'))),
        box);
    }

    // Partagé avec: each share, its access in place; then « Partager ».
    const shareRows = shares.map((sh) => {
      const who = this.nameOf(people, sh.target);
      const path = `/calendars/${seg(c.ref)}/shares/${seg(sh.target)}`;
      const access = segmented(this.doc, t('access'), [{ value: 'read', label: t('accessRead') }, { value: 'readwrite', label: t('accessReadwrite') }],
        sh.access === 'readwrite' ? 'readwrite' : 'read',
        (v) => void this.act(() => this.api.put(path, { access: v }), t('accessChanged', { name: who })), 'sm');
      const off = h('button', { type: 'button', class: 'state-layer grid size-10 shrink-0 place-items-center rounded-full text-on-surface-variant',
        title: t('unshare'), 'aria-label': `${t('unshare')} ${who}` }, icon('close'));
      off.addEventListener('click', () => void this.confirmed(t('unshareOneTitle', { name: who }), t('unshareOneText', { calendar: c.name, name: who }),
        t('unshare'), () => this.api.delete(path), t('unshared', { name: who })));
      return h('div', { class: 'flex items-center gap-3 py-2 pr-2 pl-4' }, this.mark(sh.target.startsWith('g:') ? 'group' : 'person'),
        h('span', { class: 'flex min-w-0 flex-1 flex-col gap-1.5' }, h('span', { class: 'truncate text-base' }, who), access.el), off);
    });
    const action = (ic: Parameters<typeof icon>[0], label: string, run: () => void, danger = false): HTMLElement => {
      const b = h('button', { type: 'button', class: `${ROW} ${danger ? 'text-error' : ''}` },
        h('span', { class: danger ? 'text-error' : 'text-primary' }, icon(ic)), h('span', { class: 'flex-1 text-base' }, label));
      b.addEventListener('click', run);
      return b;
    };

    if (!owned) {
      return [
        h('div', { class: 'flex flex-col gap-2 pt-2' }, back, head),
        ...group(t('onThisPhone'), [sync]),
        ...group(t('actions'), [
          action('palette', t('customise'), () => void this.editProps(c)),
          c.hidden
            ? action('show', t('showAgain'), () => void this.act(() => this.api.patch(`/calendars/${seg(c.ref)}`, { hidden: false }), t('shownAgain')))
            : action('hide', t('hide'), () => void this.act(() => this.api.patch(`/calendars/${seg(c.ref)}`, { hidden: true }), t('hiddenDone'))),
        ]),
      ];
    }
    return [
      h('div', { class: 'flex flex-col gap-2 pt-2' }, back, head,
        c.description !== '' ? h('p', { class: 'text-sm text-on-surface-variant' }, c.description) : null),
      ...group(t('onThisPhone'), [sync]),
      ...group(t('sharing'), [...shareRows, action('share', shares.length > 0 ? t('shareMore') : t('shareIt'), () => void this.share(c, people))]),
      ...group(t('actions'), [
        action('edit', t('edit'), () => void this.editProps(c)),
        action('give', t('transfer'), () => void this.transfer(c, people)),
        action('trash', t('delete'), () => void this.remove(c), true),
      ]),
    ];
  }
}

/** A calendar's state in chips: shared or private for mine; whose and what access for another's; hidden. */
function calendarChips(c: Calendar, me: string, shares: number): HTMLElement[] {
  const out = c.owner === me
    ? [shares > 0 ? chip(t('sharedChip', { count: shares }), 'accent', 'share') : chip(t('privateChip'), 'neutral', 'lock')]
    : [chip(c.access === 'read' ? t('readOnly') : t('canWrite'), c.access === 'read' ? 'neutral' : 'accent', c.access === 'read' ? 'show' : 'edit'),
      h('span', { class: 'text-xs text-on-surface-variant' }, t('ownedBy', { name: c.owner }))];
  if (c.hidden) out.push(chip(t('hiddenPill'), 'warn', 'hide'));
  return out;
}

/** A row of a grouped list: full width, 56 px at least, Material's list item. */
const ROW = 'state-layer flex min-h-14 w-full items-center gap-4 px-4 py-2 text-left';

/** A grouped list under a small heading, Android's settings fashion. */
function group(title: string, rows: readonly HTMLElement[]): Child[] {
  if (rows.length === 0) return [];
  return [
    h('h2', { class: 'px-4 pt-3 text-sm font-medium text-primary' }, title),
    h('div', { class: 'flex flex-col divide-y divide-outline-variant overflow-hidden rounded-md3-xl bg-surface-container' }, ...rows),
  ];
}

/** Appareils: approving a browser, then the devices and browsers signed in, to disconnect them. */
class PhoneDevices extends ApiPage {
  constructor(doc: Document, api: Api, private readonly own: () => number, private readonly connect: HTMLElement) {
    super(doc, api, 'app-content');
  }

  protected async render(): Promise<Child[]> {
    const [d, s] = await Promise.all([this.api.get<{ devices: Device[] }>('/devices'), this.api.get<{ sessions: Session[] }>('/sessions')]);
    const own = this.own();
    const off = (label: string, title: string, text: string, run: () => Promise<unknown>, done: string): HTMLElement => {
      const b = h('button', { type: 'button', class: ui.btn('danger-text', 'sm') }, icon('logout'), t('disconnect'));
      b.setAttribute('aria-label', `${t('disconnect')} ${label}`);
      b.addEventListener('click', () => void (async () => {
        if (!(await confirmDialog(this.doc, title, text, t('disconnect'), true))) return;
        await this.act(run, done);
      })());
      return b;
    };
    const row = (main: string, sub: Child, seenAt: string | null | undefined, side: Child): HTMLElement =>
      h('div', { class: 'flex min-h-16 items-center gap-3 px-4 py-2' },
        h('span', { class: 'flex min-w-0 flex-1 flex-col gap-1' }, h('span', { class: 'truncate font-medium' }, main),
          h('span', { class: 'flex flex-wrap items-center gap-1.5' }, sub,
            chip(seenAt ? t('lastSeen', { ago: ago(seenAt) }) : t('neverSeen'), recencyTone(seenAt), 'clock'))),
        side);
    const list = (rows: HTMLElement[], empty: string): HTMLElement => rows.length === 0
      ? h('p', { class: ui.empty }, empty)
      : h('div', { class: 'flex flex-col divide-y divide-outline-variant overflow-hidden rounded-md3-xl bg-surface-container' }, ...rows);
    const devices = d.devices.map((x) => row(x.label, h('span', { class: 'text-xs text-on-surface-variant' }, kindLabel(x.kind)), x.last_used_at,
      x.id === own ? chip(t('thisPhone'), 'accent', 'phone')
        : off(x.label, t('disconnectDeviceTitle', { name: x.label }), t('disconnectDeviceText'), () => this.api.delete(`/devices/${x.id}`), t('deviceDisconnected'))));
    // The app's own sessions are its devices, listed above.
    const browsers = s.sessions.filter((x) => !x.app).map((x) => {
      const name = browserName(x.user_agent);
      return row(name, null, x.seen_at,
        off(name, t('disconnectBrowserTitle', { name }), t('disconnectBrowserText'), () => this.api.delete(`/sessions/${x.id}`), t('browserDisconnected')));
    });
    return [
      heading(t('tabDevices')),
      this.connect,
      h('h2', { class: 'px-4 pt-3 text-sm font-medium text-primary' }, t('syncedDevices')),
      list(devices, t('noDevices')),
      h('h2', { class: 'px-4 pt-3 text-sm font-medium text-primary' }, t('signedInBrowsers')),
      list(browsers, t('noBrowsers')),
      browsers.length > 1 ? this.allOff(browsers.length) : null,
    ];
  }

  /** « Déconnecter tous les navigateurs »: every browser of mine at once (not the app). */
  private allOff(count: number): HTMLElement {
    const b = h('button', { type: 'button', class: `${ui.btn('danger-text')} self-start` }, icon('logout'), t('disconnectAllBrowsers'));
    b.addEventListener('click', () => void (async () => {
      if (!(await confirmDialog(this.doc, t('disconnectAllTitle'), t('disconnectAllText', { count }), t('disconnectAllButton'), true))) return;
      await this.act(() => this.api.delete('/sessions'), t('allBrowsersDisconnected'));
    })());
    return b;
  }
}

/** The code of a browser's sign-in page: its QR code (…/approve#c=CODE), or the code typed. */
export function approvalCode(text: string): string {
  const s = text.trim();
  try {
    const u = new URL(s);
    return u.pathname.endsWith('/approve') ? codeFromHash(u.hash) : '';
  } catch {
    return /^[A-Za-z0-9]{3}-?[A-Za-z0-9]{3,}$/.test(s) ? s.toUpperCase() : '';
  }
}

export class App {
  private readonly toasts: Toasts;
  private readonly api: Api;
  private state: AppState | null = null;
  private tab: Tab = 'sync';
  /** Réglages, or its permissions' view. */
  private settingsView: 'main' | 'permissions' = 'main';
  /** The update offer the dialog showed last (its time), not to show it twice. */
  private updateShown = 0;
  /** The open update dialog's status line, which the "update" events fill. */
  private updateStatus: ((state: string, message?: string) => void) | null = null;
  private page: ApiPage | null = null;

  constructor(private readonly doc: Document, private readonly root: HTMLElement, private readonly bridge: Bridge) {
    this.toasts = new Toasts(doc.getElementById('toasts') ?? doc.body);
    this.api = bridgeApi(bridge);
    bridge.on('state', (s) => this.show(parseState(s)));
    // A tapped alert: its view.
    bridge.on('navigate', (d) => {
      if ((d as Record<string, unknown> | null)?.['view'] === 'permissions' && this.state?.account) this.showPermissions();
    });
    bridge.on('update', (d) => {
      const o = (d ?? {}) as Record<string, unknown>;
      this.updateStatus?.(String(o['state'] ?? ''), typeof o['message'] === 'string' ? o['message'] : undefined);
    });
    // A calendar made, shared or removed: the phone syncs it now.
    doc.addEventListener('agd:changed', () => void bridge.call('sync.now').catch(() => undefined));
  }

  async start(): Promise<void> {
    this.show(parseState(await this.bridge.call('state')));
  }

  private show(s: AppState): void {
    const before = this.state;
    this.state = s;
    if (s.update !== null && s.update.at !== this.updateShown) {
      this.updateShown = s.update.at;
      this.updateDialog(s.update);
    }
    // Only Synchro draws from the state: the other tabs keep their page
    // while the sync reports, unless the account itself changed.
    const same = before?.account != null && s.account != null && before.account.server === s.account.server
      && before.account.username === s.account.username && before.crash === s.crash;
    if (this.tab !== 'sync' && this.tab !== 'settings' && same) return;
    if (s.account === null) this.tab = 'sync';
    this.draw();
  }

  private draw(): void {
    const s = this.state;
    if (s === null) return;
    this.page?.dispose();
    this.page = null;
    this.root.replaceChildren();
    this.root.classList.toggle('pb-28', s.account !== null);
    if (s.crash !== null) append(this.root, this.crashCard(s));
    if (s.account === null) {
      append(this.root, ...this.welcome());
      return;
    }
    if (s.push.service && !s.push.modeChosen) {
      append(this.root, ...this.modeView());
      return;
    }
    if (this.tab === 'sync') append(this.root, ...this.account(s));
    else if (this.tab === 'calendars') this.calendarsTab(s);
    else if (this.tab === 'devices') this.devicesTab(s);
    else append(this.root, ...(this.settingsView === 'permissions' ? this.permissionsView(s) : this.settingsTab(s)));
    append(this.root, this.nav());
  }

  /** The tabs, at the bottom, as the web app's on a phone. */
  private nav(): HTMLElement {
    const item = (tab: Tab, ic: Parameters<typeof icon>[0], label: string): HTMLElement => {
      const on = this.tab === tab;
      const b = h('button', {
        type: 'button',
        class: on ? 'flex flex-1 flex-col items-center gap-1 pt-3 pb-3 text-xs font-semibold text-on-surface' : 'flex flex-1 flex-col items-center gap-1 pt-3 pb-3 text-xs font-medium text-on-surface-variant',
        ...(on ? { 'aria-current': 'page' } : {}),
      }, h('span', { class: on ? 'grid h-8 w-16 place-items-center rounded-md3-full bg-secondary-container text-on-secondary-container' : 'grid h-8 w-16 place-items-center rounded-md3-full' }, icon(ic)), label);
      b.addEventListener('click', () => {
        if (this.tab === tab) return;
        this.tab = tab;
        this.settingsView = 'main';
        this.draw();
        this.doc.defaultView?.scrollTo(0, 0);
      });
      return b;
    };
    return h('nav', { class: 'fixed inset-x-0 bottom-0 z-40 flex border-t border-outline-variant bg-surface-container pb-[env(safe-area-inset-bottom)]', 'aria-label': t('tabs') },
      item('sync', 'repeat', t('tabSync')), item('calendars', 'calendar', t('tabCalendars')),
      item('devices', 'devices', t('tabDevices')), item('settings', 'gear', t('tabSettings')));
  }

  /** The page in a mount of its own: the web app's code finds the person in .app[data-user]. */
  private mount(s: AppState, page: (doc: Document) => ApiPage): void {
    append(this.root, h('div', { class: 'app contents', 'data-user': s.account!.username }, h('div', { id: 'app-content', class: 'flex flex-col gap-4' })));
    this.page = page(this.doc);
    this.page.start();
  }

  private calendarsTab(s: AppState): void {
    if (s.account!.mode !== 'enrolled') {
      append(this.root, ...this.phoneCalendars(s));
      return;
    }
    this.mount(s, (doc) => new PhoneCalendars(doc, this.api, () => this.state, (c, on) => void this.setShown(c, on),
      (c, on) => void this.bridge.call('calendar.toggle', { id: c.id, enabled: on }).catch(() => undefined)));
  }

  /** Signed in with a device's password: the calendars on the phone, to sync or not, and how to manage them. */
  private phoneCalendars(s: AppState): Child[] {
    const rows = s.calendars.map((c) => {
      const box = switchButton(this.doc, t('syncThis', { name: c.name }), c.sync, (on) => void this.act('calendar.toggle', { id: c.id, enabled: on }));
      return h('label', { class: 'flex min-h-14 items-center gap-3 px-4 py-2' },
        swatch(c.color, 'md'),
        h('span', { class: 'flex min-w-0 flex-1 flex-col' },
          h('span', { class: 'truncate font-medium' }, c.name),
          c.read_only ? h('span', { class: 'text-xs text-on-surface-variant' }, t('readOnly')) : null),
        box);
    });
    return [
      heading(t('tabCalendars')),
      rows.length === 0
        ? h('p', { class: ui.empty }, s.permissions.calendar ? t('noCalendarsYet') : t('noCalendarsPermission'))
        : h('div', { class: 'flex flex-col divide-y divide-outline-variant overflow-hidden rounded-md3-xl bg-surface-container' }, ...rows),
      h('section', { class: 'flex flex-col gap-2 rounded-md3-xl bg-surface-container p-5' },
        h('p', {}, t('needsAppBody')),
        h('p', { class: 'text-sm text-on-surface-variant' }, t('needsAppHow'))),
    ];
  }

  private devicesTab(s: AppState): void {
    if (s.account!.mode !== 'enrolled') {
      append(this.root, heading(t('tabDevices')),
        h('section', { class: 'flex flex-col gap-2 rounded-md3-xl bg-surface-container p-5' },
          h('p', {}, t('needsAppBody')),
          h('p', { class: 'text-sm text-on-surface-variant' }, t('needsAppHow'))));
      return;
    }
    this.mount(s, (doc) => new PhoneDevices(doc, this.api, () => this.state?.account?.device_id ?? 0, this.connectBrowser()));
  }

  /** A bridge call that opens one of Android's screens; its result comes back with onResume. */
  private open(call: string): void {
    void this.bridge.call(call).catch((e: unknown) => this.toasts.error(new Error(reason(e))));
  }

  /** How many permissions are still to grant. */
  private static missing(s: AppState): number {
    const p = s.permissions;
    // The camera is asked when scanning; the battery only matters to instant sync.
    // Instant sync: the battery exemption, and its notification hidden.
    return [p.calendar, p.notifications, ...(s.push.instant ? [p.battery, s.push.hidden] : [])].filter((x) => !x).length;
  }

  /** The permissions' view, from anywhere: Réglages' tab, the view on top. */
  private showPermissions(): void {
    this.tab = 'settings';
    this.settingsView = 'permissions';
    this.draw();
    this.doc.defaultView?.scrollTo(0, 0);
  }

  /** Synchro, while a permission is missing: a small link to them all. */
  private permissionsLink(s: AppState): HTMLElement | null {
    const n = App.missing(s);
    if (n === 0) return null;
    const b = h('button', { type: 'button', class: 'state-layer -ml-3 inline-flex h-10 items-center gap-2 self-start rounded-md3-full px-3 text-sm font-medium text-warning [&_.ic]:size-4' }, icon('warn'), t('someMissing', { count: n }), icon('right'));
    b.addEventListener('click', () => this.showPermissions());
    return b;
  }

  /** Réglages' card: how many permissions are missing, and the view of them all. */
  private permissionsCard(s: AppState): HTMLElement {
    const missing = App.missing(s);
    const go = h('button', { type: 'button', class: `${ui.btn('tonal')} self-start` }, icon('lock'), t('checkPermissions'));
    go.addEventListener('click', () => this.showPermissions());
    return card(t('permissions'), [],
      h('div', {}, missing === 0 ? chip(t('allGranted'), 'ok', 'check') : chip(t('someMissing', { count: missing }), 'warn', 'warn')),
      go);
  }

  /**
   * Instant sync's notification, while instant sync is on: Android's, not a
   * permission, but the person's to show or hide in its own category.
   */
  private pushNotificationRow(s: AppState): HTMLElement {
    const b = h('button', { type: 'button', class: ui.btn(s.push.hidden ? 'text' : 'filled', 'sm') }, s.push.hidden ? t('pushNotificationSetting') : t('hideIt'));
    b.addEventListener('click', () => void this.bridge.call('push.set', { notification: true }).catch((e: unknown) => this.toasts.error(new Error(reason(e)))));
    return h('div', { class: 'flex items-start gap-4 px-4 py-3' },
      h('span', { class: 'pt-0.5 text-primary' }, icon('repeat')),
      h('span', { class: 'flex min-w-0 flex-1 flex-col gap-1.5' },
        h('span', { class: 'text-base font-medium' }, t('permPushNotification')),
        h('span', { class: 'text-sm text-on-surface-variant' }, t('permPushNotificationWhy')),
        h('span', { class: 'flex flex-wrap items-center gap-2' },
          s.push.hidden ? chip(t('permHidden'), 'ok', 'check') : chip(t('permVisible'), 'warn', 'warn'), b)));
  }

  /** Which mode the list is for, and the way to change it (Réglages' card). */
  private permissionsMode(s: AppState): HTMLElement | null {
    if (!s.push.service) return null;
    const change = h('button', { type: 'button', class: ui.btn('text', 'sm') }, t('changeMode'));
    change.addEventListener('click', () => {
      this.settingsView = 'main';
      this.draw();
    });
    return h('div', { class: 'flex flex-wrap items-center gap-2 rounded-md3-l bg-secondary-container px-4 py-2 text-on-secondary-container' },
      h('span', { class: 'flex-1 text-sm' }, s.push.instant ? t('permsForInstant') : t('permsForStandard')), change);
  }

  /** Each permission: what it is for, granted or not, and the way to grant it. */
  private permissionsView(s: AppState): Child[] {
    const p = s.permissions;
    const back = h('button', { type: 'button', class: `${ui.btn('text', 'sm')} -ml-3 self-start` }, icon('left'), t('tabSettings'));
    back.addEventListener('click', () => {
      this.settingsView = 'main';
      this.draw();
    });
    const row = (ic: Parameters<typeof icon>[0], title: string, why: string, ok: boolean | null, call: string, label: string, actionCall?: (b: HTMLElement) => void): HTMLElement => {
      const b = h('button', { type: 'button', class: ui.btn(ok === false ? 'filled' : 'text', 'sm') }, label);
      if (actionCall) actionCall(b);
      else b.addEventListener('click', () => void this.bridge.call(call).then((st) => { if (st) this.show(parseState(st)); }, (e: unknown) => this.toasts.error(new Error(reason(e)))));
      return h('div', { class: 'flex items-start gap-4 px-4 py-3' },
        h('span', { class: 'pt-0.5 text-primary' }, icon(ic)),
        h('span', { class: 'flex min-w-0 flex-1 flex-col gap-1.5' },
          h('span', { class: 'text-base font-medium' }, title),
          h('span', { class: 'text-sm text-on-surface-variant' }, why),
          h('span', { class: 'flex flex-wrap items-center gap-2' },
            ok === null ? chip(call === 'camera.request' ? t('optional') : t('toCheck'), 'neutral', 'info') : ok ? chip(t('granted'), 'ok', 'check') : chip(t('notGranted'), 'warn', 'warn'),
            ok === true ? null : b)));
    };
    const opener = (call: string) => (b: HTMLElement): void => b.addEventListener('click', () => this.open(call));
    const rows = [
      row('calendar', t('permCalendar'), t('permCalendarWhy'), p.calendar, 'calendar.request', t('allow')),
      row('bell', t('permNotifications'), t('permNotificationsWhy'), p.notifications, 'notifications.request', t('allow')),
      ...(s.push.instant ? [
        row('clock', t('permBattery'), t('permBatteryWhyInstant'), p.battery, 'battery.request', t('allow'), opener('battery.request')),
        this.pushNotificationRow(s),
      ] : []),
      row('qr', t('permCamera'), t('permCameraWhy'), p.camera ? true : null, 'camera.request', t('allow')),
      ...(s.xiaomi ? [row('repeat', t('permAutostart'), t('permAutostartWhy'), null, 'autostart.open', t('open'), opener('autostart.open'))] : []),
    ];
    const settings = h('button', { type: 'button', class: `${ui.btn('text', 'sm')} self-start` }, icon('gear'), t('appSettings'));
    settings.addEventListener('click', () => this.open('settings.open'));
    return [
      h('div', { class: 'flex flex-col gap-2 pt-2' }, back, h('h1', { class: 'text-[22px] leading-7 font-normal' }, t('permissions'))),
      this.permissionsMode(s),
      h('div', { class: 'flex flex-col divide-y divide-outline-variant overflow-hidden rounded-md3-xl bg-surface-container' }, ...rows),
      h('p', { class: 'px-1 text-sm text-on-surface-variant' }, t('deniedTwice')),
      settings,
    ];
  }

  // --- Réglages

  /** « Rechercher une mise à jour »: the app checks now; Android's own dialog offers it. */
  private updateButton(): HTMLElement {
    const b = h('button', { type: 'button', class: `${ui.btn('tonal')} self-start` }, icon('download'), t('checkUpdate'));
    b.addEventListener('click', () => void (async () => {
      b.disabled = true;
      try {
        const r = await this.bridge.call<{ current?: string; available?: string | null }>('update.check');
        if (r.available) this.toasts.show(t('updateFound', { version: r.available }));
        else this.toasts.show(t('upToDateApp', { version: r.current ?? '' }));
      } catch (e) {
        this.toasts.error(new Error(reason(e)));
      } finally {
        b.disabled = false;
      }
    })());
    return b;
  }

  /** Choosing the mode: a card each, what it does and what it asks. */
  private modeChoices(current: boolean | null): HTMLElement {
    const choice = (instant: boolean, ic: Parameters<typeof icon>[0], title: string, body: string, asks: string): HTMLElement => {
      const on = current === instant;
      const b = h('button', { type: 'button', 'aria-pressed': String(on),
        class: on ? 'state-layer flex w-full flex-col gap-1.5 rounded-md3-xl border-2 border-primary bg-secondary-container p-4 text-left text-on-secondary-container'
          : 'state-layer flex w-full flex-col gap-1.5 rounded-md3-xl border-2 border-outline-variant bg-surface-container p-4 text-left' },
      h('span', { class: 'flex items-center gap-2 text-lg font-medium' }, icon(ic), title, on ? h('span', { class: 'ml-auto' }, icon('check')) : null),
      h('span', { class: 'text-sm' }, body),
      h('span', { class: 'text-sm text-on-surface-variant' }, asks));
      b.addEventListener('click', () => {
        if (current === instant) return;
        void this.bridge.call('push.set', { instant }).then((st) => { if (st) this.show(parseState(st)); },
          (e: unknown) => this.toasts.error(new Error(reason(e))));
      });
      return b;
    };
    return h('div', { class: 'flex flex-col gap-3' },
      choice(false, 'repeat', t('modeStandard'), t('modeStandardBody'), t('modeStandardAsks')),
      choice(true, 'bell', t('modeInstant'), t('modeInstantBody'), t('modeInstantAsks')));
  }

  /** The first screen after signing in, once: the mode, changeable in Réglages. */
  private modeView(): Child[] {
    return [
      h('div', { class: 'flex flex-col gap-2 pt-4' },
        h('h1', { class: 'text-[22px] leading-7 font-normal' }, t('modeTitle')),
        h('p', { class: 'text-sm text-on-surface-variant' }, t('modeIntro'))),
      this.modeChoices(null),
    ];
  }

  /** Réglages: the mode, and instant sync's state while it is chosen. */
  private pushCard(s: AppState): HTMLElement {
    const p = s.push;
    if (!p.service) return card(t('modeTitle'), [], h('p', { class: 'text-sm text-on-surface-variant' }, t('pushNoServiceBody')));
    const body: Child[] = [this.modeChoices(p.instant)];
    if (p.instant) {
      body.push(h('div', {}, p.connected ? chip(t('pushActive'), 'ok', 'check') : chip(t('pushConnecting'), 'warn', 'clock')));
      if (!p.hidden) {
        const ribbon = h('button', { type: 'button', class: 'state-layer flex w-full items-center gap-3 rounded-md3-l bg-warning-container px-4 py-3 text-left text-on-warning-container' },
          icon('warn'), h('span', { class: 'flex-1 text-sm' }, t('pushVisibleRibbon')), icon('right'));
        ribbon.addEventListener('click', () => this.showPermissions());
        body.push(ribbon);
      }
      body.push(h('p', { class: 'text-sm text-on-surface-variant' },
        p.users.length > 0 ? t('pushUsers', { apps: p.users.join(', ') }) : t('pushDavx5')));
    }
    return card(t('modeTitle'), [], ...body);
  }

  /**
   * An update on offer: Material's dialog, with the change of version and
   * where it comes from; then, in place, the download and Android's
   * confirmation, or what went wrong.
   */
  private updateDialog(u: UpdateOffer): void {
    const el = this.doc.createElement('dialog');
    el.className = `${ui.dialog} outline-none`;
    const status = h('div', { class: 'flex w-full flex-col items-center gap-2', hidden: true });
    const later = h('button', { type: 'button', class: ui.btn('text') }, t('updateLater'));
    const install = h('button', { type: 'button', class: ui.btn('filled') }, icon('download'), t('updateInstall'));
    const bar = (): HTMLElement => h('div', { class: 'h-1 w-full overflow-hidden rounded-full bg-surface-highest' },
      h('div', { class: 'h-full w-1/3 animate-pulse rounded-full bg-primary' }));
    const setStatus = (state: string, message?: string): void => {
      status.hidden = false;
      if (state === 'downloading') {
        status.replaceChildren(bar(), h('p', { class: 'text-sm text-on-surface-variant' }, t('updateDownloading')));
        install.disabled = true;
      } else if (state === 'confirm') {
        status.replaceChildren(h('p', { class: 'text-sm text-on-surface-variant' }, t('updateConfirm')));
      } else if (state === 'cancelled') {
        status.replaceChildren(h('p', { class: 'text-sm text-on-surface-variant' }, t('updateCancelled')));
        install.disabled = false;
        install.replaceChildren(icon('download'), t('updateRetry'));
      } else if (state === 'failed') {
        status.replaceChildren(h('p', { class: 'flex items-start gap-2 rounded-md3-m bg-error-container px-3 py-2 text-left text-sm text-on-error-container' },
          icon('warn'), message ?? t('updateFailed')));
        install.disabled = false;
        install.replaceChildren(icon('download'), t('updateRetry'));
      }
    };
    this.updateStatus = setStatus;
    install.addEventListener('click', () => void this.bridge.call('update.install').catch((e: unknown) => setStatus('failed', reason(e))));
    later.addEventListener('click', () => {
      el.close();
      void this.bridge.call('update.later').then((st) => { if (st) this.show(parseState(st)); }, () => undefined);
    });
    el.append(h('div', { class: 'flex flex-col items-center gap-4 px-6 pt-6 pb-2 text-center' },
      h('span', { class: 'grid size-14 place-items-center rounded-full bg-primary-container text-on-primary-container [&_.ic]:size-7' }, icon('download')),
      h('h2', { class: 'text-2xl leading-8 font-normal' }, t('updateTitle')),
      h('div', { class: 'flex flex-wrap items-center justify-center gap-2' },
        chip(u.current, 'neutral'), h('span', { class: 'text-on-surface-variant' }, icon('right')), chip(u.version, 'ok')),
      h('p', { class: 'text-sm text-on-surface-variant' }, u.source === 'github' ? t('updateFromGitHub') : t('updateFromServer')),
      status),
    h('div', { class: 'flex justify-end gap-2 px-6 pt-2 pb-5' }, later, install));
    el.addEventListener('close', () => {
      if (this.updateStatus === setStatus) this.updateStatus = null;
      el.remove();
    });
    this.doc.body.append(el);
    el.showModal();
    // The dialog itself, not « Plus tard »: no focus ring on a touchscreen.
    el.tabIndex = -1;
    el.focus();
  }

  /** « Versions de développement »: where updates come from. */
  private devUpdatesRow(s: AppState): HTMLElement {
    const sw = switchButton(this.doc, t('devUpdates'), s.devUpdates, (on) => {
      void this.bridge.call('updates.dev', { on }).then((st) => { if (st) this.show(parseState(st)); },
        (e: unknown) => this.toasts.error(new Error(reason(e))));
    });
    return h('label', { class: 'flex items-center gap-4' },
      h('span', { class: 'flex min-w-0 flex-1 flex-col gap-0.5' },
        h('span', { class: 'text-base font-medium' }, t('devUpdates')),
        h('span', { class: 'text-sm text-on-surface-variant' }, s.devUpdates ? t('devUpdatesOn') : t('devUpdatesOff'))),
      sw);
  }

  private settingsTab(s: AppState): Child[] {
    // The language: kept for the next start (main.ts), then the page again in it.
    const language = segmented(this.doc, t('language'),
      (Object.keys(LANGS) as Lang[]).map((l) => ({ value: l, label: LANGS[l], lead: flag(l, 16, this.doc), lang: l })), lang(this.doc),
      (l) => {
        remember('agd_lang', l);
        setLang(l, this.doc);
      }).el;
    const theme = themeToggle(this.doc);
    theme.addEventListener('click', () => remember('agd_theme', currentTheme(this.doc)));
    return [
      heading(t('tabSettings')),
      card(t('language'), [], h('p', { class: 'text-sm text-on-surface-variant' }, t('languageHint')), h('div', {}, language)),
      card(t('appearance'), [], h('p', { class: 'text-sm text-on-surface-variant' }, t('appearanceHint')), h('div', {}, theme)),
      this.permissionsCard(s),
      this.pushCard(s),
      card(t('about'), [],
        h('dl', { class: ui.kv },
          h('dt', {}, t('version')), h('dd', {}, h('code', {}, this.bridge.hello?.app_version ?? '?')),
          h('dt', {}, t('android')), h('dd', {}, String(this.bridge.hello?.android ?? '?'))),
        this.devUpdatesRow(s),
        this.updateButton()),
    ];
  }

  // --- approving a browser

  private connectBrowser(): HTMLElement {
    const code = h('input', { autocapitalize: 'characters', autocomplete: 'off', spellcheck: false, placeholder: 'ABC-123', class: `${ui.input} font-mono uppercase`, 'aria-label': t('codeLabel') });
    const scan = h('button', { type: 'button', class: `${ui.btn('filled')} h-12 justify-center` }, icon('qr'), t('scanBrowser'));
    scan.addEventListener('click', () => void this.scanBrowser());
    const next = h('button', { type: 'submit', class: ui.btn('tonal') }, t('next'));
    const form = h('form', { class: 'flex items-end gap-2' }, h('div', { class: 'min-w-0 flex-1' }, field(t('orCode'), code)), next);
    form.addEventListener('submit', (e) => {
      e.preventDefault();
      const c = approvalCode(code.value);
      if (c === '') this.toasts.show(t('notACode'), 'error');
      else void this.approve(c);
    });
    return h('section', { class: 'flex flex-col gap-4 rounded-md3-xl bg-surface-container p-5' },
      h('h2', { class: 'text-lg font-medium' }, t('browserTitle')),
      h('p', { class: 'text-sm text-on-surface-variant' }, t('browserBody')),
      scan, form);
  }

  private async scanBrowser(): Promise<void> {
    try {
      await this.bridge.call('camera.request');
    } catch (e) {
      this.toasts.error(new Error(reason(e)));
      return;
    }
    const text = await scanQR(this.doc);
    if (text === null) return;
    const c = approvalCode(text);
    if (c === '') {
      this.toasts.show(t('notABrowserCode'), 'error');
      return;
    }
    await this.approve(c);
  }

  /**
   * The waiting browser, to confirm or refuse (PLAN.md 8.2), as WhatsApp's
   * and Discord's QR sign-in: who is asking and from where, a warning, and
   * « Se connecter » after a moment.
   */
  private async approve(code: string): Promise<void> {
    let view: { user_agent: string; ip: string };
    try {
      view = await this.api.get(`/approvals/${seg(code)}`);
    } catch (e) {
      this.toasts.error(e);
      return;
    }
    const asked = dialog(this.doc, t('approveTitle'), [
      h('div', { class: 'flex items-center gap-3 rounded-md3-l bg-surface-high p-3' },
        h('span', { class: 'grid size-10 shrink-0 place-items-center rounded-full bg-secondary-container text-on-secondary-container' }, icon('laptop')),
        h('span', { class: 'flex min-w-0 flex-col' },
          h('span', { class: 'truncate font-medium' }, browserName(view.user_agent)),
          h('span', { class: 'text-sm text-on-surface-variant' }, t('fromAddress', { ip: view.ip })))),
      h('p', { class: 'text-sm' }, t('approveQuestion')),
      h('p', { class: ui.notice }, icon('warn'), t('approveWarning')),
    ], [{ label: t('notMe'), value: 'no' }, { label: t('approveConfirm'), value: 'yes', kind: 'filled' }]);
    const yes = this.doc.querySelector<HTMLButtonElement>('dialog[open] button[value="yes"]');
    if (yes !== null) {
      yes.disabled = true;
      this.doc.defaultView?.setTimeout(() => { yes.disabled = false; }, CONFIRM_DELAY_MS);
    }
    const v = await asked;
    try {
      if (v === 'yes') {
        await this.api.post(`/approvals/${seg(code)}`, {});
        this.toasts.show(t('approved'));
        void this.page?.refresh();
      } else if (v === 'no') {
        await this.api.delete(`/approvals/${seg(code)}`);
        this.toasts.show(t('refused'));
      }
    } catch (e) {
      this.toasts.error(e);
    }
  }

  /** A call that answers with the new state, its error in a toast. */
  /**
   * The switch of a calendar: the person's (« affiché », on the server, the
   * web app's too), then this phone at once; other phones at their next sync.
   */
  private async setShown(c: CalendarState, on: boolean): Promise<void> {
    try {
      await this.api.patch(`/calendars/${seg(c.ref)}`, { shown: on });
    } catch (e) {
      this.toasts.error(new Error(reason(e)));
      return;
    }
    await this.act('calendar.toggle', { id: c.id, enabled: on }, on ? t('syncOn', { name: c.name }) : t('syncOff', { name: c.name }));
  }

  private async act(name: string, args?: Record<string, unknown>, done?: string): Promise<boolean> {
    try {
      this.show(parseState(await this.bridge.call(name, args)));
      if (done !== undefined) this.toasts.show(done);
      return true;
    } catch (e) {
      this.toasts.error(new Error(reason(e)));
      return false;
    }
  }

  /** The report of what made the app stop last time: copy it, send it, or let it be. */
  private crashCard(s: AppState): HTMLElement {
    const report = h('pre', { class: 'max-h-48 overflow-auto rounded-md3-m bg-surface-lowest p-3 font-mono text-[11px] whitespace-pre-wrap' }, s.crash ?? '');
    const copy = h('button', { type: 'button', class: ui.btn('text', 'sm') }, icon('copy'), t('crashCopy'));
    copy.addEventListener('click', () => {
      void navigator.clipboard?.writeText(s.crash ?? '').then(() => this.toasts.show(t('crashCopied')), () => undefined);
    });
    const sendIt = h('button', { type: 'button', class: ui.btn('tonal', 'sm'), disabled: s.account === null }, icon('upload'), t('crashSend'));
    sendIt.addEventListener('click', () => void this.act('crash.send', undefined, t('crashSent')));
    const dismiss = h('button', { type: 'button', class: ui.btn('text', 'sm') }, icon('close'), t('crashDismiss'));
    dismiss.addEventListener('click', () => void this.act('crash.clear'));
    return h('section', { class: 'mt-4 flex flex-col gap-3 rounded-md3-xl bg-error-container p-5 text-on-error-container' },
      h('h2', { class: 'text-lg font-medium' }, t('crashTitle')),
      h('p', { class: 'text-sm' }, t('crashBody')),
      report,
      h('div', { class: 'flex flex-wrap gap-2' }, sendIt, copy, dismiss));
  }

  // --- welcome

  private welcome(): Child[] {
    const parts: HTMLElement[] = [
      h('header', { class: 'flex flex-col items-center gap-4 pt-10 pb-2 text-center' },
        h('img', { src: '/icon.svg', alt: '', width: 88, height: 88, class: 'rounded-md3-xl' }),
        h('h1', { class: 'text-[28px] font-normal' }, t('welcome')),
        h('p', { class: 'max-w-sm text-on-surface-variant' }, t('welcomeBody'))),
      h('div', { class: 'flex flex-col gap-3' },
        h('button', { type: 'button', class: `${ui.btn('filled')} h-14 justify-center text-base`, 'data-action': 'scan' }, icon('qr'), t('scan')),
        h('button', { type: 'button', class: `${ui.btn('tonal')} h-12 justify-center`, 'data-action': 'manual' }, icon('edit'), t('manual'))),
      h('p', { class: 'px-2 text-center text-sm text-on-surface-variant' }, t('whereQr')),
    ];
    return parts.map((el) => this.wire(el));
  }

  private wire<E extends HTMLElement>(el: E): E {
    for (const b of Array.from(el.querySelectorAll<HTMLButtonElement>('[data-action]'))) {
      const a = b.dataset['action'];
      b.addEventListener('click', () => void (a === 'scan' ? this.scan() : a === 'manual' ? this.manual() : undefined));
    }
    return el;
  }

  private async scan(): Promise<void> {
    try {
      await this.bridge.call('camera.request');
    } catch (e) {
      this.toasts.error(new Error(reason(e)));
      return;
    }
    const text = await scanQR(this.doc);
    if (text === null) return;
    await this.act('enroll', { link: text }, t('signedIn'));
  }

  private async manual(): Promise<void> {
    const server = h('input', { required: true, type: 'url', inputmode: 'url', autocomplete: 'url', placeholder: 'https://agenda.example.org', autocapitalize: 'off' });
    const username = h('input', { required: true, autocomplete: 'username', autocapitalize: 'off', spellcheck: false });
    const password = h('input', { required: true, type: 'password', autocomplete: 'current-password' });
    await formDialog(this.doc, t('manualTitle'), [
      h('p', { class: 'text-sm text-on-surface-variant' }, t('manualHelp')),
      field(t('server'), server), field(t('username'), username), field(t('password'), password),
    ], t('signIn'), async () => {
      try {
        this.show(parseState(await this.bridge.call('manual', { server: server.value.trim(), username: username.value.trim(), password: password.value })));
      } catch (e) {
        throw new Error(reason(e));
      }
      this.toasts.show(t('signedIn'));
    });
  }

  // --- the account

  private account(s: AppState): Child[] {
    const a = s.account!;
    const out: Child[] = [
      h('header', { class: 'flex items-center gap-3 pt-4' },
        h('img', { src: '/icon.svg', alt: '', width: 44, height: 44, class: 'rounded-md3-m' }),
        h('div', { class: 'flex min-w-0 flex-col' },
          h('h1', { class: 'truncate text-[22px] font-normal' }, a.username),
          h('span', { class: 'truncate text-sm text-on-surface-variant' }, a.host))),
    ];
    if (!s.permissions.calendar) {
      const allow = h('button', { type: 'button', class: ui.btn('filled') }, icon('calendar'), t('allowCalendar'));
      allow.addEventListener('click', () => void this.act('calendar.request', undefined, t('allowed')));
      out.push(h('section', { class: 'flex flex-col gap-3 rounded-md3-xl bg-tertiary-container p-5 text-on-tertiary-container' },
        h('h2', { class: 'text-lg font-medium' }, t('permissionTitle')),
        h('p', { class: 'text-sm' }, t('permissionBody')),
        h('div', {}, allow)));
    }
    out.push(this.syncCard(s));
    const link = this.permissionsLink(s);
    if (link) out.push(link);
    out.push(this.accountCard(s));
    return out;
  }

  private syncCard(s: AppState): HTMLElement {
    const st = s.sync;
    const state = st.state === 'syncing' ? chip(t('syncing'), 'accent', 'repeat')
      : st.state === 'error' ? chip(t('syncFailed'), 'error', 'warn')
        : st.last_sync !== null ? chip(t('upToDate'), 'ok', 'check') : chip(t('neverSeen'), 'warn', 'clock');
    const now = h('button', { type: 'button', class: ui.btn('tonal', 'sm'), disabled: st.state === 'syncing' || !s.permissions.calendar }, icon('repeat'), t('syncNow'));
    now.addEventListener('click', () => void this.act('sync.now'));
    return card(t('sync'), [now],
      h('div', { class: 'flex flex-wrap items-center gap-2', role: 'status' }, state,
        st.last_sync !== null ? h('span', { class: 'text-sm text-on-surface-variant' }, ago(st.last_sync)) : null),
      st.state === 'error' && st.error ? h('p', { class: 'text-sm text-error' }, st.error) : null,
      s.calendars.length > 0
        ? h('div', { class: 'grid grid-cols-2 gap-2' }, stat(st.events, t('eventsStat')), stat(s.calendars.filter((c) => c.sync).length, t('calendarsStat')))
        : null);
  }

  private accountCard(s: AppState): HTMLElement {
    const a = s.account!;
    const remove = h('button', { type: 'button', class: ui.btn('danger-text') }, icon('logout'), t('signOut'));
    remove.addEventListener('click', () => void (async () => {
      if (!(await confirmDialog(this.doc, t('removeTitle'), a.mode === 'enrolled' ? t('removeBodyEnrolled') : t('removeBodyManual'), t('signOut'), true))) return;
      await this.act('account.remove', undefined, t('removed'));
    })());
    return card(t('account'), [],
      h('dl', { class: ui.kv },
        h('dt', {}, t('server')), h('dd', {}, h('code', {}, a.server)),
        h('dt', {}, t('username')), h('dd', {}, h('code', {}, a.username)),
        h('dt', {}, t('signedInBy')), h('dd', {}, a.mode === 'enrolled' ? chip(t('byLink'), 'ok', 'check')
          : chip(`${a.mode === 'manual' ? t('byHand') : t('byDavx5')} · ${t('syncOnly')}`, 'warn', 'lock'))),
      h('div', {}, remove));
  }
}
