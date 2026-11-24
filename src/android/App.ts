/**
 * The Android app's screens (PLAN.md 7, DECISIONS: the app is a sync
 * app): a welcome that signs the phone in (the QR code of an enrollment
 * link, or the account by hand), then the account: permissions, the
 * calendars synced to the phone, the sync. No calendar is shown here: the
 * phone's own calendar app shows them. Java does the work, over the bridge.
 */
import { append, h, type Child } from '@/shared/core/Dom.ts';
import { icon } from '@/shared/components/icons.ts';
import { ui } from '@/shared/components/recipes.ts';
import { Toasts, card, confirmDialog, field, formDialog, swatch } from '@/shared/components/Ui.ts';
import { scanQR } from '@/shared/components/Scanner.ts';
import { ago } from '@/shared/render/format.ts';
import { BridgeError, type Bridge } from '@/shared/core/Bridge.ts';
import { parseState, type AppState } from '@/android/state.ts';
import { t } from '@/android/App.msg.ts';

/** The bridge's error as a sentence: the app's message, else its code's. */
function reason(e: unknown): string {
  if (e instanceof BridgeError) return e.detail ?? t('failed');
  return e instanceof Error ? e.message : String(e);
}

export class App {
  private readonly toasts: Toasts;

  constructor(private readonly doc: Document, private readonly root: HTMLElement, private readonly bridge: Bridge) {
    this.toasts = new Toasts(doc.getElementById('toasts') ?? doc.body);
    bridge.on('state', (s) => this.show(parseState(s)));
  }

  async start(): Promise<void> {
    this.show(parseState(await this.bridge.call('state')));
  }

  private show(s: AppState): void {
    this.root.replaceChildren();
    if (s.crash !== null) append(this.root, this.crashCard(s));
    append(this.root, ...(s.account === null ? this.welcome() : this.account(s)));
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
