import { describe, expect, it } from 'vitest';
import { fold, pick } from '@/shared/components/Picker.ts';

const items = [
  { value: 'lea', label: 'Léa', sub: 'lea' },
  { value: 'maman', label: 'Maman', sub: 'maman' },
  { value: 'tom', label: 'Tom', sub: 'tom' },
];

const rows = (): HTMLButtonElement[] => Array.from(document.querySelectorAll<HTMLButtonElement>('dialog [role="option"]'));
const search = (q: string): void => {
  const s = document.querySelector<HTMLInputElement>('dialog input[type="search"]')!;
  s.value = q;
  s.dispatchEvent(new Event('input'));
};

describe('pick', () => {
  it('folds accents and case', () => {
    expect(fold('Léa ÉLODIE')).toBe('lea elodie');
  });

  it('filters as you type, and one click picks for a single choice', async () => {
    const p = pick(document, { title: 'Nouveau propriétaire', items });
    expect(rows()).toHaveLength(3);
    search('LEA');
    expect(rows().map((r) => r.dataset['value'])).toEqual(['lea']);
    search('zzz');
    expect(rows()).toHaveLength(0);
    expect(document.querySelector('dialog')!.textContent).toContain('Aucun résultat pour');
    search('');
    rows()[1]!.click();
    expect(await p).toEqual(['maman']);
    expect(document.querySelector('dialog')).toBeNull();
  });

  it('ticks several, then confirms; cancel gives null', async () => {
    const p = pick(document, { title: 'Ajouter des membres', items, multiple: true, confirm: 'Ajouter' });
    const confirm = Array.from(document.querySelectorAll<HTMLButtonElement>('dialog button')).find((b) => b.textContent === 'Ajouter')!;
    expect(confirm.disabled).toBe(true);
    rows()[0]!.click();
    rows()[2]!.click();
    expect(rows()[0]!.getAttribute('aria-selected')).toBe('true');
    expect(document.querySelector('dialog')!.textContent).toContain('2 sélectionnés');
    confirm.click();
    expect(await p).toEqual(['lea', 'tom']);

    const q = pick(document, { title: 'x', items, multiple: true });
    Array.from(document.querySelectorAll<HTMLButtonElement>('dialog button')).find((b) => b.textContent === 'Annuler')!.click();
    expect(await q).toBeNull();
  });

  it('opens with the current choices ticked, and may confirm none when allowed', async () => {
    const p = pick(document, { title: 'Partager', items, multiple: true, allowNone: true, selected: ['maman'], confirm: 'Enregistrer' });
    const confirm = Array.from(document.querySelectorAll<HTMLButtonElement>('dialog button')).find((b) => b.textContent === 'Enregistrer')!;
    expect(rows()[1]!.getAttribute('aria-selected')).toBe('true');
    rows()[1]!.click();
    expect(confirm.disabled).toBe(false);
    confirm.click();
    expect(await p).toEqual([]);
  });
});
