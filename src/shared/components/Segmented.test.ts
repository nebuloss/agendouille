import { describe, expect, it } from 'vitest';
import { segmented } from '@/shared/components/Ui.ts';

describe('segmented', () => {
  it('shows the choices side by side, one on, and says when it changes', () => {
    const seen: string[] = [];
    const s = segmented(document, 'Accès', [{ value: 'read', label: 'Lecture' }, { value: 'readwrite', label: 'Lecture et écriture' }], 'read',
      (v) => seen.push(v));
    document.body.append(s.el);
    const radios = (): HTMLButtonElement[] => Array.from(s.el.querySelectorAll<HTMLButtonElement>('[role="radio"]'));
    expect(s.el.getAttribute('role')).toBe('radiogroup');
    expect(radios().map((b) => b.getAttribute('aria-checked'))).toEqual(['true', 'false']);
    radios()[1]!.click();
    expect(s.value).toBe('readwrite');
    expect(seen).toEqual(['readwrite']);
    expect(radios().map((b) => b.getAttribute('aria-checked'))).toEqual(['false', 'true']);
    // The one already on: nothing happens.
    radios()[1]!.click();
    expect(seen).toEqual(['readwrite']);
    s.value = 'read';
    expect(radios()[0]!.getAttribute('aria-checked')).toBe('true');
    s.el.remove();
  });
});
