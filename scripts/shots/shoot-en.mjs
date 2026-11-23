// The English pass: a few pages of both UIs with Accept-Language: en, to
// check the translation reads and fits. Usage: node shoot-en.mjs <enrol> <out> <admin cookie jar>
import puppeteer from 'puppeteer-core';
import { readFileSync } from 'node:fs';

const [enrol, out, jar] = process.argv.slice(2);
const base = new URL(enrol).origin;
const browser = await puppeteer.launch({ executablePath: '/usr/bin/chromium', args: ['--no-sandbox'], headless: true });
const wait = (ms) => new Promise((r) => setTimeout(r, ms));
const page = await browser.newPage();
const errors = [];
page.on('pageerror', (e) => errors.push(`page: ${e.message}`));
await page.setExtraHTTPHeaders({ 'Accept-Language': 'en-GB,en' });
await page.setViewport({ width: 1280, height: 860 });
await page.goto(enrol, { waitUntil: 'networkidle2' });
await page.screenshot({ path: `${out}/en-enrol.png` });
await Promise.all([page.waitForNavigation({ waitUntil: 'networkidle2' }), page.click('button[type=submit]')]);
for (const [name, path] of [['calendar', '/'], ['devices', '/devices'], ['me', '/me'], ['calendars', '/calendars']]) {
  await page.goto(base + path, { waitUntil: 'networkidle2' });
  await wait(400);
  await page.screenshot({ path: `${out}/en-app-${name}.png`, fullPage: true });
}
await page.goto(base + '/', { waitUntil: 'networkidle2' });
await page.click('button[class*="bg-primary-container"]');
await wait(500);
await page.screenshot({ path: `${out}/en-app-editor.png` });

// The admin UI with the cookie run.sh signed in with.
const cookies = readFileSync(jar, 'utf8').split('\n').filter((l) => l.includes('\t') && !l.startsWith('# ') ).map((l) => l.replace(/^#HttpOnly_/, '').split('\t'))
  .map((c) => ({ name: c[5], value: c[6], domain: '127.0.0.1', path: c[2] }));
const admin = 'http://127.0.0.1:18892';
await page.setCookie(...cookies.map((c) => ({ ...c, url: admin })));
for (const [name, path] of [['overview', '/'], ['users', '/users'], ['user', '/users/lea'], ['activity', '/activity'], ['settings', '/settings']]) {
  await page.goto(admin + path, { waitUntil: 'networkidle2' });
  await wait(500);
  await page.screenshot({ path: `${out}/en-admin-${name}.png`, fullPage: true });
}
await browser.close();
console.log(errors.length ? errors.join('\n') : 'en: no page errors');
