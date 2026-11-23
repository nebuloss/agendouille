import type { Api } from '@/shared/core/Api.ts';
import { DisposableBag } from '@/shared/core/Disposable.ts';
import { h, type Child } from '@/shared/core/Dom.ts';
import type { Page } from '@/shared/core/Pages.ts';
import { Toasts } from '@/shared/components/Ui.ts';
import { ui } from '@/shared/components/recipes.ts';

/**
 * A page the server rendered as a shell (navigation, title, a mount
 * point): the page fills the mount point from the API and draws itself
 * again after every change. The admin UI and the family web app both use it.
 */
export abstract class ApiPage implements Page {
  protected readonly bag = new DisposableBag();
  protected readonly root: HTMLElement;
  protected readonly toasts: Toasts;
  private readonly title: HTMLElement | null;

  constructor(
    protected readonly doc: Document,
    protected readonly api: Api,
    mount: string,
  ) {
    this.root = doc.getElementById(mount) ?? doc.body;
    this.title = this.root.querySelector('h1');
    this.toasts = new Toasts(doc.getElementById('toasts'));
  }

  start(): void {
    void this.refresh();
  }

  dispose(): void {
    this.bag.dispose();
  }

  /** Loads and draws again. */
  async refresh(): Promise<void> {
    try {
      const nodes = await this.render();
      this.show(nodes);
    } catch (e) {
      this.show([h('p', { class: ui.error, role: 'alert' }, e instanceof Error ? e.message : String(e))]);
    }
  }

  private show(nodes: Child[]): void {
    while (this.root.lastChild !== null && this.root.lastChild !== this.title) this.root.lastChild.remove();
    if (this.title === null || this.title.parentNode !== this.root) this.root.replaceChildren();
    for (const n of nodes) if (n !== null && n !== undefined && n !== false) this.root.append(n);
  }

  /** Runs an action, shows its error, and draws again when it worked. */
  protected async act(run: () => Promise<unknown>, done?: string): Promise<void> {
    try {
      await run();
      if (done !== undefined) this.toasts.show(done);
      // What else on the page depends on the state (the admin's bell) reads again.
      this.doc.dispatchEvent(new Event('agd:changed'));
      await this.refresh();
    } catch (e) {
      this.toasts.error(e);
    }
  }

  protected abstract render(): Promise<Child[]>;
}
