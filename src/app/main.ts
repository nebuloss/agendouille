// The family web app's bundle (static/app/app.js). It also runs inside the
// Android app's WebView. Pages are added here as they land.
import { ApprovePage } from '@/app/pages/ApprovePage.ts';
import { SigninPage } from '@/app/pages/SigninPage.ts';
import { Pages, whenReady } from '@/shared/core/Pages.ts';

export const pages = new Pages({
  signin: (doc) => new SigninPage(doc),
  approve: (doc) => new ApprovePage(doc),
});

whenReady(document, () => {
  document.documentElement.dataset['bundle'] = 'app';
  pages.mount(document);
});
