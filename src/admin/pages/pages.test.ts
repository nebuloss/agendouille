import { beforeEach, describe, expect, it } from 'vitest';
import { Api } from '@/shared/core/Api.ts';
import { OverviewPage } from '@/admin/pages/Overview.ts';
import { UsersPage } from '@/admin/pages/Users.ts';
import { CalendarsPage } from '@/admin/pages/More.ts';
import { bytes, ago } from '@/shared/render/format.ts';
import { isImageDataUrl, svgDataUrl } from '@/shared/core/Dom.ts';

function shell(page: string): void {
  document.body.replaceChildren();
  document.body.dataset['page'] = page;
  const content = document.createElement('section');
  content.id = 'admin-content';
  const title = document.createElement('h1');
  title.textContent = 'T';
  content.append(title);
  const toasts = document.createElement('div');
  toasts.id = 'toasts';
  document.body.append(content, toasts);
}

function fakeApi(routes: Record<string, unknown>): Api {
  return new Api('/api/admin', async (input) => {
    const path = input.replace('/api/admin', '').split('?')[0] ?? '';
    if (!(path in routes)) return new Response(JSON.stringify({ error: 'not_found', message: `no ${path}` }), { status: 404 });
    return new Response(JSON.stringify(routes[path]), { status: 200, headers: { 'Content-Type': 'application/json' } });
  });
}

async function settle(): Promise<void> {
  for (let i = 0; i < 5; i++) await new Promise((r) => setTimeout(r, 0));
}

describe('admin pages', () => {
  beforeEach(() => shell('x'));

  it('the overview shows the warnings and the counts', async () => {
    const page = new OverviewPage(document, fakeApi({
      '/overview': { users: 2, groups: 1, calendars: 3, events: 40, devices: 4, stale_devices: [], storage: { database: 2048, backups: 0 },
        last_backup: null, version: 'dev', public_url: '', warnings: [{ code: 'no_public_listener', message: 'CalDAV is off' }] },
    }));
    page.start();
    await settle();
    const text = document.getElementById('admin-content')?.textContent ?? '';
    expect(text).toContain('CalDAV is off');
    expect(text).toContain('utilisateurs');
    expect(document.querySelector('[data-code="no_public_listener"]')).not.toBeNull();
    expect(document.querySelector('h1')?.textContent).toBe('T'); // the title stays
  });

  it('the users page lists users as text, never as markup', async () => {
    const page = new UsersPage(document, fakeApi({
      '/users': { users: [{ name: 'lea', display_name: '<img src=x onerror=alert(1)>', color: '#c2185b', disabled: false, created_at: '',
        groups: ['famille'], devices: 1, calendars_owned: 2 }] },
      '/groups': { groups: [{ name: 'famille', display_name: 'Famille' }] },
    }));
    page.start();
    await settle();
    expect(document.querySelector('td img')).toBeNull();
    expect(document.body.textContent).toContain('<img src=x onerror=alert(1)>');
    expect(document.querySelector('a[href="/admin/users/lea"]')).not.toBeNull();
    expect(document.querySelectorAll('form.add-user input[type="checkbox"]').length).toBe(1);
  });

  it('an error from the server is shown in place of the page', async () => {
    const page = new CalendarsPage(document, fakeApi({}));
    page.start();
    await settle();
    expect(document.querySelector('#admin-content [role="alert"]')?.textContent).toContain('no /calendars');
  });
});

describe('format', () => {
  it('sizes and ages', () => {
    expect(bytes(512)).toBe('512 o');
    expect(bytes(1536)).toBe('1,5 Kio');
    expect(ago(new Date(Date.now() - 3 * 86400e3).toISOString())).toBe('il y a 3 jours');
    expect(ago(undefined)).toBe('jamais');
  });
});

describe('QR images', () => {
  it('only base64 SVG and PNG data URLs pass', () => {
    expect(isImageDataUrl(svgDataUrl('<svg xmlns="http://www.w3.org/2000/svg"/>'))).toBe(true);
    expect(isImageDataUrl('data:text/html;base64,PHNjcmlwdD4=')).toBe(false);
    expect(isImageDataUrl('javascript:alert(1)')).toBe(false);
  });
});
