import { describe, expect, it } from 'vitest';
import { codeFromHash } from '@/app/pages/ApprovePage.ts';
import { nextStep } from '@/app/pages/SigninPage.ts';

describe('codeFromHash', () => {
  it('reads the code a QR carries', () => {
    expect(codeFromHash('#c=dY-5CaY_Z4PL')).toBe('dY-5CaY_Z4PL');
    expect(codeFromHash('c=abc')).toBe('abc');
  });

  it('is empty without one', () => {
    expect(codeFromHash('')).toBe('');
    expect(codeFromHash('#other=1')).toBe('');
  });
});

describe('nextStep', () => {
  it('goes home once approved, waits while pending, starts over otherwise', () => {
    expect(nextStep('approved')).toBe('home');
    expect(nextStep('pending')).toBe('again');
    for (const s of ['cancelled', 'expired', 'used', 'unknown', 'error'] as const) {
      expect(nextStep(s)).toBe('restart');
    }
  });
});
