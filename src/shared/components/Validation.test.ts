import { describe, expect, it } from 'vitest';
import { ownValidation, reason } from '@/shared/components/Validation.ts';

describe('own validation', () => {
  it('says why under the field, in the page’s language, and clears once fixed', () => {
    ownValidation(document);
    const form = document.createElement('form');
    const input = document.createElement('input');
    input.required = true;
    form.append(input);
    document.body.append(form);
    expect(input.checkValidity()).toBe(false);
    expect(input.getAttribute('aria-invalid')).toBe('true');
    const id = input.getAttribute('aria-describedby')!;
    expect(document.getElementById(id)?.textContent).toBe('Ce champ est obligatoire.');
    input.value = 'Léa';
    input.dispatchEvent(new Event('input', { bubbles: true }));
    expect(input.hasAttribute('aria-invalid')).toBe(false);
    expect(document.getElementById(id)).toBeNull();
    form.remove();
  });

  it('names each kind of problem', () => {
    const url = document.createElement('input');
    url.type = 'url';
    url.value = 'not a url';
    expect(reason(url)).toContain('https://');
    const long = document.createElement('input');
    long.required = true;
    expect(reason(long)).toBe('Ce champ est obligatoire.');
  });
});
