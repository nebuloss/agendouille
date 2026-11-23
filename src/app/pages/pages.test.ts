import { beforeEach, describe, expect, it } from 'vitest';
import { Api } from '@/shared/core/Api.ts';
import { parseLocal } from '@/shared/core/Dates.ts';
import { agenda, month, week, whenText, type ViewContext } from '@/app/components/views.ts';
import { CalendarPage } from '@/app/pages/CalendarPage.ts';
import { CalendarsPage, browserName } from '@/app/pages/Manage.ts';
import { secretField } from '@/shared/components/Ui.ts';
import type { Occurrence } from '@/app/api.ts';

function shell(page: string): void {
  document.body.replaceChildren();
  document.body.dataset['page'] = page;
  const app = document.createElement('div');
  app.className = 'app';
  app.dataset['zone'] = 'Europe/Paris';
  app.dataset['user'] = 'lea';
  const content = document.createElement('section');
  content.id = 'app-content';
  const toasts = document.createElement('div');
  toasts.id = 'toasts';
  app.append(content, toasts);
  document.body.append(app);
}

function fakeApi(routes: Record<string, unknown>, seen: string[] = []): Api {
  return new Api('/api', async (input, init) => {
    seen.push(`${init.method} ${input}`);
    const path = input.replace(/^\/api/, '').split('?')[0] ?? '';
    if (!(path in routes)) return new Response(JSON.stringify({ error: 'not_found', message: `no ${path}` }), { status: 404 });
    return new Response(JSON.stringify(routes[path]), { status: 200 });
  });
}

async function settle(): Promise<void> {
  for (let i = 0; i < 5; i++) await new Promise((r) => setTimeout(r, 0));
}

const occ = (summary: string, start: string, end: string, extra: Partial<Occurrence> = {}): Occurrence => ({
  calendar: 'lea~ecole', name: `${summary}.ics`, etag: '"1"', uid: summary, recurrence_id: null, summary, location: '', all_day: false,
  start, end, recurring: false, overridden: false, created_by: 'lea', modified_by: 'lea', ...extra,
});

const ctx = (opened: Occurrence[] = []): ViewContext => ({
  colorOf: () => '#1e88e5', today: parseLocal('2026-11-25T10:30:00'), onOpen: (o) => opened.push(o), onNew: () => undefined,
});

describe('views', () => {
  it('the agenda lists days with events, today always, as text', () => {
    const el = agenda([
      occ('<b>Judo</b>', '2026-11-25T14:00:00', '2026-11-25T15:30:00'),
      occ('Vacances', '2026-11-27', '2026-11-30', { all_day: true }),
    ], parseLocal('2026-11-24'), 7, ctx());
    const days = [...el.querySelectorAll('.agenda-section')].map((s) => s.getAttribute('data-date'));
    expect(days).toEqual(['2026-11-25', '2026-11-27', '2026-11-28', '2026-11-29']); // a 3-day all-day event, today
    expect(el.querySelector('b')).toBeNull();
    expect(el.textContent).toContain('<b>Judo</b>');
  });

  it('the week puts overlapping events side by side', () => {
    const el = week([
      occ('A', '2026-11-25T14:00:00', '2026-11-25T16:00:00'),
      occ('B', '2026-11-25T15:00:00', '2026-11-25T17:00:00'),
      occ('C', '2026-11-25T18:00:00', '2026-11-25T19:00:00'),
    ], parseLocal('2026-11-23'), ctx());
    const col = el.querySelector('[data-date="2026-11-25"]')!;
    const chips = [...col.querySelectorAll<HTMLElement>('.event')];
    expect(chips.map((c) => c.style.getPropertyValue('width'))).toEqual(['50%', '50%', '100%']);
    expect(chips[0]!.style.getPropertyValue('top')).toBe(`${14 * 48}px`);
    expect(col.querySelector('.now-line')).not.toBeNull();
  });

  it('the month shows three events a day and the rest as +n', () => {
    const many = ['a', 'b', 'c', 'd', 'e'].map((s, i) => occ(s, `2026-11-25T0${i}:00:00`, `2026-11-25T0${i}:30:00`));
    const opened: Occurrence[] = [];
    const el = month(many, parseLocal('2026-11-01'), parseLocal('2026-10-26'), ctx(opened));
    const cell = el.querySelector('[data-date="2026-11-25"]')!;
    expect(cell.querySelectorAll('.event').length).toBe(3);
    expect(cell.querySelector('.more')?.textContent).toBe('+2');
    expect(el.querySelectorAll('.month-cell').length).toBe(42);
    cell.querySelector<HTMLElement>('.event')!.click();
    expect(opened[0]?.summary).toBe('a');
  });

  it('says when in words', () => {
    expect(whenText({ start: '2026-11-25T14:00:00', end: '2026-11-25T15:30:00', all_day: false })).toBe('mercredi 25 novembre, 14:00 – 15:30');
    expect(whenText({ start: '2026-11-25', end: '2026-11-26', all_day: true })).toBe('mercredi 25 novembre');
    expect(whenText({ start: '2026-11-25', end: '2026-11-28', all_day: true })).toBe('du mercredi 25 novembre au vendredi 27 novembre');
  });
});

