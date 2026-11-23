import type { Api } from '@/shared/core/Api.ts';
import { append, h, type Child } from '@/shared/core/Dom.ts';
import type { Disposable } from '@/shared/core/Disposable.ts';
import { icon, type IconName } from '@/shared/components/icons.ts';
import { ui } from '@/shared/components/recipes.ts';
import { ago } from '@/shared/render/format.ts';
import { t } from '@/admin/Notices.msg.ts';

type Key = Parameters<typeof t>[0];

/**
 * The notification centre, as claudication's and cafard's: a bell in the
 * top bar counting what needs the admin, in the colour of the worst, and a
 * side sheet listing it by what it is about, each a link to where it is
 * dealt with. The server works the notices out from the state; one marked
 * read leaves the bell until its problem is gone.
 */

export interface Notice {
  id: string;
  kind: 'server' | 'device' | 'user';
  severity: 'error' | 'warn' | 'info';
  title: string;
  detail?: string;
  link?: string;
  since?: string;
  read: boolean;
}

type Severity = Notice['severity'];

const TONE: Record<Severity, { icon: IconName; circle: string; badge: string; label: Key }> = {
  error: { icon: 'warn', circle: 'bg-error-container text-on-error-container', badge: 'bg-error text-on-error', label: 'toFix' },
  warn: { icon: 'warn', circle: 'bg-warning-container text-on-warning-container', badge: 'bg-warning text-on-warning', label: 'warning' },
  info: { icon: 'info', circle: 'bg-info-container text-on-info-container', badge: 'bg-info-container text-on-info-container', label: 'info' },
};

const GROUPS: { kind: Notice['kind']; label: Key }[] = [
  { kind: 'server', label: 'server' },
  { kind: 'device', label: 'devices' },
  { kind: 'user', label: 'users' },
];

/** The worst severity among notices, or null. */
export function worst(notices: Notice[]): Severity | null {
  return (['error', 'warn', 'info'] as const).find((s) => notices.some((n) => n.severity === s)) ?? null;
}

export class NoticeCenter implements Disposable {
  private notices: Notice[] = [];
  private filter: Severity | 'all' = 'all';
  private readonly bell: HTMLButtonElement;
  private sheet: HTMLDialogElement | null = null;
  private timer: ReturnType<typeof setInterval> | null = null;

  constructor(
    private readonly doc: Document,
    private readonly api: Api,
    slot: HTMLElement,
  ) {
    this.bell = h('button', { type: 'button', class: 'state-layer relative grid size-10 shrink-0 place-items-center rounded-full border border-outline text-on-surface-variant', 'aria-haspopup': 'dialog' });
    this.bell.addEventListener('click', () => this.open());
    slot.replaceChildren(this.bell);
    this.drawBell();
  }

  /** Reads the notices now and every minute, so the bell counts what just happened. */
  start(): void {
    void this.load();
    this.timer = setInterval(() => void this.load(), 60_000);
  }

  dispose(): void {
    if (this.timer !== null) clearInterval(this.timer);
  }

  unread(): Notice[] {
    return this.notices.filter((n) => !n.read);
  }

  async load(): Promise<void> {
    try {
      this.notices = (await this.api.get<{ notices: Notice[] }>('/notices')).notices;
    } catch {
      return; // the page says so if the server is gone; the bell keeps its count
    }
    this.drawBell();
    this.drawSheet();
  }

  private drawBell(): void {
    const unread = this.unread();
    const n = unread.length;
    const w = worst(unread);
    const label = t('bell', { count: n });
    this.bell.setAttribute('aria-label', label);
    this.bell.setAttribute('title', label);
    this.bell.replaceChildren(icon('bell'));
    append(this.bell,
      n > 0 && w !== null ? h('span', { class: `absolute -top-1 -right-1 grid h-5 min-w-5 place-items-center rounded-full px-1 text-[11px] font-semibold tabular-nums ${TONE[w].badge}` }, n > 99 ? '99+' : String(n)) : null);
  }

  private async mark(ids: string[]): Promise<void> {
    try {
      this.notices = (await this.api.post<{ notices: Notice[] }>('/notices/read', { ids })).notices;
    } catch {
      return;
    }
    this.drawBell();
    this.drawSheet();
  }

  open(): void {
    if (this.sheet !== null) return;
    const sheet = this.doc.createElement('dialog');
    sheet.className = 'm-0 ml-auto flex h-dvh max-h-dvh w-full max-w-[460px] flex-col bg-surface p-0 text-on-surface elevation-2 animate-sheet-in sm:rounded-l-md3-xl';
    sheet.setAttribute('aria-label', t('notifications'));
    sheet.addEventListener('click', (e) => {
      if (e.target === sheet) sheet.close();
    });
    sheet.addEventListener('close', () => {
      sheet.remove();
      this.sheet = null;
    });
    this.sheet = sheet;
    this.doc.body.append(sheet);
    this.drawSheet();
    if (typeof sheet.showModal === 'function') sheet.showModal();
    else sheet.setAttribute('open', '');
  }

