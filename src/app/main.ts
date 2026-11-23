// The family web app's bundle (static/app/app.js). It also runs inside the
// Android app's WebView.
import './app.css';
import { ApprovePage } from '@/app/pages/ApprovePage.ts';
import { CalendarPage } from '@/app/pages/CalendarPage.ts';
import { CalendarsPage, DevicesPage, MePage } from '@/app/pages/Manage.ts';
import { SigninPage } from '@/app/pages/SigninPage.ts';
import { Pages, whenReady } from '@/shared/core/Pages.ts';
import { langMenu } from '@/shared/components/Ui.ts';


export const pages = new Pages({
  signin: (doc) => new SigninPage(doc),
  approve: (doc) => new ApprovePage(doc),
  calendar: (doc) => new CalendarPage(doc),
  calendars: (doc) => new CalendarsPage(doc),
  devices: (doc) => new DevicesPage(doc),
  me: (doc) => new MePage(doc),
});

whenReady(document, () => {
  document.documentElement.dataset['bundle'] = 'app';
  pages.mount(document);
  // The language menu, wherever the page has room for it.
  for (const slot of Array.from(document.querySelectorAll('[data-lang-menu]'))) slot.replaceWith(langMenu(document));
});
