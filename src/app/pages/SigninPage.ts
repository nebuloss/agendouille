import { Countdown } from '@/shared/components/Countdown.ts';
import { Backoff } from '@/shared/core/Backoff.ts';
import { Dom } from '@/shared/core/Dom.ts';
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
  private countdown: Countdown | null = null;
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
    // The code lives two minutes; at zero a fresh one takes its place.
    const timer = this.doc.getElementById('signin-timer');
    const left = timer === null ? null : Dom.number(timer, 'data-remaining-ms');
    const ttl = timer === null ? null : Dom.number(timer, 'data-ttl-ms');
    if (timer !== null && left !== null && ttl !== null) {
      this.countdown = new Countdown(timer, left, ttl, () => {
        if (!this.stopped) this.doc.location.assign('/');
      });
    }
    void this.loop(action);
  }

  dispose(): void {
    this.stopped = true;
    this.countdown?.dispose();
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
        this.doc.location.assign('/');
        return;
      }
    }
  }
}
