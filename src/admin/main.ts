// The admin UI's bundle (static/admin/admin.js), served on the admin
// listener only. Each page fills the shell the server rendered.
import './admin.css';
import { adminApi } from '@/admin/pages/base.ts';
import { NoticeCenter } from '@/admin/Notices.ts';
import { CalendarPage, CalendarsPage } from '@/admin/pages/Calendar.ts';
import { ActivityPage, BackupsPage, GroupPage, GroupsPage, SettingsPage } from '@/admin/pages/More.ts';
import { OverviewPage } from '@/admin/pages/Overview.ts';
import { UserPage, UsersPage } from '@/admin/pages/Users.ts';
import { Pages, whenReady } from '@/shared/core/Pages.ts';
import { langMenu, themeToggle } from '@/shared/components/Ui.ts';
import { ownValidation } from '@/shared/components/Validation.ts';

// Field errors in our words and design, not the browser's bubbles.
ownValidation(document);

const api = adminApi();

export const pages = new Pages({
  overview: (doc) => new OverviewPage(doc, api),
  users: (doc) => new UsersPage(doc, api),
  user: (doc) => new UserPage(doc, api),
  groups: (doc) => new GroupsPage(doc, api),
  group: (doc) => new GroupPage(doc, api),
  calendar: (doc) => new CalendarPage(doc, api),
  calendars: (doc) => new CalendarsPage(doc, api),
  activity: (doc) => new ActivityPage(doc, api),
  settings: (doc) => new SettingsPage(doc, api),
  backups: (doc) => new BackupsPage(doc, api),
});

whenReady(document, () => {
  document.documentElement.dataset['bundle'] = 'admin';
  pages.mount(document);
  // On a phone the rail is folded to its icons; its menu button opens it.
  const rail = document.querySelector<HTMLElement>('.rail');
  if (rail !== null) foldable(rail);
  // On a phone the trail can be wider than the screen: show its end.
  const trail = document.querySelector<HTMLElement>('.trail');
  if (trail !== null) trail.scrollLeft = trail.scrollWidth;
  // The theme, one button that cycles: system, light, dark.
  for (const slot of Array.from(document.querySelectorAll('[data-lang-menu]'))) slot.replaceWith(langMenu(document));
  const themeSlot = document.getElementById('theme-slot');
  if (themeSlot !== null) themeSlot.replaceWith(langMenu(document), themeToggle(document, 'sm'));
  // The bell, on every page of the shell; it counts again after a change.
  const slot = document.getElementById('bell-slot');
  if (slot !== null) {
    const notices = new NoticeCenter(document, api, slot);
    notices.start();
    document.addEventListener('agd:changed', () => void notices.load());
  }
});

/** The rail's menu button, on a phone: open over the page, folded again on a tap beside it. */
function foldable(rail: HTMLElement): void {
  const button = rail.querySelector<HTMLButtonElement>('.rail-toggle');
  if (button === null) return;
  const open = (): boolean => rail.dataset['folded'] === 'false';
  const draw = (): void => {
    const label = open() ? button.dataset['fold'] ?? '' : button.dataset['unfold'] ?? '';
    button.setAttribute('aria-expanded', String(open()));
    button.title = label;
    const span = button.querySelector('span');
    if (span !== null) span.textContent = label;
  };
  button.addEventListener('click', () => {
    rail.dataset['folded'] = open() ? 'auto' : 'false';
    draw();
  });
  document.addEventListener('click', (e) => {
    if (open() && !rail.contains(e.target as Node)) {
      rail.dataset['folded'] = 'auto';
      draw();
    }
  });
  draw();
}