describe('app pages', () => {
  beforeEach(() => shell('calendar'));

  it('the calendar page loads a range and draws it', async () => {
    const seen: string[] = [];
    const page = new CalendarPage(document, fakeApi({
      '/calendars': { family_timezone: 'Europe/Paris', calendars: [{ ref: 'lea~ecole', owner: 'lea', name: 'École', color: '#1e88e5', access: 'owner', shown: true, hidden: false }] },
      '/occurrences': { occurrences: [occ('Judo', '2026-11-25T14:00:00', '2026-11-25T15:00:00')] },
    }, seen));
    page.start();
    await settle();
    expect(seen.some((s) => /GET \/api\/occurrences\?from=\d{4}-\d{2}-\d{2}&to=/.test(s))).toBe(true);
    expect(document.querySelector('.cal-toolbar')).not.toBeNull();
    page.dispose();
  });

  it('the calendars page splits mine and shared, as text', async () => {
    const page = new CalendarsPage(document, fakeApi({
      '/calendars': { calendars: [
        { ref: 'lea~ecole', owner: 'lea', name: 'École', color: '#1e88e5', description: '', access: 'owner', shown: true, hidden: false, shares: [{ target: 'g:famille', access: 'read' }] },
        { ref: 'maman~maison', owner: 'maman', name: '<i>Maison</i>', color: '#43a047', description: '', access: 'read', shown: true, hidden: false, shares: null },
      ] },
      '/people': { users: [{ name: 'maman', display_name: 'Maman', color: '' }], groups: [{ name: 'famille', display_name: 'Famille', color: '' }] },
    }));
    page.start();
    await settle();
    const text = document.getElementById('app-content')?.textContent ?? '';
    expect(text).toContain('groupe Famille');
    expect(text).toContain('<i>Maison</i>');
    expect(document.querySelector('#app-content i')).toBeNull();
  });

  it('names browsers', () => {
    expect(browserName('Mozilla/5.0 (X11; Linux x86_64; rv:128.0) Gecko/20100101 Firefox/128.0')).toBe('Firefox, Linux');
    expect(browserName('Mozilla/5.0 (Linux; Android 14) AppleWebKit/537.36 Chrome/126.0 Mobile Safari/537.36')).toBe('Chrome, Android');
  });
});

describe('secretField', () => {
  it('masks a secret until asked, then shows it', () => {
    const f = secretField('agd_abcd-efgh-ijkl', 'Mot de passe');
    expect(f.textContent).not.toContain('agd_');
    const show = [...f.querySelectorAll('button')].find((b) => b.textContent === 'Afficher')!;
    show.click();
    expect(f.textContent).toContain('agd_abcd-efgh-ijkl');
    show.click();
    expect(f.textContent).not.toContain('agd_');
  });
});
