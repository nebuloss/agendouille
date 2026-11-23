/**
 * Translations (DECISIONS: languages). Every string a person reads comes
 * from a catalog: one per module, in a sibling `*.msg.ts`, French and
 * English side by side, checked by the compiler (a key missing in one
 * language does not build).
 *
 * The page's language is <html lang>, which the server sets from the
 * reader's choice (the agd_lang cookie) or the browser's languages.
 *
 *   export const t = messages({
 *     fr: { save: 'Enregistrer', devices: { one: '{count} appareil', other: '{count} appareils' } },
 *     en: { save: 'Save', devices: { one: '{count} device', other: '{count} devices' } },
 *   });
 *   t('save'); t('devices', { count: 3 });
 *
 * A message is a string with {name} placeholders, or plural forms chosen
 * by the `count` parameter (Intl.PluralRules: one, other, and others where
 * a language has them).
 */

export type Lang = 'fr' | 'en';

/** The languages offered, each named in itself. */
export const LANGS: Readonly<Record<Lang, string>> = { fr: 'Français', en: 'English' };

export const DEFAULT_LANG: Lang = 'fr';

/** Plural forms; other is required, as in Intl.PluralRules. */
export interface Plural {
  readonly zero?: string;
  readonly one?: string;
  readonly two?: string;
  readonly few?: string;
  readonly many?: string;
  readonly other: string;
}

export type Message = string | Plural;
export type Params = Readonly<Record<string, string | number>>;

/** The page's language, from <html lang>. */
export function lang(doc: Document = document): Lang {
  const l = (doc.documentElement.lang || DEFAULT_LANG).slice(0, 2).toLowerCase();
  return l in LANGS ? (l as Lang) : DEFAULT_LANG;
}

/** The locale for Intl formats (dates, numbers) in the page's language. */
export function locale(doc: Document = document): string {
  return lang(doc) === 'en' ? 'en-GB' : 'fr-FR';
}

function fill(text: string, params: Params | undefined): string {
  if (params === undefined) return text;
  return text.replace(/\{(\w+)\}/g, (all, name: string) => {
    const v = params[name];
    if (v === undefined) return all;
    return typeof v === 'number' ? v.toLocaleString(locale()) : v;
  });
}

/** Renders one message in a language (exported for the tests). */
export function render(m: Message, l: Lang, params?: Params): string {
  if (typeof m === 'string') return fill(m, params);
  const n = Number(params?.['count'] ?? 0);
  const form = n === 0 && m.zero !== undefined ? 'zero' : new Intl.PluralRules(l === 'en' ? 'en' : 'fr').select(n);
  return fill(m[form as keyof Plural] ?? m.other, params);
}

/**
 * A module's catalog. French is the reference: English must have exactly
 * its keys. Returns t(key, params) in the page's language.
 */
export function messages<const F extends Readonly<Record<string, Message>>>(catalog: {
  readonly fr: F;
  readonly en: { readonly [K in keyof F]: Message };
}): (key: keyof F & string, params?: Params) => string {
  return (key, params) => {
    const l = lang();
    const m = (catalog[l] as Readonly<Record<string, Message>>)[key] ?? catalog.fr[key];
    return m === undefined ? key : render(m, l, params);
  };
}

const COOKIE = 'agd_lang';

/** Chooses the language: a cookie the server reads, then a reload. */
export function setLang(l: Lang, doc: Document = document): void {
  doc.cookie = `${COOKIE}=${l}; Path=/; Max-Age=${400 * 24 * 3600}; SameSite=Lax`;
  doc.location.reload();
}
