import { Api } from '@/shared/core/Api.ts';
import { ApiPage } from '@/shared/components/ApiPage.ts';
import { Pages } from '@/shared/core/Pages.ts';
import { READS, liveStream } from '@/admin/live.ts';
import type { Topic } from '@/shared/core/Live.ts';

/** The admin API, at /api/admin on the admin listener. */
export function adminApi(fetchFn?: (input: string, init: RequestInit) => Promise<Response>): Api {
  return new Api('/api/admin', fetchFn);
}

/**
 * A page of the admin UI: the server rendered the shell (top bar,
 * navigation, title); the page fills #admin-content from the API and
 * draws itself again after every change.
 */
export abstract class AdminPage extends ApiPage {
  private pending = false;

  constructor(doc: Document, api: Api) {
    super(doc, api, 'admin-content');
  }

  /**
   * Reads again when a topic the page shows changed elsewhere (a phone, the
   * family app, another tab): the stream says so, with no polling. Not
   * under a hand: while a dialog is open or a field has the focus, it waits
   * for them to close.
   */
  override start(): void {
    super.start();
    const id = Pages.idOf(this.doc) as keyof typeof READS;
    const topics: readonly Topic[] = READS[id] ?? [];
    if (topics.length === 0) return;
    this.bag.defer(liveStream.on(topics, () => this.changedElsewhere()));
  }

  private changedElsewhere(): void {
    const busy = this.doc.querySelector('dialog[open]') !== null
      || (this.doc.activeElement instanceof HTMLElement && this.doc.activeElement.matches('input, textarea, select, [contenteditable]'));
    if (!busy) {
      this.pending = false;
      void this.refresh();
      return;
    }
    if (this.pending) return;
    this.pending = true;
    const again = (): void => {
      if (!this.pending || this.bag.isDisposed) return;
      this.doc.defaultView?.setTimeout(() => this.changedElsewhere(), 500);
    };
    this.doc.addEventListener('close', again, { capture: true, once: true });
    this.doc.addEventListener('focusout', again, { once: true });
  }

  /** The {name} of the page's path (a user's page). */
  protected get name(): string {
    return this.root.dataset['name'] ?? '';
  }
}
