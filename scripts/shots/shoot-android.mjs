// The Android app's screens (src/android, built into android/assets/web),
// in Chromium at a phone's size, with a fake bridge in place of Java (its
// data: fixtures/android.json): the welcome, the manual setup, then the
// four tabs, a calendar's page and an approval.
// Usage: node shoot-android.mjs <android/assets/web> <out dir> [lang]
import puppeteer from 'puppeteer-core';
import { createServer } from 'node:http';
import { readFileSync } from 'node:fs';
import { extname, join } from 'node:path';

const [dir, out, lang = 'fr'] = process.argv.slice(2);
const types = { '.html': 'text/html', '.js': 'text/javascript', '.css': 'text/css', '.svg': 'image/svg+xml' };
const server = createServer((req, res) => {
  try {
    const p = req.url === '/' ? '/index.html' : req.url.split('?')[0];
    const body = readFileSync(join(dir, p));
    res.writeHead(200, { 'Content-Type': types[extname(p)] ?? 'application/octet-stream' });
    res.end(body);
  } catch {
    res.writeHead(404);
    res.end();
  }
}).listen(0);
const base = `http://127.0.0.1:${server.address().port}`;

// The fake bridge's data (fixtures/android.json), its relative times made absolute.
const absolute = (v) => {
  if (Array.isArray(v)) return v.map(absolute);
  if (v === null || typeof v !== 'object') return v;
  if ('minutes_ago' in v) return new Date(Date.now() - v.minutes_ago * 60000).toISOString();
  if ('in_minutes' in v) return new Date(Date.now() + v.in_minutes * 60000).toISOString();
  return Object.fromEntries(Object.entries(v).map(([k, x]) => [k, absolute(x)]));
};
const fixture = absolute(JSON.parse(readFileSync(new URL('./fixtures/android.json', import.meta.url), 'utf8')));

