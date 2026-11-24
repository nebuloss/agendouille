/**
 * The Android app's screens (PLAN.md 7, DECISIONS: the app is a sync
 * app): a welcome that signs the phone in (the QR code of an enrollment
 * link, or the account by hand), then three tabs: Synchro (permissions,
 * the calendars synced to the phone, the sync), Calendriers (the web
 * app's own page: create, share, give, remove; DECISIONS) and Navigateur
 * (approving a browser's sign-in). No event is shown here: the phone's own
 * calendar app shows them. Java does the work, over the bridge.
 */
import { append, h, type Child } from '@/shared/core/Dom.ts';
import { icon } from '@/shared/components/icons.ts';
import { ui } from '@/shared/components/recipes.ts';
import { Toasts, card, confirmDialog, dialog, field, formDialog, swatch } from '@/shared/components/Ui.ts';
import { scanQR } from '@/shared/components/Scanner.ts';
import { ago } from '@/shared/render/format.ts';
import { BridgeError, type Bridge } from '@/shared/core/Bridge.ts';
import { parseState, type AppState } from '@/android/state.ts';
import { bridgeApi } from '@/android/api.ts';
import { seg, type Api } from '@/shared/core/Api.ts';
import { CalendarsPage, browserName } from '@/app/pages/Manage.ts';
import { codeFromHash } from '@/app/pages/ApprovePage.ts';
import { t } from '@/android/App.msg.ts';

/** The bridge's error as a sentence: the app's message, else its code's. */
function reason(e: unknown): string {
  if (e instanceof BridgeError) return e.detail ?? t('failed');
  return e instanceof Error ? e.message : String(e);
}

type Tab = 'sync' | 'calendars' | 'browser';