  private drawSheet(): void {
    const sheet = this.sheet;
    if (sheet === null) return;
    const close = h('button', { type: 'button', class: ui.iconBtn, 'aria-label': t('close') }, icon('close'));
    close.addEventListener('click', () => sheet.close());
    const head = h('div', { class: 'flex items-center gap-3 px-6 pt-5 pb-3' }, h('h2', { class: 'mr-auto text-2xl font-normal' }, t('notifications')), close);
    const unread = this.unread();
    if (unread.length === 0) {
      sheet.replaceChildren(head, h('div', { class: 'flex flex-1 flex-col items-center justify-center gap-3 px-6 py-16 text-center' },
        h('span', { class: 'grid size-14 place-items-center rounded-full bg-success-container text-on-success-container [&_.ic]:size-7' }, icon('check')),
        h('span', { class: 'text-base' }, t('allClear'))));
      return;
    }
    const count = (s: Severity | 'all'): number => unread.filter((n) => s === 'all' || n.severity === s).length;
    if (this.filter !== 'all' && count(this.filter) === 0) this.filter = 'all';
    const chips = (['all', 'error', 'warn', 'info'] as const).filter((s) => s === 'all' || count(s) > 0).map((s) => {
      const on = this.filter === s;
      const b = h('button', { type: 'button', role: 'radio', 'aria-checked': String(on),
        class: on ? 'state-layer flex h-8 items-center gap-1.5 rounded-md3-s bg-secondary-container px-3 text-xs font-medium text-on-secondary-container'
          : 'state-layer flex h-8 items-center gap-1.5 rounded-md3-s px-3 text-xs font-medium text-on-surface-variant ring-1 ring-outline-variant ring-inset' },
      s === 'all' ? t('all') : t(TONE[s].label), h('span', { class: 'tabular-nums opacity-70' }, String(count(s))));
      b.addEventListener('click', () => {
        this.filter = s;
        this.drawSheet();
      });
      return b;
    });
    const shown = unread.filter((n) => this.filter === 'all' || n.severity === this.filter);
    const readAll = h('button', { type: 'button', class: ui.btn('text', 'sm') }, icon('check'), t('markAllRead'));
    readAll.addEventListener('click', () => void this.mark(shown.map((n) => n.id)));
    const groups: Child[] = GROUPS.map((g) => {
      const items = shown.filter((n) => n.kind === g.kind);
      if (items.length === 0) return null;
      return h('section', { 'aria-label': t(g.label) },
        h('h3', { class: 'mb-2 flex items-center gap-2 text-xs font-semibold tracking-wide text-on-surface-variant uppercase' }, t(g.label), h('span', { class: 'tabular-nums' }, String(items.length))),
        h('ul', { class: 'm-0 flex list-none flex-col gap-2 p-0' }, ...items.map((n) => this.row(n))));
    });
    sheet.replaceChildren(head, h('div', { class: 'flex min-h-0 flex-1 flex-col gap-5 overflow-y-auto px-6 pb-6' },
      h('div', { class: 'flex flex-wrap items-center gap-2' }, h('div', { class: 'flex flex-1 flex-wrap gap-2', role: 'radiogroup', 'aria-label': t('filterBySeverity') }, ...chips), readAll),
      ...groups));
  }

  private row(n: Notice): HTMLElement {
    const tone = TONE[n.severity];
    const body: Child[] = [
      h('span', { class: `grid size-9 shrink-0 place-items-center rounded-full ${tone.circle} [&_.ic]:size-5` }, icon(tone.icon)),
      h('span', { class: 'flex min-w-0 flex-1 flex-col gap-1' },
        h('span', { class: 'text-sm font-medium text-on-surface' }, n.title),
        n.detail !== undefined ? h('span', { class: 'line-clamp-3 text-xs text-on-surface-variant' }, n.detail) : null,
        n.since !== undefined ? h('span', { class: 'text-xs tabular-nums text-on-surface-variant' }, ago(n.since)) : null),
      n.link !== undefined ? h('span', { class: 'mt-2 text-on-surface-variant' }, icon('right')) : null,
    ];
    const main = n.link !== undefined
      ? h('a', { href: n.link, class: 'state-layer flex min-w-0 flex-1 items-start gap-3 rounded-md3-l p-3 text-left no-underline' }, ...body)
      : h('div', { class: 'flex min-w-0 flex-1 items-start gap-3 p-3' }, ...body);
    const read = h('button', { type: 'button', class: 'state-layer mt-2.5 mr-2 grid size-9 shrink-0 place-items-center rounded-full text-on-surface-variant [&_.ic]:size-5', 'aria-label': t('markRead'), title: t('markRead') }, icon('check'));
    read.addEventListener('click', () => void this.mark([n.id]));
    return h('li', { class: 'flex items-start rounded-md3-l bg-surface-lowest' }, main, read);
  }
}
