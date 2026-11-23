/**
 * The class recipes every page draws with: claudication's primitives
 * (button, surface, table, field, chip), as cafard writes them
 * (web/src/shared/ui.ts there), plus what a calendar needs (navigation rail
 * and bar, FAB, list items). Colours are the shared design's tokens
 * (src/shared/design/theme.css): bg-primary, text-on-surface-variant, ...
 *
 * Tailwind finds a class by reading these files, so every class is written
 * out whole here, never assembled from pieces.
 */

export type ButtonVariant = 'filled' | 'tonal' | 'outlined' | 'text' | 'danger' | 'danger-text';

const BASE_BTN =
  'state-layer inline-flex items-center justify-center rounded-md3-full font-medium whitespace-nowrap no-underline disabled:pointer-events-none disabled:opacity-40 [&_.ic]:size-[18px]';

const BTN: Record<ButtonVariant, string> = {
  filled: 'bg-primary text-on-primary',
  tonal: 'bg-secondary-container text-on-secondary-container',
  outlined: 'border border-outline text-primary',
  text: 'text-primary',
  danger: 'bg-error text-on-error',
  'danger-text': 'border border-outline-variant text-error',
};

const SIZE = { md: 'h-10 gap-2 px-5 text-sm', sm: 'h-8 shrink-0 gap-1.5 px-3 text-[13px]' } as const;

