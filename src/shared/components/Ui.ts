import { currentTheme, nextTheme, setTheme, type Theme } from '@/shared/core/Theme.ts';
import { LANGS, lang, setLang, type Lang } from '@/shared/i18n/i18n.ts';
import { t } from '@/shared/components/Ui.msg.ts';
import { flag } from '@/shared/components/flags.ts';
import { h, type Child } from '@/shared/core/Dom.ts';
import { icon, type IconName } from '@/shared/components/icons.ts';
import { ui, type ButtonVariant } from '@/shared/components/recipes.ts';

/** Snackbars in #toasts; at most three; each goes after five seconds. */
export class Toasts {
  constructor(
    private readonly root: HTMLElement | null,
    private readonly ms = 5000,
  ) {}

  show(text: string, tone: 'info' | 'error' = 'info'): void {
    if (this.root === null) return;
    const t = h('div', { class: tone === 'error' ? ui.toastError : ui.toast, role: tone === 'error' ? 'alert' : 'status' }, text);
    this.root.append(t);
    while (this.root.children.length > 3) this.root.firstElementChild?.remove();
    setTimeout(() => t.remove(), this.ms);
  }

  /** Shows an error's message, the server's own sentence when there is one. */
  error(e: unknown): void {
    this.show(e instanceof Error ? e.message : String(e), 'error');
  }
}

export interface DialogButton {
  readonly label: string;
  readonly value: string;
  readonly kind?: ButtonVariant;
}

/**
 * A modal <dialog> (MD3 dialog). Resolves with the value of the button
 * pressed, or "" when it is closed otherwise (Escape, the scrim).
 */
export function dialog(doc: Document, title: string, body: Child[], buttons: DialogButton[]): Promise<string> {
  const el = doc.createElement('dialog');
  el.className = ui.dialog;
  const form = h('form', { method: 'dialog', class: 'flex max-h-[inherit] flex-col' });
  const row = h('div', { class: ui.dialogFoot });
  for (const b of buttons) {
    // Only the confirming button checks the form: Annuler with an empty
    // required title must still close the dialog.
    const confirms = b.kind === 'filled' || b.kind === 'danger';
    row.append(h('button', { class: ui.btn(b.kind ?? 'text'), value: b.value, type: 'submit', formnovalidate: !confirms }, b.label));
  }
  form.append(h('div', { class: ui.dialogHead }, h('h2', {}, title)), h('div', { class: ui.dialogBody }, ...body), row);
  el.append(form);
  // A click on the scrim (the dialog element itself, outside the form) closes.
  el.addEventListener('click', (e) => {
    if (e.target === el) el.close('');
  });
  doc.body.append(el);
  return new Promise((resolve) => {
    el.addEventListener(
      'close',
      () => {
        resolve(el.returnValue);
        el.remove();
      },
      { once: true },
    );
    if (typeof el.showModal === 'function') el.showModal();
    else el.setAttribute('open', '');
  });
}

/**
 * A form in a modal: it stays open while the server says no, with the
 * reason inside it, and closes once submit has worked. Resolves true when
 * it did, false when the form was cancelled. The browser checks the
 * fields first (required, pattern).
 */