/** The web app's Calendriers, without its files (export, import). */
class PhoneCalendars extends CalendarsPage {
  protected override readonly files = false;
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
  private calendars: PhoneCalendars | null = null;

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
    this.calendars?.dispose();
    this.calendars = null;
    this.root.replaceChildren();
    this.root.classList.toggle('pb-28', s.account !== null);
    if (s.crash !== null) append(this.root, this.crashCard(s));
    if (s.account === null) {
      append(this.root, ...this.welcome());
      return;
    }
    if (this.tab === 'sync') append(this.root, ...this.account(s));
    else if (this.tab === 'calendars') this.calendarsTab(s);
    else append(this.root, ...this.browserTab(s));
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
      item('sync', 'repeat', t('tabSync')), item('calendars', 'calendar', t('tabCalendars')), item('browser', 'laptop', t('tabBrowser')));
  }

  /** Calendriers and Navigateur speak for the person: a device's password opens CalDAV only. */
  private needsApp(title: string): Child[] {
    return [
      h('h1', { class: 'pt-4 text-[22px] font-normal' }, title),
      h('section', { class: 'flex flex-col gap-3 rounded-md3-xl bg-surface-container p-5' },
        h('p', {}, t('needsAppBody')),
        h('p', { class: 'text-sm text-on-surface-variant' }, t('needsAppHow'))),
    ];
  }

  private calendarsTab(s: AppState): void {
    const a = s.account!;
    if (a.mode !== 'enrolled') {
      append(this.root, ...this.needsApp(t('tabCalendars')));
      return;
    }
    // The web app's page, in a mount of its own: it finds the person in .app[data-user].
    const mount = h('div', { id: 'app-content', class: 'flex flex-col gap-4' }, h('h1', { class: 'pt-4 text-[22px] font-normal' }, t('tabCalendars')));
    append(this.root, h('div', { class: 'app contents', 'data-user': a.username }, mount));
    this.calendars = new PhoneCalendars(this.doc, this.api);
    this.calendars.start();
  }

  // --- approving a browser

  private browserTab(s: AppState): Child[] {
    if (s.account!.mode !== 'enrolled') return this.needsApp(t('tabBrowser'));
    const code = h('input', { autocapitalize: 'characters', autocomplete: 'off', spellcheck: false, placeholder: 'ABC-123', class: 'font-mono uppercase', 'aria-label': t('codeLabel') });
    const scan = h('button', { type: 'button', class: `${ui.btn('filled')} h-14 justify-center text-base` }, icon('qr'), t('scanBrowser'));
    scan.addEventListener('click', () => void this.scanBrowser());
    const next = h('button', { type: 'submit', class: ui.btn('tonal') }, t('next'));
    const form = h('form', { class: 'flex flex-col gap-3' }, field(t('codeLabel'), code), h('div', { class: 'flex justify-end' }, next));
    form.addEventListener('submit', (e) => {
      e.preventDefault();
      const c = approvalCode(code.value);
      if (c === '') this.toasts.show(t('notACode'), 'error');
      else void this.approve(c);
    });
    return [
      h('h1', { class: 'pt-4 text-[22px] font-normal' }, t('browserTitle')),
      h('section', { class: 'flex flex-col gap-4 rounded-md3-xl bg-surface-container p-5' },
        h('p', {}, t('browserBody')),
        scan),
      card(t('byCode'), [], h('p', { class: 'text-sm text-on-surface-variant' }, t('byCodeHelp')), form),
    ];
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
    out.push(this.syncCard(s), this.calendarsCard(s), this.accountCard(s));
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
      s.calendars.length > 0 ? h('p', { class: 'text-sm text-on-surface-variant' }, t('eventsOnPhone', { count: st.events })) : null);
  }

  private calendarsCard(s: AppState): HTMLElement {
    if (s.calendars.length === 0) {
      return card(t('calendars'), [], h('p', { class: ui.empty }, s.permissions.calendar ? t('noCalendarsYet') : t('noCalendarsPermission')));
    }
    return card(t('calendars'), [],
      h('p', { class: 'text-sm text-on-surface-variant' }, t('calendarsHelp')),
      h('div', { class: 'flex flex-col divide-y divide-outline-variant overflow-hidden rounded-md3-l bg-surface-lowest' },
        ...s.calendars.map((c) => {
          const box = h('input', { type: 'checkbox', checked: c.sync, 'aria-label': t('syncThis', { name: c.name }) });
          box.addEventListener('change', () => void this.act('calendar.toggle', { id: c.id, enabled: box.checked }));
          return h('label', { class: 'flex min-h-14 items-center gap-3 px-4 py-2' },
            swatch(c.color, 'md'),
            h('span', { class: 'flex min-w-0 flex-1 flex-col' },
              h('span', { class: 'truncate font-medium' }, c.name),
              c.read_only ? h('span', { class: 'text-xs text-on-surface-variant' }, t('readOnly')) : null),
            box);
        })));
  }

  private accountCard(s: AppState): HTMLElement {
    const a = s.account!;
    const open = h('button', { type: 'button', class: ui.btn('text', 'sm') }, icon('link'), t('openWeb'));
    open.addEventListener('click', () => void this.bridge.call('open', { url: `${a.server}/` }).catch(() => undefined));
    const remove = h('button', { type: 'button', class: ui.btn('danger-text', 'sm') }, icon('logout'), t('remove'));
    remove.addEventListener('click', () => void (async () => {
      if (!(await confirmDialog(this.doc, t('removeTitle'), a.mode === 'enrolled' ? t('removeBodyEnrolled') : t('removeBodyManual'), t('remove'), true))) return;
      await this.act('account.remove', undefined, t('removed'));
    })());
    return card(t('account'), [],
      h('dl', { class: ui.kv },
        h('dt', {}, t('server')), h('dd', {}, h('code', {}, a.server)),
        h('dt', {}, t('username')), h('dd', {}, h('code', {}, a.username)),
        h('dt', {}, t('signedInBy')), h('dd', {}, a.mode === 'manual' ? t('byHand') : a.mode === 'qr' ? t('byDavx5') : t('byLink'))),
      h('div', { class: 'flex flex-wrap gap-2' }, open, remove));
  }
}
