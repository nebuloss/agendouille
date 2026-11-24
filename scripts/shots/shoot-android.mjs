// The Android app's screens (src/android, built into android/assets/web),
// in Chromium at a phone's size, with a fake bridge in place of Java: the
// welcome, the manual setup, then the account with its calendars.
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

const browser = await puppeteer.launch({ executablePath: '/usr/bin/chromium', args: ['--no-sandbox', `--lang=${lang}`], headless: true });
const errors = [];
for (const [name, scheme, signedIn] of [['welcome', 'light', false], ['welcome-dark', 'dark', false], ['account', 'light', true], ['account-dark', 'dark', true]]) {
  const page = await browser.newPage();
  page.on('pageerror', (e) => errors.push(`${name}: ${e.message}`));
  page.on('console', (m) => { if (m.type() === 'error' || m.type() === 'warn') errors.push(`${name} console: ${m.text()}`); });
  page.on('requestfailed', (r) => errors.push(`${name} failed: ${r.url()}`));
  await page.setViewport({ width: 390, height: 844, isMobile: true, deviceScaleFactor: 2 });
  await page.emulateMediaFeatures([{ name: 'prefers-color-scheme', value: scheme }]);
  await page.evaluateOnNewDocument((lang, signedIn, crashed) => {
    Object.defineProperty(navigator, 'language', { get: () => lang });
    const state = {
      account: signedIn ? { server: 'https://agendouille.patapouf.club', host: 'agendouille.patapouf.club', username: 'lea', mode: 'enrolled' } : null,
      permissions: { calendar: signedIn, camera: false },
      sync: { state: 'idle', last_sync: new Date(Date.now() - 4 * 60000).toISOString(), error: null, events: 42 },
      calendars: signedIn ? [
        { id: 1, name: 'Famille', color: '#3b6ea5', sync: true, read_only: false },
        { id: 2, name: 'École', color: '#e8710a', sync: true, read_only: true },
        { id: 3, name: 'Perso de Léa', color: '#7b1fa2', sync: false, read_only: false },
      ] : [],
      crash: crashed ? 'agendouille dev on Google Pixel 8, Android 15 (35)\nthe page\'s renderer stopped: it crashed, priority 2' : null,
    };
    window.addEventListener('load', () => setTimeout(() => {
      const ch = new MessageChannel();
      ch.port2.onmessage = (e) => {
        const r = JSON.parse(e.data);
        // The family API the tabs call (MainActivity "api"), as the server answers.
        const api = {
          'GET /api/calendars': { calendars: [
            { ref: 'lea/famille', name: 'Famille', color: '#3b6ea5', owner: 'lea', access: 'owner', hidden: false, description: 'Les rendez-vous de toute la maison',
              shares: [{ target: 'g:famille', access: 'readwrite' }] },
            { ref: 'lea/perso', name: 'Perso de Léa', color: '#7b1fa2', owner: 'lea', access: 'owner', hidden: false, description: '', shares: [] },
            { ref: 'maman/ecole', name: 'École', color: '#e8710a', owner: 'maman', access: 'read', hidden: false, description: '' },
          ] },
          'GET /api/people': { users: [{ name: 'maman', display_name: 'Maman', color: '#e53935' }], groups: [{ name: 'famille', display_name: 'Famille', color: '' }] },
          'GET /api/approvals/PKW-6VY': { user_agent: 'Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0 Safari/537.36', ip: '192.168.1.24', numbers: [30, 72, 15], expires_at: new Date(Date.now() + 120000).toISOString() },
        };
        const result = r.call === 'hello' ? { app_bridge: 2, app_version: 'dev', android: 34 }
          : r.call === 'api' ? { status: 200, body: JSON.stringify(api[`${r.args.method} ${r.args.path}`] ?? {}) }
            : state;
        ch.port2.postMessage(JSON.stringify({ id: r.id, ok: true, result }));
      };
      window.postMessage('agendouille-bridge', '*', [ch.port1]);
    }, 50));
  }, lang, signedIn, name === 'welcome-dark');
  await page.goto(base + '/index.html', { waitUntil: 'networkidle0' });
  await new Promise((r) => setTimeout(r, 500));
  await page.screenshot({ path: `${out}/android-${lang}-${name}.png`, fullPage: true });
  if (name === 'account') {
    // The other tabs: Calendriers (the web app's page), Navigateur, and approving one.
    for (const [i, shot] of [[1, 'calendars'], [2, 'browser']]) {
      await (await page.$$('nav button'))[i].click();
      await new Promise((r) => setTimeout(r, 500));
      await page.screenshot({ path: `${out}/android-${lang}-${shot}.png`, fullPage: true });
    }
    await page.type('input[aria-label]', 'PKW-6VY');
    await (await page.$('form button[type="submit"]')).click();
    await new Promise((r) => setTimeout(r, 500));
    await page.screenshot({ path: `${out}/android-${lang}-approve.png` });
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