export function formDialog(doc: Document, title: string, body: Child[], submitLabel: string, submit: () => Promise<void>, kind: ButtonVariant = 'filled'): Promise<boolean> {
  const el = doc.createElement('dialog');
  el.className = ui.dialog;
  const error = h('p', { class: ui.error, role: 'alert', hidden: true });
  const cancel = h('button', { class: ui.btn('text'), type: 'button' }, t('cancel'));
  const ok = h('button', { class: ui.btn(kind), type: 'submit' }, submitLabel);
  const form = h('form', { class: 'flex max-h-[inherit] flex-col', novalidate: false },
    h('div', { class: ui.dialogHead }, h('h2', {}, title)),
    h('div', { class: ui.dialogBody }, ...body, error),
    h('div', { class: ui.dialogFoot }, cancel, ok));
  el.append(form);
  doc.body.append(el);
  return new Promise((resolve) => {
    let done = false;
    el.addEventListener('close', () => {
      el.remove();
      resolve(done);
    }, { once: true });
    cancel.addEventListener('click', () => el.close());
    el.addEventListener('click', (e) => {
      if (e.target === el) el.close();
    });
    form.addEventListener('submit', (e) => {
      e.preventDefault();
      ok.disabled = true;
      error.hidden = true;
      submit().then(() => {
        done = true;
        el.close();
      }, (err: unknown) => {
        error.textContent = err instanceof Error ? err.message : String(err);
        error.hidden = false;
      }).finally(() => {
        ok.disabled = false;
      });
    });
    if (typeof el.showModal === 'function') el.showModal();
    else el.setAttribute('open', '');
    const first = form.querySelector<HTMLElement>('input, select, textarea');
    first?.focus();
  });
}

/** Asks yes or no. */
export async function confirmDialog(doc: Document, title: string, text: string, yes: string, danger = false): Promise<boolean> {
  const v = await dialog(doc, title, [h('p', {}, text)], [
    { label: t('cancel'), value: 'no' },
    { label: yes, value: 'yes', kind: danger ? 'danger' : 'filled' },
  ]);
  return v === 'yes';
}

/** A labelled field. Inputs, selects and textareas get the field style. */
export function field(label: string, input: HTMLElement, hint?: string): HTMLElement {
  styleControl(input);
  return h('label', { class: ui.field }, h('span', { class: ui.label }, label), input, hint ? h('span', { class: 'text-xs text-on-surface-variant' }, hint) : null);
}

/** Gives a bare input, select or textarea the design's look. */
export function styleControl<E extends HTMLElement>(input: E): E {
  if (input.className !== '') return input;
  const type = input.getAttribute('type') ?? '';
  if (input.tagName === 'TEXTAREA') input.className = ui.textarea;
  else if (input.tagName === 'SELECT' || (input.tagName === 'INPUT' && !['checkbox', 'radio', 'color', 'file'].includes(type))) input.className = ui.input;
  else if (type === 'color') input.className = 'h-10 w-16 rounded-md3-s border border-outline-variant bg-surface-lowest';
  else if (type === 'file') input.className = 'text-sm file:mr-3 file:h-9 file:rounded-md3-full file:border-0 file:bg-secondary-container file:px-4 file:font-medium file:text-on-secondary-container';
  return input;
}

/** A checkbox and its label on one line. */
export function check(box: HTMLInputElement, ...label: Child[]): HTMLElement {
  return h('label', { class: ui.check }, box, ...label);
}

/**
 * A table on its card; rows are lists of cells. Below sm it stacks into
 * cards, each cell under its column's heading (theme.css's table-stack).
 */
export function table(head: string[], rows: Child[][], empty = t('empty')): HTMLElement {
  if (rows.length === 0) return h('p', { class: ui.empty }, empty);
  const t = h('table', { class: ui.table });
  t.append(h('thead', {}, h('tr', { class: ui.tr }, ...head.map((c) => h('th', { scope: 'col', class: ui.th }, c)))));
  const tbody = h('tbody', {});
  for (const r of rows) {
    tbody.append(h('tr', { class: ui.tr }, ...r.map((c, i) => h('td', { class: ui.td, 'data-label': head[i] || null }, c))));
  }
  t.append(tbody);
  return h('div', { class: ui.tableWrap }, t);
}

