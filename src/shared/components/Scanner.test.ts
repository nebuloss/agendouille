import { describe, expect, it } from 'vitest';
import { approvalTarget } from '@/shared/components/Scanner.ts';

describe('approvalTarget', () => {
  const origin = 'https://agenda.example.org';
  it('takes our sign-in codes', () => {
    expect(approvalTarget('https://agenda.example.org/approve#c=AbC-12_x', origin)).toBe('/approve#c=AbC-12_x');
    expect(approvalTarget('  https://agenda.example.org/approve#c=ABCD  ', origin)).toBe('/approve#c=ABCD');
  });
  it('refuses anything else', () => {
    expect(approvalTarget('https://evil.example.org/approve#c=ABCD', origin)).toBeNull();
    expect(approvalTarget('https://agenda.example.org/enroll/xyz', origin)).toBeNull();
    expect(approvalTarget('https://agenda.example.org/approve', origin)).toBeNull();
    expect(approvalTarget('https://agenda.example.org/approve#c=a<b>', origin)).toBeNull();
    expect(approvalTarget('javascript:alert(1)', origin)).toBeNull();
    expect(approvalTarget('not a url', origin)).toBeNull();
  });
});
