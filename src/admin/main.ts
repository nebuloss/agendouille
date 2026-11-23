// The admin UI's bundle (static/admin/admin.js), served on the admin
// listener only. Pages are added here as they land (M3).
import { Pages, whenReady } from '@/shared/core/Pages.ts';

export const pages = new Pages({});

whenReady(document, () => {
  document.documentElement.dataset['bundle'] = 'admin';
  pages.mount(document);
});
