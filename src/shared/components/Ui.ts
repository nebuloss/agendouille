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
  for (const b of buttons) row.append(h('button', { class: ui.btn(b.kind ?? 'text'), value: b.value, type: 'submit' }, b.label));
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

/** Asks yes or no. */
export async function confirmDialog(doc: Document, title: string, text: string, yes: string, danger = false): Promise<boolean> {
  const v = await dialog(doc, title, [h('p', {}, text)], [
    { label: 'Annuler', value: 'no' },
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
export function table(head: string[], rows: Child[][], empty = 'Rien pour le moment.'): HTMLElement {
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
