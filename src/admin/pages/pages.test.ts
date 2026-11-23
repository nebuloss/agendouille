import { beforeEach, describe, expect, it } from 'vitest';
import { Api } from '@/shared/core/Api.ts';
import { OverviewPage } from '@/admin/pages/Overview.ts';
import { UsersPage } from '@/admin/pages/Users.ts';
import { GroupsPage } from '@/admin/pages/More.ts';
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
    expect(document.querySelector('a[href="/users/lea"]')).not.toBeNull();
    // "Ajouter un utilisateur" opens the form in a modal, with one box per group.
    [...document.querySelectorAll('button')].find((b) => b.textContent === 'Ajouter un utilisateur')!.click();
    expect(document.querySelectorAll('dialog .add-user-groups input[type="checkbox"]').length).toBe(1);
    expect(document.querySelector('dialog')?.textContent).toContain('Nouvel utilisateur');
  });

  it('the overview agrees with its counts, in French and in English', async () => {
    const overview = { users: 1, groups: 0, calendars: 2, events: 1234, devices: 1, stale_devices: [], storage: { database: 0, backups: 0 },
      last_backup: null, version: 'dev', public_url: '', warnings: [] };
    new OverviewPage(document, fakeApi({ '/overview': overview })).start();
    await settle();
    let text = document.getElementById('admin-content')?.textContent ?? '';
    expect(text).toContain('1utilisateur');
    expect(text).toContain('2calendriers');
    expect(text).toMatch(/1\s234événements/);
    expect(text).toContain('Dernière sauvegarde : aucune');
    document.documentElement.lang = 'en';
    try {
      shell('x');
      new OverviewPage(document, fakeApi({ '/overview': overview })).start();
      await settle();
      text = document.getElementById('admin-content')?.textContent ?? '';
      expect(text).toContain('1user');
      expect(text).toContain('0groups');
      expect(text).toContain('1,234events');
      expect(text).toContain('Last backup: none');
    } finally {
      document.documentElement.lang = 'fr';
    }
  });

  it('a user without groups says so, in the page’s language', async () => {
    const routes = {
      '/users': { users: [{ name: 'tom', display_name: 'Tom', color: '', disabled: true, created_at: '', groups: [], devices: 0, calendars_owned: 1 }] },
      '/groups': { groups: [] },
    };
    new UsersPage(document, fakeApi(routes)).start();
    await settle();
    expect(document.body.textContent).toContain('tom · aucun groupe · jamais vu');
    expect(document.body.textContent).toContain('désactivé');
    document.documentElement.lang = 'en';
    try {
      shell('x');
      new UsersPage(document, fakeApi(routes)).start();
      await settle();
      expect(document.body.textContent).toContain('tom · no group · never seen');
      expect(document.body.textContent).toContain('0devices');
      expect(document.body.textContent).toContain('1calendar');
    } finally {
      document.documentElement.lang = 'fr';
    }
  });

  it('an error from the server is shown in place of the page', async () => {
    const page = new GroupsPage(document, fakeApi({}));
    page.start();
    await settle();
    expect(document.querySelector('#admin-content [role="alert"]')?.textContent).toContain('no /groups');
  });
});

describe('format', () => {
  it('sizes and ages', () => {
    expect(bytes(512)).toBe('512 o');
    expect(bytes(1536)).toBe('1,5 Kio');
    expect(ago(new Date(Date.now() - 3 * 86400e3).toISOString())).toBe('il y a 3 jours');
    expect(ago(undefined)).toBe('Jamais');
  });

  it('sizes and ages in English when the page is', () => {
    document.documentElement.lang = 'en';
    try {
      expect(bytes(1536)).toBe('1.5 KiB');
      expect(ago(new Date(Date.now() - 3 * 86400e3).toISOString())).toBe('3 days ago');
      expect(ago(undefined)).toBe('Never');
    } finally {
      document.documentElement.lang = 'fr';
    }
  });
});

describe('QR images', () => {
  it('only base64 SVG and PNG data URLs pass', () => {
    expect(isImageDataUrl(svgDataUrl('<svg xmlns="http://www.w3.org/2000/svg"/>'))).toBe(true);
    expect(isImageDataUrl('data:text/html;base64,PHNjcmlwdD4=')).toBe(false);
    expect(isImageDataUrl('javascript:alert(1)')).toBe(false);
  });
});
