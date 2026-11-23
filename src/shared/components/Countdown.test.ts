import { describe, expect, it } from 'vitest';
import { Countdown } from '@/shared/components/Countdown.ts';

describe('Countdown', () => {
  it('drains, turns red near the end, and asks for a new code once at zero', () => {
    let t = 0;
    let done = 0;
    const root = document.createElement('div');
    const c = new Countdown(root, 90_000, 120_000, () => done++, () => t);
    const bar = root.querySelector('i')!;
    expect(bar.style.getPropertyValue('width')).toBe('75.0%');
    expect(root.textContent).toBe('Nouveau code dans 1:30');
    expect(bar.hasAttribute('data-low')).toBe(false);

    t = 80_000;
    c.tick();
    expect(root.textContent).toBe('Nouveau code dans 0:10');
    expect(bar.hasAttribute('data-low')).toBe(true);

    t = 95_000;
    c.tick();
    c.tick();
    expect(done).toBe(1);
    expect(root.textContent).toBe('Génération d’un nouveau code…');
    c.dispose();
  });
});
