import { describe, expect, it } from 'vitest';
import { currentTheme, nextTheme, setTheme } from '@/shared/core/Theme.ts';
import { dialog, themeToggle } from '@/shared/components/Ui.ts';
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

  it('the toggle checks the current choice and switches (light, follow, dark)', () => {
    setTheme('light');
    const c = themeToggle(document);
    const checked = (): string => c.querySelector('[aria-checked="true"]')!.getAttribute('aria-label') ?? '';
    expect(checked()).toBe('Clair');
    (c.querySelectorAll('button')[2] as HTMLButtonElement).click();
    expect(checked()).toBe('Sombre');
    (c.querySelectorAll('button')[1] as HTMLButtonElement).click();
    expect(checked()).toBe('Suivre l’appareil');
    expect(currentTheme()).toBe('auto');
    (c.querySelectorAll('button')[2] as HTMLButtonElement).click();
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
