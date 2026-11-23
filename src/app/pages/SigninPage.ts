import { Backoff } from '@/shared/core/Backoff.ts';
import type { Page } from '@/shared/core/Pages.ts';

type WaitStatus = 'approved' | 'pending' | 'cancelled' | 'expired' | 'used' | 'unknown' | 'error';

/** What the waiting page does with each answer of /signin/wait/{id}. */
export function nextStep(status: WaitStatus): 'home' | 'again' | 'restart' {
  switch (status) {
    case 'approved':
      return 'home';
    case 'pending':
      return 'again';
    default:
      return 'restart';
  }
}

/**
 * The approval screen (PLAN.md 8.2). The server rendered the QR code, the
 * short code and the number; this waits on /signin/wait/{id}, which the
 * server holds open until someone approves, and goes home once signed in.
 * Without the bundle the page's "continue" button does the same by hand.
 */
export class SigninPage implements Page {
  private stopped = false;
  private readonly backoff = new Backoff({ baseMs: 1000, maxMs: 15000 });

  constructor(
    private readonly doc: Document,
    private readonly fetcher: typeof fetch = fetch.bind(globalThis),
  ) {}

  start(): void {
    const form = this.doc.getElementById('signin-wait');
    const action = form?.getAttribute('action') ?? '';
    if (!action.startsWith('/signin/wait/')) return;
    form?.setAttribute('hidden', '');
    void this.loop(action);
  }

  dispose(): void {
    this.stopped = true;
  }

  private async loop(url: string): Promise<void> {
    while (!this.stopped) {
      let status: WaitStatus = 'error';
      try {
        const resp = await this.fetcher(url, { credentials: 'same-origin', headers: { Accept: 'application/json' } });
        status = ((await resp.json()) as { status: WaitStatus }).status;
      } catch {
        await new Promise((r) => setTimeout(r, this.backoff.next()));
        continue;
      }
      this.backoff.reset();
      if (this.stopped) return;
      const step = nextStep(status);
      if (step === 'home') {
        this.doc.location.assign('/');
        return;
      }
      if (step === 'restart') {
        this.doc.location.assign('/signin');
        return;
      }
    }
  }
}