const browser = await puppeteer.launch({ executablePath: '/usr/bin/chromium', args: ['--no-sandbox', `--lang=${lang}`], headless: true });
const errors = [];
for (const [name, scheme, signedIn] of [['welcome', 'light', false], ['welcome-dark', 'dark', false], ['account', 'light', true], ['account-dark', 'dark', true], ['mode', 'light', true]]) {
  const page = await browser.newPage();
  page.on('pageerror', (e) => errors.push(`${name}: ${e.message}`));
  page.on('console', (m) => { if (m.type() === 'error' || m.type() === 'warn') errors.push(`${name} console: ${m.text()}`); });
  page.on('requestfailed', (r) => errors.push(`${name} failed: ${r.url()}`));
  await page.setViewport({ width: 390, height: 844, isMobile: true, hasTouch: true, deviceScaleFactor: 2 });
  await page.emulateMediaFeatures([{ name: 'prefers-color-scheme', value: scheme }]);
  await page.evaluateOnNewDocument((lang, signedIn, crashed, data, choosing) => {
    Object.defineProperty(navigator, 'language', { get: () => lang });
    const state = {
      account: signedIn ? data.account : null,
      permissions: { calendar: signedIn, camera: false, notifications: true, battery: false },
      xiaomi: true,
      update: choosing ? null : { version: 'dev-b6e228e', current: 'dev-9adc837', source: 'server', at: 1 },
      dev_updates: true,
      push: choosing ? { instant: false, mode_chosen: false, service: true, connected: false, hidden: false, users: [] } : { instant: true, mode_chosen: true, service: true, connected: true, hidden: false, users: ['DAVx⁵'] },
      sync: data.sync,
      calendars: signedIn ? data.calendars : [],
      crash: crashed ? data.crash : null,
    };
    window.addEventListener('load', () => setTimeout(() => {
      const ch = new MessageChannel();
      ch.port2.onmessage = (e) => {
        const r = JSON.parse(e.data);
        // The mode chosen, as Java keeps it.
        if (r.call === 'push.set' && r.args && 'instant' in r.args) Object.assign(state.push, { instant: r.args.instant === true, mode_chosen: true });
        const result = r.call === 'hello' ? data.hello
          : r.call === 'update.check' ? { current: data.hello.app_version, available: null }
          : r.call === 'api' ? { status: 200, body: JSON.stringify(data.api[`${r.args.method} ${r.args.path}`] ?? {}) }
            : state;
        ch.port2.postMessage(JSON.stringify({ id: r.id, ok: true, result }));
      };
      window.postMessage('agendouille-bridge', '*', [ch.port1]);
    }, 50));
  }, lang, signedIn, name === 'welcome-dark', fixture, name === 'mode');
  await page.goto(base + '/index.html', { waitUntil: 'networkidle0' });
  await new Promise((r) => setTimeout(r, 500));
  if (name === 'account') {
    // The update on offer: its dialog, then closed for the other shots.
    await page.screenshot({ path: `${out}/android-${lang}-update.png` });
    await page.evaluate(() => document.querySelector('dialog')?.close());
    await new Promise((r) => setTimeout(r, 200));
  }
  await page.screenshot({ path: `${out}/android-${lang}-${name}.png`, fullPage: true });
  if (name === 'account') {
    // The other tabs: Calendriers (the web app's page), Navigateur, and approving one.
    for (const [i, shot] of [[1, 'calendars'], [3, 'settings'], [2, 'devices']]) {
      await (await page.$$('nav button'))[i].click();
      await new Promise((r) => setTimeout(r, 500));
      await page.screenshot({ path: `${out}/android-${lang}-${shot}.png` });
      if (shot === 'settings') {
        // The bottom: the version, « Versions de développement », the update check.
        await page.evaluate(() => window.scrollTo(0, document.body.scrollHeight));
        await new Promise((r) => setTimeout(r, 300));
        await page.screenshot({ path: `${out}/android-${lang}-settings-bottom.png` });
        await page.evaluate(() => window.scrollTo(0, 0));
        // The permissions, each with its state and its way.
        await (await page.$('xpath/.//button[contains(., "Vérifier les autorisations")]')).click();
        await new Promise((r) => setTimeout(r, 300));
        await page.screenshot({ path: `${out}/android-${lang}-permissions.png`, fullPage: true });
        // The rows below the fold: instant sync's notification, autostart.
        await page.evaluate(() => window.scrollTo(0, document.body.scrollHeight));
        await new Promise((r) => setTimeout(r, 300));
        await page.screenshot({ path: `${out}/android-${lang}-permissions-bottom.png` });
        await page.evaluate(() => window.scrollTo(0, 0));
      }
      if (shot === 'calendars') {
        // One calendar's page.
        await (await page.$$('#app-content button.min-h-14'))[0].click();
        await new Promise((r) => setTimeout(r, 500));
        await page.screenshot({ path: `${out}/android-${lang}-calendar.png` });
        // Sharing it: the groups and the people, in one picker.
        await (await page.$('xpath/.//button[contains(., "Partager")]')).click();
        await new Promise((r) => setTimeout(r, 400));
        await page.screenshot({ path: `${out}/android-${lang}-share.png` });
        // On a touchscreen the search waits for a tap: no keyboard up front.
        const focused = await page.evaluate(() => `${document.activeElement?.tagName} coarse=${matchMedia('(pointer: coarse)').matches}`);
        if (focused.startsWith('INPUT') && focused.endsWith('coarse=true')) errors.push(`share: the search is focused on a touchscreen (${focused})`);        await page.keyboard.press('Escape');
        await new Promise((r) => setTimeout(r, 300));
      }
    }
    // The bottom of Appareils: the browsers, and « Déconnecter tous les navigateurs ».
    await page.evaluate(() => window.scrollTo(0, document.body.scrollHeight));
    await new Promise((r) => setTimeout(r, 300));
    await page.screenshot({ path: `${out}/android-${lang}-devices-bottom.png` });
    await page.evaluate(() => window.scrollTo(0, 0));
    await page.type('input[aria-label]', 'PKW-6VY');
    await (await page.$('form button[type="submit"]')).click();
    await new Promise((r) => setTimeout(r, 500));
    await page.screenshot({ path: `${out}/android-${lang}-approve.png` });
  }
  if (name === 'mode') {
    // « Standard » chosen: its permissions are only the calendar (and the camera, optional).
    await (await page.$('xpath/.//button[contains(., "Standard")]')).click();
    await new Promise((r) => setTimeout(r, 400));
    await (await page.$$('nav button'))[3].click();
    await new Promise((r) => setTimeout(r, 400));
    await (await page.$('xpath/.//button[contains(., "Vérifier les autorisations")]')).click();
    await new Promise((r) => setTimeout(r, 300));
    await page.screenshot({ path: `${out}/android-${lang}-permissions-standard.png`, fullPage: true });
  }
  if (name === 'welcome') {
    const manual = await page.$('[data-action="manual"]');
    if (manual === null) {
      errors.push(`${name}: no welcome rendered: ${await page.evaluate(() => document.body.innerHTML.slice(0, 300))}`);
      await page.close();
      continue;
    }
    await manual.click();
    await new Promise((r) => setTimeout(r, 300));
    await page.screenshot({ path: `${out}/android-${lang}-manual.png` });
  }
  await page.close();
}
await browser.close();
server.close();
console.log(errors.length ? errors.join('\n') : `android ${lang}: no page errors`);
