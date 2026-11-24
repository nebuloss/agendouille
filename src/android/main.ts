// The Android app's screens (static/android/android.js in the APK's assets,
// served by the app itself: android/MainActivity). They follow the phone's
// language and theme; the bridge to Java is CONTRACTS.md 6, version 2.
import './android.css';
import { App } from '@/android/App.ts';
import { ownValidation } from '@/shared/components/Validation.ts';
import { bridge } from '@/shared/core/Bridge.ts';
import { DEFAULT_LANG, LANGS } from '@/shared/i18n/i18n.ts';

// Field errors in our words and design, not the browser's bubbles.
ownValidation(document);

// The language and theme chosen in Réglages, else the phone's: the server
// is not there to say.
const saved = (key: string): string | null => {
  try {
    return globalThis.localStorage?.getItem(key) ?? null;
  } catch {
    return null;
  }
};
const cookie = (/(?:^|; )agd_lang=([a-z]{2})/.exec(document.cookie) ?? [])[1];
const wanted = (saved('agd_lang') ?? cookie ?? navigator.language ?? DEFAULT_LANG).slice(0, 2).toLowerCase();
document.documentElement.lang = wanted in LANGS ? wanted : 'en';
const theme = saved('agd_theme');
if (theme === 'light' || theme === 'dark') document.documentElement.dataset['theme'] = theme;

const root = document.getElementById('app')!;
void bridge().then(async (b) => {
  if (b === null) {
    root.textContent = 'agendouille: this page runs inside the Android app.';
    return;
  }
  await new App(document, root, b).start();
});
