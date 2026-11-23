import { describe, expect, it } from 'vitest';
import { Pages, type Page } from '@/shared/core/Pages.ts';

function recorder(log: string[], name: string): () => Page {
  return () => ({
    start: () => log.push(`start ${name}`),
    dispose: () => log.push(`dispose ${name}`),
  });
}

describe('Pages', () => {
  it('mounts the page the body names', () => {
    const log: string[] = [];
    document.body.dataset['page'] = 'calendar';
    const pages = new Pages({ calendar: recorder(log, 'calendar') });
    expect(pages.mount(document)).not.toBeNull();
    expect(log).toEqual(['start calendar']);
  });

  it('mounts nothing for an unknown or missing id', () => {
    const pages = new Pages({ calendar: recorder([], 'calendar') });
    document.body.dataset['page'] = 'not-a-page';
    expect(pages.mount(document)).toBeNull();
    delete document.body.dataset['page'];
    expect(pages.mount(document)).toBeNull();
  });

  it('does not treat Object.prototype names as pages', () => {
    const pages = new Pages({});
    document.body.dataset['page'] = 'constructor';
    expect(pages.mount(document)).toBeNull();
  });

  it('disposes the previous page before mounting the next', () => {
    const log: string[] = [];
    const pages = new Pages({ a: recorder(log, 'a'), b: recorder(log, 'b') });
    document.body.dataset['page'] = 'a';
    pages.mount(document);
    document.body.dataset['page'] = 'b';
    pages.mount(document);
    expect(log).toEqual(['start a', 'dispose a', 'start b']);
  });
});
