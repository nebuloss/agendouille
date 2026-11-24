// The admin UI's bundle (static/admin/admin.js), served on the admin
// listener only. Each page fills the shell the server rendered.
import './admin.css';
import { adminApi } from '@/admin/pages/base.ts';
import { NoticeCenter } from '@/admin/Notices.ts';
import { CalendarPage, CalendarsPage } from '@/admin/pages/Calendar.ts';
import { ActivityPage, BackupsPage, GroupPage, GroupsPage, SettingsPage } from '@/admin/pages/More.ts';
import { OverviewPage } from '@/admin/pages/Overview.ts';
import { UserPage, UsersPage } from '@/admin/pages/Users.ts';
import { Pages, whenReady, type Page } from '@/shared/core/Pages.ts';
import { langMenu, themeButton } from '@/shared/components/Ui.ts';
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
  const page = pages.mount(document);
  live(page);
  // The rail folds to its icons (folded by default on a phone); the choice
  // is this browser's.
  const rail = document.querySelector<HTMLElement>('.rail');
  if (rail !== null) foldable(rail);
  // On a phone the trail can be wider than the screen: show its end.
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

/** The rail's fold button: folded shows the icons only, their names as tooltips. */
function foldable(rail: HTMLElement): void {
  const button = rail.querySelector<HTMLButtonElement>('.rail-toggle');
  if (button === null) return;
  const key = 'agd_rail';
  let stored: string | null = null;
  try {
    stored = window.localStorage.getItem(key);
  } catch {
    stored = null;
  }
  if (stored === 'folded' || stored === 'open') rail.dataset['folded'] = String(stored === 'folded');
  const folded = (): boolean => rail.dataset['folded'] === 'true' || (rail.dataset['folded'] === 'auto' && window.matchMedia('(max-width: 47.99rem)').matches);
  const draw = (): void => {
    const f = folded();
    const label = f ? button.dataset['unfold'] ?? '' : button.dataset['fold'] ?? '';
    button.setAttribute('aria-expanded', String(!f));
    button.title = label;
    const span = button.querySelector('span');
    if (span !== null) span.textContent = label;
  };
  // On a phone the open rail covers the page: it does not stay open from one
  // page to the next, and a tap beside it folds it.
  const phone = (): boolean => window.matchMedia('(max-width: 47.99rem)').matches;
  if (phone() && stored === 'open') rail.dataset['folded'] = 'auto';
  button.addEventListener('click', () => {
    const next = !folded();
    rail.dataset['folded'] = String(next);
    if (!phone()) {
      try {
        window.localStorage.setItem(key, next ? 'folded' : 'open');
      } catch {
        // a browser that keeps nothing: the choice lasts this page
      }
    }
    draw();
  });
  document.addEventListener('click', (e) => {
    if (phone() && !folded() && !rail.contains(e.target as Node)) {
      rail.dataset['folded'] = 'true';
      draw();
    }
  });
  draw();
}

/**
 * Keeps the page in step with changes made elsewhere (a phone that syncs,
 * the family app, another admin tab): a short fingerprint of what the admin
 * shows (/api/admin/stamp), read every few seconds while the tab is seen
 * and when it comes back; when it changed, the page loads its data again,
 * unless a dialog is open or a field has the focus.
 */
function live(page: Page | null): void {
  const reloadable = page as (Page & { refresh?: () => Promise<void> }) | null;
  if (reloadable === null || typeof reloadable.refresh !== 'function') return;
  let last: string | null = null;
  let busy = false;
  const check = async (): Promise<void> => {
    if (busy || document.visibilityState !== 'visible') return;
    busy = true;
    try {
      const { stamp } = await api.get<{ stamp: string }>('/stamp');
      const typing = document.activeElement instanceof HTMLElement && document.activeElement.matches('input, textarea, select, [contenteditable]');
      if (last !== null && stamp !== last) {
        if (document.querySelector('dialog[open]') !== null || typing) return; // next time: last stays the old one
        await reloadable.refresh!();
        document.dispatchEvent(new Event('agd:changed'));
      }
      last = stamp;
    } catch {
      // offline or signed out: the next check tells
    } finally {
      busy = false;
    }
  };
  void check();
  window.setInterval(() => void check(), 5000);
  document.addEventListener('visibilitychange', () => void check());
  window.addEventListener('focus', () => void check());
}
