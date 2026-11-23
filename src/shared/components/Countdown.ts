import { h } from '@/shared/core/Dom.ts';
import type { Disposable } from '@/shared/core/Disposable.ts';

/**
 * A QR code's life, shown like an authenticator app's: a bar that drains,
 * "Nouveau code dans 1:23", red for the last fifth, and onDone once at
 * zero, where the page puts a fresh code in its place.
 *
 * It counts from what the server says is left (remainingMs), on this
 * page's monotonic clock, so a phone whose clock is wrong still shows the
 * right time.
 */
export class Countdown implements Disposable {
  private readonly bar: HTMLElement;
  private readonly label: HTMLElement;
  private readonly started: number;
  private timer: ReturnType<typeof setInterval> | null = null;
  private done = false;

  constructor(
    readonly root: HTMLElement,
    private readonly remainingMs: number,
    private readonly ttlMs: number,
    private readonly onDone: () => void,
    private readonly now: () => number = () => performance.now(),
  ) {
    this.bar = h('i', { class: 'block h-full rounded-full bg-primary transition-[width,background-color] duration-300 ease-linear data-[low]:bg-error' });
    this.label = h('p', { class: 'text-center text-xs tabular-nums text-on-surface-variant', 'aria-live': 'off' });
    root.replaceChildren(
      h('div', { class: 'h-1.5 overflow-hidden rounded-full bg-surface-highest', role: 'progressbar', 'aria-label': 'Temps restant pour ce code' }, this.bar),
      this.label,
    );
    root.classList.add('flex', 'flex-col', 'gap-1.5');
    this.started = now();
    this.tick();
    this.timer = setInterval(() => this.tick(), 250);
  }

  /** What is left, in milliseconds. */
  left(): number {
    return Math.max(0, this.remainingMs - (this.now() - this.started));
  }

  tick(): void {
    const left = this.left();
    const share = this.ttlMs > 0 ? Math.min(1, left / this.ttlMs) : 0;
    this.bar.style.setProperty('width', `${(share * 100).toFixed(1)}%`);
    this.bar.toggleAttribute('data-low', share < 0.2);
    const s = Math.ceil(left / 1000);
    this.label.textContent = left > 0 ? `Nouveau code dans ${Math.floor(s / 60)}:${String(s % 60).padStart(2, '0')}` : 'Nouveau code…';
    if (left === 0 && !this.done) {
      this.done = true;
      this.dispose();
      this.onDone();
    }
  }

  dispose(): void {
    if (this.timer !== null) clearInterval(this.timer);
    this.timer = null;
  }
}
