// The Android app's screens (static/android/android.js in the APK's assets,
// served by the app itself: android/MainActivity). They follow the phone's
// language and theme; the bridge to Java is CONTRACTS.md 6, version 2.
import './android.css';
import { App } from '@/android/App.ts';
import { bridge } from '@/shared/core/Bridge.ts';
import { DEFAULT_LANG, LANGS } from '@/shared/i18n/i18n.ts';

// The phone's language, when we speak it: the server is not there to say.
const wanted = (navigator.language || DEFAULT_LANG).slice(0, 2).toLowerCase();
document.documentElement.lang = wanted in LANGS ? wanted : 'en';

const root = document.getElementById('app')!;
void bridge().then(async (b) => {
  if (b === null) {
    root.textContent = 'agendouille: this page runs inside the Android app.';
    return;
  }
  await new App(document, root, b).start();
});
