import { afterEach, describe, expect, it } from 'vitest';
import { lang, locale, messages, render } from '@/shared/i18n/i18n.ts';

const t = messages({
  fr: { save: 'Enregistrer', hello: 'Bonjour {name}', devices: { zero: 'Aucun appareil', one: '{count} appareil', other: '{count} appareils' } },
  en: { save: 'Save', hello: 'Hello {name}', devices: { zero: 'No devices', one: '{count} device', other: '{count} devices' } },
});

describe('i18n', () => {
  afterEach(() => {
    document.documentElement.lang = 'fr';
  });

  it('follows <html lang>, French by default', () => {
    document.documentElement.lang = '';
    expect(lang()).toBe('fr');
    document.documentElement.lang = 'en';
    expect(lang()).toBe('en');
    expect(locale()).toBe('en-GB');
    document.documentElement.lang = 'de';
    expect(lang()).toBe('fr');
  });

  it('fills placeholders and picks plural forms per language', () => {
    expect(t('save')).toBe('Enregistrer');
    expect(t('hello', { name: 'Léa' })).toBe('Bonjour Léa');
    expect(t('devices', { count: 0 })).toBe('Aucun appareil');
    expect(t('devices', { count: 1 })).toBe('1 appareil');
    expect(t('devices', { count: 2 })).toBe('2 appareils');
    document.documentElement.lang = 'en';
    expect(t('save')).toBe('Save');
    expect(t('devices', { count: 1 })).toBe('1 device');
    expect(t('devices', { count: 1500 })).toBe('1,500 devices');
  });

  it('French counts 0 and 1 as singular, English only 1', () => {
    const m = { one: 'un', other: 'plusieurs' };
    expect(render(m, 'fr', { count: 1 })).toBe('un');
    expect(render(m, 'en', { count: 1 })).toBe('un');
    expect(render(m, 'en', { count: 0 })).toBe('plusieurs');
    expect(render(m, 'fr', { count: 0 })).toBe('un');
  });
});
