import { describe, expect, it } from 'vitest';
import { Api } from '@/shared/core/Api.ts';
import { NoticeCenter, worst, type Notice } from '@/admin/Notices.ts';

const n = (id: string, severity: Notice['severity'], read = false): Notice => ({ id, kind: 'server', severity, title: `t ${id}`, read });

async function settle(): Promise<void> {
  for (let i = 0; i < 5; i++) await new Promise((r) => setTimeout(r, 0));
}

describe('NoticeCenter', () => {
  it('counts the unread in the colour of the worst, and marks read', async () => {
    let notices = [n('b', 'error'), n('a', 'warn'), n('c', 'info', true)]; // the server sorts, errors first
    const marked: string[] = [];
    const api = new Api('/api/admin', async (input, init) => {
      if (input.endsWith('/notices/read')) {
        const ids = (JSON.parse(String(init.body)) as { ids: string[] }).ids;
        marked.push(...ids);
        notices = notices.map((x) => (ids.includes(x.id) ? { ...x, read: true } : x));
      }
      return new Response(JSON.stringify({ notices }), { status: 200 });
    });
    document.body.replaceChildren();
    const slot = document.createElement('span');
    document.body.append(slot);
    const c = new NoticeCenter(document, api, slot);
    await c.load();
    const bell = slot.querySelector('button')!;
    expect(bell.getAttribute('aria-label')).toBe('2 notifications');
    expect(bell.querySelector('span')?.className).toContain('bg-error');

    c.open();
    await settle();
    const sheet = document.querySelector('dialog')!;
    expect(sheet.textContent).toContain('t b');
    expect(sheet.textContent).not.toContain('t c'); // read already
    sheet.querySelector<HTMLButtonElement>('button[aria-label="Marquer comme lu"]')!.click();
    await settle();
    expect(marked).toEqual(['b']);
    expect(bell.getAttribute('aria-label')).toBe('1 notification');
    expect(bell.querySelector('span')?.className).toContain('bg-warning');
    c.dispose();
  });

  it('knows the worst', () => {
    expect(worst([n('a', 'info'), n('b', 'warn')])).toBe('warn');
    expect(worst([])).toBeNull();
  });
});
