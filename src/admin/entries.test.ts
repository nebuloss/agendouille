import { describe, expect, it } from 'vitest';
import { h } from '@/shared/core/Dom.ts';
import { avatar, entries } from '@/admin/entries.ts';

describe('entries', () => {
  it('makes the whole row one link to the item, with the actions beside it, outside the link', () => {
    const remove = h('button', { type: 'button' }, 'Retirer');
    const list = entries([
      { href: '/users/lea', lead: avatar('Léa', '#2e7d32'), title: 'Léa', sub: 'lea', actions: [remove] },
      { href: '/groups/famille', title: 'Famille' },
    ], 'vide');
    const rows = list.querySelectorAll('li');
    expect(rows.length).toBe(2);
    const link = rows[0]!.querySelector('a')!;
    expect(link.getAttribute('href')).toBe('/users/lea');
    expect(link.className).toContain('flex-1'); // the link takes the row's width
    expect(link.textContent).toContain('Léa');
    expect(link.contains(remove)).toBe(false);
    expect(rows[0]!.contains(remove)).toBe(true);
    expect(rows[1]!.querySelector('a')!.getAttribute('href')).toBe('/groups/famille');
  });

  it('says so when there is nothing, and names are text', () => {
    expect(entries([], 'Personne.').textContent).toBe('Personne.');
    const list = entries([{ href: '/users/x', title: '<b>x</b>' }], '');
    expect(list.querySelector('b b')).toBeNull();
    expect(list.textContent).toContain('<b>x</b>');
  });
});
