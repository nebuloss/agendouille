import { h, type Child } from '@/shared/core/Dom.ts';

/** Snackbars in #toasts; at most three; each goes after five seconds. */
export class Toasts {
  constructor(
    private readonly root: HTMLElement | null,
    private readonly ms = 5000,
  ) {}

  show(text: string, tone: 'info' | 'error' = 'info'): void {
    if (this.root === null) return;
    const t = h('div', { class: `toast ${tone}`, role: tone === 'error' ? 'alert' : 'status' }, text);
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
  readonly kind?: 'filled' | 'text' | 'danger';
}

/**
 * A modal <dialog>. open() resolves with the value of the button pressed,
 * or "" when it is closed otherwise (Escape).
 */
export function dialog(doc: Document, title: string, body: Child[], buttons: DialogButton[]): Promise<string> {
  const el = doc.createElement('dialog');
  el.className = 'dialog';
  const form = h('form', { method: 'dialog' });
  form.append(h('h2', {}, title), h('div', { class: 'dialog-body' }, ...body));
  const row = h('div', { class: 'dialog-actions' });
  for (const b of buttons) row.append(h('button', { class: `btn ${b.kind ?? 'text'}`, value: b.value, type: 'submit' }, b.label));
  form.append(row);
  el.append(form);
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

/** A labelled field. */
export function field(label: string, input: HTMLElement, hint?: string): HTMLElement {
  return h('label', { class: 'field' }, h('span', { class: 'field-label' }, label), input, hint ? h('span', { class: 'field-hint' }, hint) : null);
}

/** A table with a header row; rows are lists of cells. */
export function table(head: string[], rows: Child[][], empty = 'Rien pour le moment.'): HTMLElement {
  if (rows.length === 0) return h('p', { class: 'empty' }, empty);
  const t = h('table', { class: 'table' });
  const thead = h('thead', {}, h('tr', {}, ...head.map((c) => h('th', { scope: 'col' }, c))));
  const tbody = h('tbody', {});
  for (const r of rows) tbody.append(h('tr', {}, ...r.map((c) => h('td', {}, c))));
  t.append(thead, tbody);
  return h('div', { class: 'table-wrap' }, t);
}

/** A round swatch of a colour; the colour goes through CSSOM, never markup. */
export function swatch(color: string): HTMLElement {
  const s = h('span', { class: 'swatch', 'aria-hidden': 'true' });
  if (/^#[0-9a-f]{6}$/i.test(color)) s.style.setProperty('--swatch', color);
  return s;
}

/** A button that runs an action, disabled while it runs. */
export function action(label: string, run: () => Promise<void>, kind: 'filled' | 'tonal' | 'text' | 'danger' = 'tonal'): HTMLButtonElement {
  const b = h('button', { class: `btn ${kind}`, type: 'button' }, label);
  b.addEventListener('click', () => {
    b.disabled = true;
    run().finally(() => {
      b.disabled = false;
    });
  });
  return b;
}

/** A section with a heading. */
export function section(title: string, ...children: Child[]): HTMLElement {
  return h('section', { class: 'card' }, h('h2', {}, title), ...children);
}
