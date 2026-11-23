import type { Disposable } from '@/shared/core/Disposable.ts';

/** One page's code, mounted on the document the server rendered. */
export interface Page extends Disposable {
  start(): void;
}

export type PageFactory = (doc: Document) => Page;

/**
 * Picks the page class from `<body data-page="...">`, which the Go layout
 * sets. A page id the bundle does not know mounts nothing: the server's
 * HTML is complete on its own (PLAN.md 9.2), so an old bundle facing a new
 * page degrades instead of breaking.
 */
export class Pages {
  private current: Page | null = null;

  constructor(private readonly factories: Readonly<Record<string, PageFactory>>) {}

  /** The id the document asks for, or "" when it names none. */
  static idOf(doc: Document): string {
    return doc.body?.dataset['page']?.trim() ?? '';
  }

  /** Mounts the page the document names, replacing the one before. */
  mount(doc: Document): Page | null {
    this.current?.dispose();
    this.current = null;
    const id = Pages.idOf(doc);
    const factory = Object.hasOwn(this.factories, id) ? this.factories[id] : undefined;
    if (factory === undefined) return null;
    const page = factory(doc);
    page.start();
    this.current = page;
    return page;
  }
}

/** Runs start once the document is parsed. */
export function whenReady(doc: Document, start: () => void): void {
  if (doc.readyState === 'loading') {
    doc.addEventListener('DOMContentLoaded', start, { once: true });
  } else {
    start();
  }
}
