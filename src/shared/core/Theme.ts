/**
 * The reader's theme: follow the system, or always light, or always dark.
 * Kept in a cookie (agd_theme) so the server puts data-theme on <html>
 * before the first paint (webx.ThemeCookie); applied here at once too.
 * The pages carry Dark Reader's lock, since this theme is their own dark
 * mode: a reader who wants dark whatever the system chooses it here.
 */
export type Theme = 'auto' | 'light' | 'dark';

const COOKIE = 'agd_theme';

export function currentTheme(doc: Document = document): Theme {
  const v = doc.documentElement.dataset['theme'];
  return v === 'light' || v === 'dark' ? v : 'auto';
}

export function setTheme(theme: Theme, doc: Document = document): void {
  const root = doc.documentElement;
  if (theme === 'auto') {
    delete root.dataset['theme'];
    doc.cookie = `${COOKIE}=; Path=/; Max-Age=0; SameSite=Lax`;
  } else {
    root.dataset['theme'] = theme;
    doc.cookie = `${COOKIE}=${theme}; Path=/; Max-Age=${400 * 24 * 3600}; SameSite=Lax`;
  }
}

/** The next theme, for a single button that cycles through them. */
export function nextTheme(t: Theme): Theme {
  return t === 'auto' ? 'light' : t === 'light' ? 'dark' : 'auto';
}
