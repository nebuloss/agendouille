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
import { Toasts, card, confirmDialog, dialog, field, formDialog, langChoice, swatch, themeToggle } from '@/shared/components/Ui.ts';
import { ApiPage } from '@/shared/components/ApiPage.ts';
import { currentTheme } from '@/shared/core/Theme.ts';
import type { Calendar, Calendars } from '@/app/api.ts';
import { scanQR } from '@/shared/components/Scanner.ts';
import { ago } from '@/shared/render/format.ts';
import { BridgeError, type Bridge } from '@/shared/core/Bridge.ts';
import { parseState, type AppState, type CalendarState } from '@/android/state.ts';
import { bridgeApi } from '@/android/api.ts';
import { seg, type Api } from '@/shared/core/Api.ts';
import { CalendarsPage, browserName, kindLabel, type Device, type People, type Session } from '@/app/pages/Manage.ts';
import { codeFromHash } from '@/app/pages/ApprovePage.ts';
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

  constructor(doc: Document, api: Api, private readonly phone: () => AppState | null, private readonly toggle: (c: CalendarState, on: boolean) => void) {
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
      const sub = c.owner === me
        ? (shares.length > 0 ? t('sharedWith', { count: shares.length }) : t('notShared'))
        : t('ownedBy', { name: c.owner });
      const b = h('button', { type: 'button', class: 'state-layer flex min-h-16 w-full items-center gap-3 px-4 py-2 text-left' },
        swatch(c.color, 'md'),
        h('span', { class: 'flex min-w-0 flex-1 flex-col' },
          h('span', { class: 'truncate font-medium' }, c.name),
          h('span', { class: 'truncate text-xs text-on-surface-variant' }, sub)),
        c.hidden ? h('span', { class: ui.pill.muted }, t('hiddenPill')) : null,
        icon('right'));
      b.addEventListener('click', () => this.open(c.ref));
      return b;
    };
    const group = (title: string, cs: Calendar[]): Child[] => cs.length === 0 ? [] : [
      h('h2', { class: 'px-1 pt-2 text-sm font-semibold text-on-surface-variant' }, title),
      h('div', { class: 'flex flex-col divide-y divide-outline-variant overflow-hidden rounded-md3-xl bg-surface-container' }, ...cs.map(row)),
    ];
    const create = h('button', { type: 'button', class: ui.btn('filled') }, icon('plus'), t('newCalendar'));
    create.addEventListener('click', () => void this.create());
    const mine = all.filter((c) => c.owner === me);
    const others = all.filter((c) => c.owner !== me);
    return [
      h('div', { class: 'flex items-end justify-between gap-3' }, heading(t('tabCalendars')), create),
      ...(all.length === 0 ? [h('p', { class: ui.empty }, t('noCalendar'))] : []),
      ...group(t('mine'), mine),
      ...group(t('sharedWithMe'), others),
    ];
  }

  private detail(c: Calendar, me: string, people: People): Child[] {
    const back = h('button', { type: 'button', class: ui.btn('text', 'sm') }, icon('left'), t('tabCalendars'));
    back.addEventListener('click', () => this.open(null));
    const phone = this.phone()?.calendars.find((x) => x.ref === c.ref);
    let sync: Child;
    if (phone === undefined) {
      sync = h('p', { class: 'text-sm text-on-surface-variant' }, t('notOnPhoneYet'));
    } else {
      const box = h('input', { type: 'checkbox', checked: phone.sync, 'aria-label': t('syncThis', { name: c.name }) });
      box.addEventListener('change', () => this.toggle(phone, box.checked));
      sync = h('label', { class: 'flex min-h-12 items-center gap-3' },
        h('span', { class: 'flex min-w-0 flex-1 flex-col' },
          h('span', { class: 'font-medium' }, t('syncOnPhone')),
          h('span', { class: 'text-xs text-on-surface-variant' }, phone.read_only ? t('readOnly') : t('syncOnPhoneHelp'))),
        box);
    }
    return [
      h('div', { class: 'pt-2' }, back),
      c.owner === me ? this.ownedCard(c, people) : this.sharedCard(c),
      card(t('onThisPhone'), [], sync),
    ];
  }
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
    const row = (main: string, sub: string, side: Child): HTMLElement =>
      h('div', { class: 'flex min-h-16 items-center gap-3 px-4 py-2' },
        h('span', { class: 'flex min-w-0 flex-1 flex-col' }, h('span', { class: 'truncate font-medium' }, main), h('span', { class: 'text-xs text-on-surface-variant' }, sub)),
        side);
    const seen = (iso: string | null | undefined): string => (iso ? t('lastSeen', { ago: ago(iso) }) : t('neverSeen'));
    const list = (rows: HTMLElement[], empty: string): HTMLElement => rows.length === 0
      ? h('p', { class: ui.empty }, empty)
      : h('div', { class: 'flex flex-col divide-y divide-outline-variant overflow-hidden rounded-md3-xl bg-surface-container' }, ...rows);
    const devices = d.devices.map((x) => row(x.label, `${kindLabel(x.kind)} · ${seen(x.last_used_at)}`,
      x.id === own ? h('span', { class: ui.pill.ok }, t('thisPhone'))
        : off(x.label, t('disconnectDeviceTitle', { name: x.label }), t('disconnectDeviceText'), () => this.api.delete(`/devices/${x.id}`), t('deviceDisconnected'))));
    // The app's own sessions are its devices, listed above.
    const browsers = s.sessions.filter((x) => !x.app).map((x) => {
      const name = browserName(x.user_agent);
      return row(name, seen(x.seen_at),
        off(name, t('disconnectBrowserTitle', { name }), t('disconnectBrowserText'), () => this.api.delete(`/sessions/${x.id}`), t('browserDisconnected')));
    });
    return [
      heading(t('tabDevices')),
      this.connect,
      h('h2', { class: 'px-1 pt-2 text-sm font-semibold text-on-surface-variant' }, t('syncedDevices')),
      list(devices, t('noDevices')),
      h('h2', { class: 'px-1 pt-2 text-sm font-semibold text-on-surface-variant' }, t('signedInBrowsers')),
      list(browsers, t('noBrowsers')),
    ];
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
  private page: ApiPage | null = null;

  constructor(private readonly doc: Document, private readonly root: HTMLElement, private readonly bridge: Bridge) {
    this.toasts = new Toasts(doc.getElementById('toasts') ?? doc.body);
    this.api = bridgeApi(bridge);
    bridge.on('state', (s) => this.show(parseState(s)));
    // A calendar made, shared or removed: the phone syncs it now.
    doc.addEventListener('agd:changed', () => void bridge.call('sync.now').catch(() => undefined));
  }

  async start(): Promise<void> {
    this.show(parseState(await this.bridge.call('state')));
  }

  private show(s: AppState): void {
    const before = this.state;
    this.state = s;
    // Only Synchro draws from the state: the other tabs keep their page
    // while the sync reports, unless the account itself changed.
    const same = before?.account != null && s.account != null && before.account.server === s.account.server
      && before.account.username === s.account.username && before.crash === s.crash;
    if (this.tab !== 'sync' && same) return;
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
    if (this.tab === 'sync') append(this.root, ...this.account(s));
    else if (this.tab === 'calendars') this.calendarsTab(s);
    else if (this.tab === 'devices') this.devicesTab(s);
    else append(this.root, ...this.settingsTab());
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
    this.mount(s, (doc) => new PhoneCalendars(doc, this.api, () => this.state,
      (c, on) => void this.act('calendar.toggle', { id: c.id, enabled: on }, on ? t('syncOn', { name: c.name }) : t('syncOff', { name: c.name }))));
  }

  /** Signed in with a device's password: the calendars on the phone, to sync or not, and how to manage them. */
  private phoneCalendars(s: AppState): Child[] {
    const rows = s.calendars.map((c) => {
      const box = h('input', { type: 'checkbox', checked: c.sync, 'aria-label': t('syncThis', { name: c.name }) });
      box.addEventListener('change', () => void this.act('calendar.toggle', { id: c.id, enabled: box.checked }));
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

  // --- Réglages

  private settingsTab(): Child[] {
    const language = langChoice(this.doc);
    language.addEventListener('change', () => remember('agd_lang', language instanceof HTMLSelectElement ? language.value : ''));
    const theme = themeToggle(this.doc);
    theme.addEventListener('click', () => remember('agd_theme', currentTheme(this.doc)));
    return [
      heading(t('tabSettings')),
      card(t('language'), [], h('p', { class: 'text-sm text-on-surface-variant' }, t('languageHint')), h('div', {}, language)),
      card(t('appearance'), [], h('p', { class: 'text-sm text-on-surface-variant' }, t('appearanceHint')), h('div', {}, theme)),
      card(t('about'), [],
        h('dl', { class: ui.kv },
          h('dt', {}, t('version')), h('dd', {}, h('code', {}, this.bridge.hello?.app_version ?? '?')),
          h('dt', {}, t('android')), h('dd', {}, String(this.bridge.hello?.android ?? '?')))),
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

  /** The waiting browser, then its number among three (PLAN.md 8.2). */
  private async approve(code: string): Promise<void> {
    let view: { user_agent: string; ip: string; numbers: number[] };
    try {
      view = await this.api.get(`/approvals/${seg(code)}`);
    } catch (e) {
      this.toasts.error(e);
      return;
    }
    const numbers = h('div', { class: 'grid grid-cols-3 gap-3' }, ...view.numbers.map((n) =>
      h('button', { type: 'submit', value: String(n), formnovalidate: true, class: 'state-layer h-16 rounded-md3-l bg-secondary-container text-2xl font-semibold text-on-secondary-container' }, String(n))));
    const v = await dialog(this.doc, t('approveTitle'), [
      h('p', {}, t('approveQuestion', { device: browserName(view.user_agent), ip: view.ip })),
      h('p', { class: 'text-sm text-on-surface-variant' }, t('approvePick')),
      numbers,
    ], [{ label: t('notMe'), value: '' }]);
    if (v === '') return;
    try {
      await this.api.post(`/approvals/${seg(code)}`, { number: Number(v) });
      this.toasts.show(t('approved'));
      void this.page?.refresh();
    } catch (e) {
      this.toasts.error(e);
    }
  }

  /** A call that answers with the new state, its error in a toast. */
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
    out.push(this.syncCard(s), this.accountCard(s));
    return out;
  }

  private syncCard(s: AppState): HTMLElement {
    const st = s.sync;
    const line = st.state === 'syncing' ? t('syncing')
      : st.state === 'error' ? t('syncError', { error: st.error ?? '' })
        : st.last_sync !== null ? t('lastSync', { ago: ago(st.last_sync) }) : t('neverSynced');
    const now = h('button', { type: 'button', class: ui.btn('tonal', 'sm'), disabled: st.state === 'syncing' || !s.permissions.calendar }, icon('repeat'), t('syncNow'));
    now.addEventListener('click', () => void this.act('sync.now'));
    return card(t('sync'), [now],
      h('p', { class: st.state === 'error' ? 'text-sm font-semibold text-error' : 'text-sm text-on-surface-variant', role: 'status' }, line),
      s.calendars.length > 0 ? h('p', { class: 'text-sm text-on-surface-variant' }, t('eventsOnPhone', { count: st.events })) : null,
      s.calendars.length > 0 ? h('p', { class: 'text-sm text-on-surface-variant' }, t('calendarsOnPhone', { count: s.calendars.filter((c) => c.sync).length })) : null);
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
        h('dt', {}, t('signedInBy')), h('dd', {}, a.mode === 'manual' ? t('byHand') : a.mode === 'qr' ? t('byDavx5') : t('byLink'))),
      h('div', {}, remove));
  }
}
