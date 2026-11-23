import { Api } from '@/shared/core/Api.ts';
import { ApiPage } from '@/shared/components/ApiPage.ts';

/** The admin API, at /api/admin on the admin listener. */
export function adminApi(fetchFn?: (input: string, init: RequestInit) => Promise<Response>): Api {
  return new Api('/api/admin', fetchFn);
}

/**
 * A page of the admin UI: the server rendered the shell (top bar,
 * navigation, title); the page fills #admin-content from the API and
 * draws itself again after every change.
 */
export abstract class AdminPage extends ApiPage {
  constructor(doc: Document, api: Api) {
    super(doc, api, 'admin-content');
  }

  /** The {name} of the page's path (a user's page). */
  protected get name(): string {
    return this.root.dataset['name'] ?? '';
  }
}
