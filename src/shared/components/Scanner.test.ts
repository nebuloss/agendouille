import { readFileSync } from 'node:fs';
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

describe('the scanner', () => {
  // In Android's WebView, BarcodeDetector binds Google Play services and
  // kills the app (a Xiaomi, Android 17): the scanner decodes with jsQR only.
  it('never touches BarcodeDetector', () => {
    const code = readFileSync('src/shared/components/Scanner.ts', 'utf8').replace(/\/\*[\s\S]*?\*\//g, '').replace(/\/\/.*$/gm, '');
    expect(code).not.toContain('BarcodeDetector');
  });
});