export const ui = {
  /** A button, or a link drawn as one. */
  btn(variant: ButtonVariant = 'tonal', size: 'md' | 'sm' = 'md'): string {
    return `${BASE_BTN} ${SIZE[size]} ${BTN[variant]}`;
  },
  /** A round button holding one icon. */
  iconBtn: 'state-layer grid size-10 shrink-0 place-items-center rounded-full text-on-surface-variant',
  /** MD3's floating action button, extended (an icon and a label). */
  fab: 'state-layer fixed right-4 bottom-24 z-30 inline-flex h-14 items-center gap-3 rounded-md3-l bg-primary-container px-5 font-semibold text-on-primary-container elevation-2 md:relative md:inset-auto md:h-12 md:rounded-md3-l [&_.ic]:size-6',

  /** A card: a tonal surface, its title and actions on one line. */
  card: 'flex min-w-0 flex-col gap-3 rounded-md3-xl bg-surface-container p-4 lg:px-5 lg:pb-5 [&>button]:self-start',
  cardHead: 'flex min-h-9 flex-wrap items-center gap-x-4 gap-y-2',
  cardTitle: 'mr-auto text-lg leading-7 font-normal text-on-surface',
  cardAside: 'flex flex-wrap items-center gap-2',
  /** Cards side by side, as many as the width takes. */
  cards: 'grid items-start gap-4 [grid-template-columns:repeat(auto-fit,minmax(min(26rem,100%),1fr))]',
  /** The ground inside a card where rows sit. */
  well: 'overflow-hidden rounded-md3-m bg-surface-lowest',

  /** A table on its card. */
  tableWrap: 'overflow-x-auto rounded-md3-m bg-surface-lowest',
  table: 'table-stack w-full border-collapse text-sm',
  th: 'px-3 py-2.5 text-left text-[11px] font-semibold tracking-wider text-on-surface-variant uppercase first:pl-4 last:pr-4',
  td: 'px-3 py-2 align-middle first:pl-4 last:pr-4',
  tr: 'border-b border-outline-variant/50 last:border-0',

  /** A list item: leading thing, text, trailing action (MD3 list). */
  listItem: 'flex min-h-14 items-center gap-3 border-b border-outline-variant/50 px-4 py-2.5 last:border-0',
  listText: 'flex min-w-0 flex-1 flex-col [&_b]:truncate [&_b]:font-semibold [&_span]:text-[12.5px] [&_span]:text-on-surface-variant',

  /** A field's label and its control. */
  field: 'flex flex-col gap-1.5',
  label: 'text-xs font-semibold text-on-surface-variant',
  input:
    'w-full min-h-10 rounded-md3-s border border-outline-variant bg-surface-lowest px-3 py-2 text-sm text-on-surface outline-none focus:border-primary aria-invalid:border-error',
  textarea:
    'w-full min-h-20 resize-y rounded-md3-s border border-outline-variant bg-surface-lowest px-3 py-2 text-sm leading-snug text-on-surface outline-none focus:border-primary',
  check: 'inline-flex items-center gap-2 text-sm',

  /** Segmented buttons; the pressed one is aria-pressed. */
  seg: 'inline-flex shrink-0 overflow-hidden rounded-md3-full border border-outline-variant bg-surface-lowest',
  segBtn:
    'state-layer h-[34px] border-r border-outline-variant px-3.5 text-[13px] font-medium last:border-0 aria-pressed:bg-secondary-container aria-pressed:text-on-secondary-container',

  /** Small labels. */
  chip: 'inline-flex h-6 items-center gap-1 rounded-md3-s bg-surface-high px-2 text-xs font-semibold text-on-surface no-underline',
  pill: {
    ok: 'inline-flex h-5 items-center rounded-full bg-success-container px-2 text-[11.5px] font-semibold whitespace-nowrap text-on-success-container',
    warn: 'inline-flex h-5 items-center rounded-full bg-warning-container px-2 text-[11.5px] font-semibold whitespace-nowrap text-on-warning-container',
    err: 'inline-flex h-5 items-center rounded-full bg-error-container px-2 text-[11.5px] font-semibold whitespace-nowrap text-on-error-container',
    muted: 'inline-flex h-5 items-center rounded-full bg-surface-high px-2 text-[11.5px] font-semibold whitespace-nowrap text-on-surface-variant',
  },

  /** Term and value pairs. */
  kv: 'grid grid-cols-[auto_minmax(0,1fr)] items-baseline gap-x-4 gap-y-1.5 text-sm [&_dd]:m-0 [&_dd]:break-all [&_dt]:text-on-surface-variant',
  link: 'font-semibold text-primary no-underline hover:underline',
  dim: 'text-on-surface-variant',
  empty: 'p-5 text-center text-on-surface-variant',
  /** A message across the page. */
  notice: 'flex items-center gap-3 rounded-md3-m bg-warning-container px-4 py-2.5 text-on-warning-container [&_.ic]:text-warning',
  error: 'flex items-center gap-3 rounded-md3-m bg-error-container px-4 py-2.5 text-on-error-container',
  info: 'flex items-center gap-3 rounded-md3-m bg-secondary-container px-4 py-2.5 text-on-secondary-container',
  /** A secret shown once, to copy. */
  secret: 'block rounded-md3-s bg-surface-lowest px-3 py-2 font-mono text-[13px] break-all select-all',

  /** What a dialog is made of (MD3 dialog). */
  dialog:
    'm-auto w-[min(560px,calc(100%-24px))] max-h-[calc(100%-24px)] overflow-hidden rounded-md3-xl bg-surface-high p-0 text-on-surface elevation-2',
  dialogHead: 'px-6 pt-6 pb-2 [&_h2]:text-2xl [&_h2]:font-normal',
  dialogBody: 'flex max-h-[65vh] flex-col gap-4 overflow-y-auto px-6 py-3 text-on-surface-variant',
  dialogFoot: 'flex flex-wrap justify-end gap-2 px-6 pt-2 pb-5',

  /** The snackbar. */
  toast: 'pointer-events-auto max-w-[calc(100vw-32px)] rounded-md3-s bg-on-surface px-4 py-3 text-sm text-surface elevation-2 [animation:pop_200ms_cubic-bezier(.2,0,0,1)]',
  toastError: 'pointer-events-auto max-w-[calc(100vw-32px)] rounded-md3-s bg-error px-4 py-3 text-sm text-on-error elevation-2 [animation:pop_200ms_cubic-bezier(.2,0,0,1)]',
} as const;
