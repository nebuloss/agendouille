import { describe, expect, it } from 'vitest';
import { currentTheme, nextTheme, setTheme } from '@/shared/core/Theme.ts';
import { dialog, themeChoice } from '@/shared/components/Ui.ts';
import { h } from '@/shared/core/Dom.ts';

describe('Theme', () => {
  it('sets data-theme and the cookie, and auto removes both', () => {
    setTheme('dark');
    expect(document.documentElement.dataset['theme']).toBe('dark');
    expect(document.cookie).toContain('agd_theme=dark');
    expect(currentTheme()).toBe('dark');
    setTheme('auto');
    expect(document.documentElement.dataset['theme']).toBeUndefined();
    expect(document.cookie).not.toContain('agd_theme=dark');
    expect(nextTheme('auto')).toBe('light');
    expect(nextTheme('dark')).toBe('auto');
  });

  it('the appearance control checks the current choice and switches', () => {
    setTheme('light');
    const c = themeChoice(document);
    const checked = (): string => c.querySelector('[aria-checked="true"]')!.textContent ?? '';
    expect(checked()).toBe('Clair');
    (c.querySelectorAll('button')[2] as HTMLButtonElement).click();
    expect(checked()).toBe('Sombre');
    expect(currentTheme()).toBe('dark');
    setTheme('auto');
  });
});

describe('dialog', () => {
  it('only the confirming button validates: Annuler closes with a required field empty', () => {
    void dialog(document, 'Nouvel événement', [h('input', { required: true })], [
      { label: 'Annuler', value: 'cancel' }, { label: 'Enregistrer', value: 'save', kind: 'filled' },
    ]);
    const [cancel, save] = Array.from(document.querySelectorAll('dialog button')) as HTMLButtonElement[];
    expect(cancel!.formNoValidate).toBe(true);
    expect(save!.formNoValidate).toBe(false);
    document.querySelector('dialog')!.remove();
  });
});
