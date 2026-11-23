// The admin UI's bundle (static/admin/admin.js), served on the admin
// listener only. Each page fills the shell the server rendered.
import './admin.css';
import { adminApi } from '@/admin/pages/base.ts';
import { NoticeCenter } from '@/admin/Notices.ts';
import { ActivityPage, BackupsPage, GroupsPage, SettingsPage } from '@/admin/pages/More.ts';
import { OverviewPage } from '@/admin/pages/Overview.ts';
import { UserPage, UsersPage } from '@/admin/pages/Users.ts';
import { Pages, whenReady } from '@/shared/core/Pages.ts';

const api = adminApi();

export const pages = new Pages({
  overview: (doc) => new OverviewPage(doc, api),
  users: (doc) => new UsersPage(doc, api),
  user: (doc) => new UserPage(doc, api),
  groups: (doc) => new GroupsPage(doc, api),
  activity: (doc) => new ActivityPage(doc, api),
  settings: (doc) => new SettingsPage(doc, api),
  backups: (doc) => new BackupsPage(doc, api),
});

whenReady(document, () => {
  document.documentElement.dataset['bundle'] = 'admin';
  pages.mount(document);
  // The bell, on every page of the shell; it counts again after a change.
  const slot = document.getElementById('bell-slot');
  if (slot !== null) {
    const notices = new NoticeCenter(document, api, slot);
    notices.start();
    document.addEventListener('agd:changed', () => void notices.load());
  }
});
