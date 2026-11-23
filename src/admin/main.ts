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
import { langMenu, themeButton } from '@/shared/components/Ui.ts';

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
  // On a phone the rail is a row and the trail can be wider than the
  // screen: bring the page's own entry, and the trail's end, into view.
  document.querySelector('.rail [aria-current="page"]')?.scrollIntoView({ block: 'nearest', inline: 'nearest' });
  const trail = document.querySelector<HTMLElement>('.trail');
  if (trail !== null) trail.scrollLeft = trail.scrollWidth;
  // The theme, one button that cycles: system, light, dark.
  for (const slot of Array.from(document.querySelectorAll('[data-lang-menu]'))) slot.replaceWith(langMenu(document));
  const themeSlot = document.getElementById('theme-slot');
  if (themeSlot !== null) themeSlot.replaceWith(langMenu(document), themeButton(document));
  // The bell, on every page of the shell; it counts again after a change.
  const slot = document.getElementById('bell-slot');
  if (slot !== null) {
    const notices = new NoticeCenter(document, api, slot);
    notices.start();
    document.addEventListener('agd:changed', () => void notices.load());
  }
});
