// The family web app's bundle (static/app/app.js). It also runs inside the
// Android app's WebView. Pages are added here as they land (M4).
import { Pages, whenReady } from '@/shared/core/Pages.ts';

export const pages = new Pages({});

whenReady(document, () => {
  document.documentElement.dataset['bundle'] = 'app';
  pages.mount(document);
});