/** A round swatch of a colour; the colour goes through CSSOM, never markup. */
export function swatch(color: string, size: 'sm' | 'md' = 'sm'): HTMLElement {
  const s = h('span', { class: size === 'sm' ? 'mr-1.5 inline-block size-3.5 shrink-0 rounded-full bg-outline align-[-2px]' : 'inline-block size-6 shrink-0 rounded-full bg-outline', 'aria-hidden': 'true' });
  if (/^#[0-9a-f]{6}$/i.test(color)) s.style.setProperty('background-color', color);
  return s;
}

/** A button that runs an action, disabled while it runs. */
export function action(label: string, run: () => Promise<void>, kind: ButtonVariant = 'tonal', ic?: IconName, size: 'md' | 'sm' = 'md'): HTMLButtonElement {
  const b = h('button', { class: ui.btn(kind, size), type: 'button' }, ic ? icon(ic) : null, label);
  b.addEventListener('click', () => {
    b.disabled = true;
    run().finally(() => {
      b.disabled = false;
    });
  });
  return b;
}

/** A round button with one icon and a label for screen readers. */
export function iconButton(ic: IconName, label: string, run: () => void): HTMLButtonElement {
  const b = h('button', { class: ui.iconBtn, type: 'button', 'aria-label': label, title: label }, icon(ic));
  b.addEventListener('click', run);
  return b;
}

/** A card with a heading, actions beside it, then its content. */
export function section(title: string, ...children: Child[]): HTMLElement {
  return card(title, [], ...children);
}

export function card(title: string, aside: Child[], ...children: Child[]): HTMLElement {
  return h('section', { class: ui.card },
    h('div', { class: ui.cardHead }, h('h2', { class: ui.cardTitle }, title), aside.length > 0 ? h('div', { class: ui.cardAside }, ...aside) : null),
    ...children);
}

/**
 * A secret shown once (a device password, a subscription address): masked
 * until asked, with a button to show it and one to copy it, so it is not
 * on the screen for whoever looks over a shoulder.
 */
export function secretField(value: string, label: string): HTMLElement {
  const text = h('code', { class: 'min-w-0 flex-1 font-mono text-[13px] break-all select-all', 'aria-label': label }, '•'.repeat(16));
  let shown = false;
  const toggle = h('button', { type: 'button', class: ui.btn('text', 'sm'), 'aria-pressed': 'false' }, icon('show'), t('show'));
  toggle.addEventListener('click', () => {
    shown = !shown;
    text.textContent = shown ? value : '•'.repeat(16);
    toggle.setAttribute('aria-pressed', String(shown));
    toggle.replaceChildren(icon(shown ? 'hide' : 'show'), shown ? t('hide') : t('show'));
  });
  const copy = h('button', { type: 'button', class: ui.btn('tonal', 'sm') }, icon('link'), t('copy'));
  copy.addEventListener('click', () => {
    const done = (ok: boolean): void => {
      copy.replaceChildren(icon(ok ? 'check' : 'warn'), ok ? t('copied') : t('copyFailed'));
      setTimeout(() => copy.replaceChildren(icon('link'), t('copy')), 2000);
    };
    const clip = (globalThis.navigator as Navigator | undefined)?.clipboard;
    if (clip === undefined) {
      done(false);
      return;
    }
    clip.writeText(value).then(() => done(true), () => done(false));
  });
  return h('div', { class: 'flex flex-wrap items-center gap-2 rounded-md3-s bg-surface-lowest px-3 py-2' }, text, toggle, copy);
}

/**
 * Appearance: follow the system, light or dark (src/shared/core/Theme.ts).
 * Three segments; the choice applies at once and is remembered.
 */
export function themeChoice(doc: Document = document): HTMLElement {
  const choices: [Theme, string, IconName][] = [['auto', t('themeAuto'), 'contrast'], ['light', t('themeLight'), 'sun'], ['dark', t('themeDark'), 'moon']];
  const group = h('div', { class: ui.seg, role: 'radiogroup', 'aria-label': t('appearance') });
  const draw = (): void => {
    const now = currentTheme(doc);
    group.replaceChildren(...choices.map(([value, label, ic]) => {
      const b = h('button', { type: 'button', role: 'radio', 'aria-checked': String(now === value),
        class: 'state-layer inline-flex items-center gap-1.5 px-3.5 py-2 text-sm aria-checked:bg-secondary-container aria-checked:text-on-secondary-container' }, icon(ic), label);
      b.addEventListener('click', () => {
        setTheme(value, doc);
        draw();
      });
      return b;
    }));
  };
  draw();
  return group;
}

/** The theme as one round button that cycles (the admin's header). */
export function themeButton(doc: Document = document): HTMLElement {
  const LABEL: Record<Theme, string> = { auto: t('themeIsAuto'), light: t('themeIsLight'), dark: t('themeIsDark') };
  const ICON: Record<Theme, IconName> = { auto: 'contrast', light: 'sun', dark: 'moon' };
  const b = h('button', { type: 'button', class: ui.iconBtn });
  const draw = (): void => {
    const t = currentTheme(doc);
    b.replaceChildren(icon(ICON[t]));
    b.title = LABEL[t];
    b.setAttribute('aria-label', LABEL[t]);
  };
  b.addEventListener('click', () => {
    setTheme(nextTheme(currentTheme(doc)), doc);
    draw();
  });
  draw();
  return b;
}

/** The language, as a select; choosing one reloads the page in it. */
export function langChoice(doc: Document = document): HTMLElement {
  const current = lang(doc);
  const select = h('select', { 'aria-label': t('language'), class: ui.input },
    ...(Object.keys(LANGS) as Lang[]).map((l) => h('option', { value: l, selected: l === current, lang: l }, LANGS[l])));
  select.addEventListener('change', () => setLang(select.value as Lang, doc));
  select.classList.add('!w-auto');
  return select;
}

/**
 * The language as a small dropdown (both headers, the sign-in pages): the
 * current flag and code, then a list of the languages, each with its flag
 * and its own name. Choosing one reloads the page in it.
 */
export function langMenu(doc: Document = document): HTMLElement {
  const current = lang(doc);
  const wrap = h('div', { class: 'relative' });
  const button = h('button', { type: 'button', 'aria-haspopup': 'menu', 'aria-expanded': 'false', title: t('language'), 'aria-label': `${t('language')} : ${LANGS[current]}`,
    class: 'state-layer inline-flex h-9 items-center gap-1.5 rounded-md3-full border border-outline-variant px-2.5 text-[13px] font-semibold text-on-surface-variant' },
  flag(current, 13, doc), current.toUpperCase(), icon('chevronDown'));
  button.querySelector('.ic')?.classList.add('!size-4');
  const menu = h('div', { role: 'menu', hidden: true,
    class: 'absolute right-0 z-50 mt-1.5 flex min-w-44 flex-col rounded-md3-l bg-surface-high p-1.5 text-on-surface elevation-2' },
  ...(Object.keys(LANGS) as Lang[]).map((l) => {
    const item = h('button', { type: 'button', role: 'menuitemradio', 'aria-checked': String(l === current), lang: l,
      class: 'state-layer flex items-center gap-3 rounded-md3-m px-3 py-2 text-left text-sm aria-checked:font-semibold' },
    flag(l, 15, doc), h('span', { class: 'flex-1' }, LANGS[l]), l === current ? icon('check') : null);
    item.addEventListener('click', () => {
      close();
      if (l !== current) setLang(l, doc);
    });
    return item;
  }));
  const close = (): void => {
    menu.hidden = true;
    button.setAttribute('aria-expanded', 'false');
  };
  button.addEventListener('click', () => {
    menu.hidden = !menu.hidden;
    button.setAttribute('aria-expanded', String(!menu.hidden));
    if (!menu.hidden) menu.querySelector<HTMLButtonElement>('[aria-checked="true"]')?.focus();
  });
  doc.addEventListener('click', (e) => {
    if (!wrap.contains(e.target as Node)) close();
  });
  wrap.addEventListener('keydown', (e) => {
    if (e.key === 'Escape' && !menu.hidden) {
      close();
      button.focus();
    }
  });
  wrap.append(button, menu);
  return wrap;
}
