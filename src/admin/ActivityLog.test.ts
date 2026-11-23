import { beforeEach, describe, expect, it } from 'vitest';
import { Api } from '@/shared/core/Api.ts';
import { ActivityLog, stamp, type Activity } from '@/admin/ActivityLog.ts';

const rows: Activity[] = [
  { at: '2026-11-23T10:00:00Z', kind: 'event', action: 'created', by: 'lea', calendar: 'lea~perso', via: 'dav', device: 'Pixel' },
  { at: '2026-11-23T11:00:00Z', kind: 'share', action: 'shared', by: 'maman', calendar: 'maman~ecole', detail: 'g:famille' },
  { at: '2026-11-23T12:00:00Z', kind: 'event', action: 'changed', by: 'lea', calendar: 'maman~ecole', via: 'web' },
];

async function settle(): Promise<void> {
  for (let i = 0; i < 5; i++) await new Promise((r) => setTimeout(r, 0));
}

describe('ActivityLog', () => {
  let asked: string[];
  let api: Api;
  beforeEach(() => {
    document.body.replaceChildren();
    window.history.replaceState(null, '', '/activity');
    asked = [];
    api = new Api('/api/admin', async (input) => {
      asked.push(input);
      const q = new URL(input, 'http://x').searchParams;
      const kept = rows.filter((r) => (!q.get('user') || r.by === q.get('user')) && (!q.get('kind') || r.kind === q.get('kind')));
      return new Response(JSON.stringify({ activity: kept }), { status: 200 });
    });
  });

  it('draws newest first, narrows from a cell, shows the filter as a chip, and keeps it in the URL', async () => {
    const log = new ActivityLog(document, api);
    document.body.append(log.root);
    await log.load();
    const first = log.root.querySelector('tbody tr td:nth-child(3)')!;
    expect(first.textContent).toBe('événement · modifié'); // 12:00, newest
    // A value in a row narrows to itself.
    [...log.root.querySelectorAll<HTMLButtonElement>('tbody button')].find((b) => b.textContent === 'maman')!.click();
    await settle();
    expect(asked.at(-1)).toContain('user=maman');
    expect(window.location.search).toBe('?user=maman');
    expect(log.root.querySelectorAll('tbody tr').length).toBe(1);
    const chip = [...log.root.querySelectorAll('button')].find((b) => b.textContent?.startsWith('qui : maman'))!;
    expect(chip).toBeDefined();
    // The chip takes it off.
    chip.click();
    await settle();
    expect(window.location.search).toBe('');
    expect(log.root.querySelectorAll('tbody tr').length).toBe(3);
    log.dispose();
  });

  it('a heading opens its values with their counts', async () => {
    const log = new ActivityLog(document, api);
    document.body.append(log.root);
    await log.load();
    [...log.root.querySelectorAll<HTMLButtonElement>('thead button')].find((b) => b.textContent?.startsWith('Quoi'))!.click();
    const menu = document.querySelector('[role="menu"]')!;
    expect(menu.textContent).toContain('événement2');
    expect(menu.textContent).toContain('partage1');
    log.dispose();
  });

  it('on a user page, the user is the scope: no chip, no menu, always sent', async () => {
    const log = new ActivityLog(document, api, { user: 'lea' });
    document.body.append(log.root);
    await log.load();
    expect(asked.at(-1)).toContain('user=lea');
    expect([...log.root.querySelectorAll('thead button')].some((b) => b.textContent?.startsWith('Qui'))).toBe(false);
    expect(log.root.querySelector('a[download]')?.getAttribute('href')).toBe('/api/admin/activity/export?user=lea');
    log.dispose();
  });

  it('stamps today with the time alone', () => {
    const now = new Date('2026-11-23T15:00:00Z');
    expect(stamp('2026-11-23T14:00:00Z', now)).not.toContain('nov');
    expect(stamp('2026-11-20T14:00:00Z', now)).toContain('nov');
  });
});
