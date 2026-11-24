/**
 * Choosing people or groups in a modal: a search bar over a list of
 * rows (a mark, a name, a line under it), in place of a long <select>.
 * One choice closes it at once; several are ticked, then confirmed.
 * Optional controls (an access level, "keep as a member") sit under the
 * list and are read by the caller once it resolves.
 */
import { h, type Child } from '@/shared/core/Dom.ts';
import { icon } from '@/shared/components/icons.ts';
import { ui } from '@/shared/components/recipes.ts';
import { t } from '@/shared/components/Picker.msg.ts';

export interface PickItem {
  readonly value: string;
  readonly label: string;
  readonly sub?: string;
  readonly lead?: Node;
  /** A heading the item is listed under ("Groupes", "Utilisateurs"). */
  readonly section?: string;
}

export interface PickOptions {
  readonly title: string;
  readonly items: readonly PickItem[];
  readonly multiple?: boolean;
  /** The confirm button's label, for several choices ("Ajouter"). */
  readonly confirm?: string;
  /** Controls under the list (read by the caller after it resolves). */
  readonly extra?: readonly Child[];
  readonly empty?: string;
  /** Chosen when it opens. */
  readonly selected?: readonly string[];
  /** For several choices: confirming none is a choice too (taking everyone off). */
  readonly allowNone?: boolean;
}

/** Folds case and accents, so "lea" finds "Léa". */
export function fold(s: string): string {
  return s.normalize('NFD').replace(/\p{M}/gu, '').toLowerCase();
}

/**
 * Opens the picker. Resolves with the values chosen (one for a single
 * choice), or null when cancelled.
 */
export function pick(doc: Document, o: PickOptions): Promise<string[] | null> {
  const el = doc.createElement('dialog');
  el.className = ui.dialog;
  const chosen = new Set(o.selected ?? []);
  const search = h('input', { type: 'search', placeholder: t('search'), 'aria-label': t('search'), autocomplete: 'off',
    class: 'min-h-11 w-full rounded-md3-full border border-outline-variant bg-surface-lowest py-2 pr-4 pl-11 text-sm outline-none focus:border-primary focus-visible:outline-none' });
  const list = h('div', { role: 'listbox', 'aria-multiselectable': o.multiple ? 'true' : 'false',
    class: 'flex max-h-[45vh] min-h-48 flex-col overflow-y-auto rounded-md3-l bg-surface-lowest p-1' });
  const count = h('span', { class: 'mr-auto self-center text-sm text-on-surface-variant' });
  const ok = h('button', { type: 'button', class: ui.btn('filled') }, o.confirm ?? t('choose'));
  const cancel = h('button', { type: 'button', class: ui.btn('text') }, t('cancel'));
  let result: string[] | null = null;

  const rows = o.items.map((it) => {
    const tick = h('span', { class: 'grid size-6 shrink-0 place-items-center rounded-full border-2 border-outline text-on-primary [&_.ic]:size-4' });
    const row = h('button', { type: 'button', role: 'option', 'aria-selected': 'false', 'data-value': it.value,
      class: 'state-layer flex w-full items-center gap-3 rounded-md3-m px-3 py-2 text-left aria-selected:bg-secondary-container aria-selected:text-on-secondary-container' },
    it.lead ?? null,
    h('span', { class: 'flex min-w-0 flex-1 flex-col' }, h('span', { class: 'truncate font-semibold' }, it.label),
      it.sub !== undefined && it.sub !== '' ? h('span', { class: 'truncate text-xs text-on-surface-variant' }, it.sub) : null),
    o.multiple ? tick : null);
    const draw = (): void => {
      const on = chosen.has(it.value);
      row.setAttribute('aria-selected', String(on));
      tick.className = on
        ? 'grid size-6 shrink-0 place-items-center rounded-full border-2 border-primary bg-primary text-on-primary [&_.ic]:size-4'
        : 'grid size-6 shrink-0 place-items-center rounded-full border-2 border-outline text-on-primary [&_.ic]:size-4';
      tick.replaceChildren(on ? icon('check') : '');
    };
    row.addEventListener('click', () => {
      if (!o.multiple) {
        result = [it.value];
        el.close();
        return;
      }
      if (chosen.has(it.value)) chosen.delete(it.value);
      else chosen.add(it.value);
      draw();
      update();
    });
    draw();
    return { it, row, text: fold(`${it.label} ${it.sub ?? ''} ${it.value}`) };
  });
  const emptyLine = h('p', { class: 'px-3 py-8 text-center text-sm text-on-surface-variant' });

  const render = (): void => {
    const words = fold(search.value).split(/\s+/).filter(Boolean);
    const shown = rows.filter((r) => words.every((w) => r.text.includes(w)));
    const out: Node[] = [];
    let section: string | undefined;
    for (const r of shown) {
      if (r.it.section !== undefined && r.it.section !== section) {
        section = r.it.section;
        out.push(h('p', { class: 'px-3 pt-3 pb-1 text-xs font-semibold tracking-wide text-on-surface-variant uppercase' }, section));
      }
      out.push(r.row);
    }
    if (shown.length === 0) {
      emptyLine.textContent = o.items.length === 0 ? (o.empty ?? t('nothing')) : t('noMatch', { query: search.value.trim() });
      out.push(emptyLine);
    }
    list.replaceChildren(...out);
  };
  const update = (): void => {
    count.textContent = o.multiple ? t('selected', { count: chosen.size }) : '';
    ok.disabled = o.multiple === true && chosen.size === 0 && o.allowNone !== true;
  };

  search.addEventListener('input', render);
  // Enter picks the only match, or the first one for a single choice.
  search.addEventListener('keydown', (e) => {
    if (e.key !== 'Enter') return;
    e.preventDefault();
    const first = list.querySelector<HTMLButtonElement>('[role="option"]');
    first?.click();
  });
  ok.addEventListener('click', () => {
    result = [...chosen];
    el.close();
  });
  cancel.addEventListener('click', () => el.close());
  el.addEventListener('click', (e) => {
    if (e.target === el) el.close();
  });

  el.append(h('div', { class: 'flex max-h-[inherit] flex-col' },
    h('div', { class: ui.dialogHead }, h('h2', {}, o.title)),
    h('div', { class: ui.dialogBody },
      h('div', { class: 'relative' }, h('span', { class: 'pointer-events-none absolute top-1/2 left-4 -translate-y-1/2 text-on-surface-variant' }, icon('search')), search),
      list, ...(o.extra ?? [])),
    h('div', { class: ui.dialogFoot }, count, cancel, o.multiple ? ok : null)));
  render();
  update();
  doc.body.append(el);
  return new Promise((resolve) => {
    el.addEventListener('close', () => {
      el.remove();
      resolve(result);
    }, { once: true });
    // showModal focuses the first field, the search: on a touchscreen that
    // brings the keyboard up over half the list before anyone asked for
    // it, so the dialog itself holds the focus there (Chromium ignores
    // autofocus on the dialog); with a pointer, the search keeps it.
    el.tabIndex = -1;
    if (typeof el.showModal === 'function') el.showModal();
    else el.setAttribute('open', '');
    if (doc.defaultView?.matchMedia?.('(pointer: coarse)').matches ?? false) el.focus();
    else search.focus();
  });
}
