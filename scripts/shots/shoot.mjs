// Screenshots of the admin UI, on dev-build (puppeteer-core from
// ~/shots/node_modules, as cafard). Usage: node shoot.mjs <admin URL> <password> <out dir>
import puppeteer from 'puppeteer-core';
import { mkdirSync } from 'node:fs';

const [base, password, out] = process.argv.slice(2);
mkdirSync(out, { recursive: true });
const browser = await puppeteer.launch({ executablePath: '/usr/bin/chromium', args: ['--no-sandbox'], headless: true });
const errors = [];
for (const [label, viewport] of [['desktop', { width: 1280, height: 860 }], ['phone', { width: 390, height: 844, isMobile: true }]]) {
  const page = await browser.newPage();
  page.on('console', (m) => { if (m.type() === 'error') errors.push(`${label} console: ${m.text()}`); });
  page.on('pageerror', (e) => errors.push(`${label} page: ${e.message}`));
  await page.setViewport(viewport);
  await page.goto(base + '/signin', { waitUntil: 'networkidle0' });
  await page.type('input[name=password]', password);
  await Promise.all([page.waitForNavigation({ waitUntil: 'networkidle0' }), page.click('button[type=submit]')]);
  for (const path of ['/', '/users', '/calendars', '/users/lea', '/users/lea/devices', '/users/lea/groups', '/users/lea/calendars', '/users/lea/signin', '/groups', '/groups/famille', '/groups/famille/members', '/users/lea/activity', '/calendars/maman~ecole', '/calendars/maman~ecole/shares', '/activity', '/settings', '/backups']) {
    await page.goto(base + path, { waitUntil: 'networkidle0' });
    await new Promise((r) => setTimeout(r, 300));
    const name = path.replaceAll('/', '_').replace(/^_/, '') || 'overview';
    await page.screenshot({ path: `${out}/${label}-${name}.png`, fullPage: label === 'desktop' });
  }
  // The notification centre, open.
  await page.goto(base + '/', { waitUntil: 'networkidle0' });
  await page.click('#bell-slot button');
  await new Promise((r) => setTimeout(r, 400));
  await page.screenshot({ path: `${out}/${label}-notices.png` });
  await page.keyboard.press('Escape');
  // Creating a user is a modal.
  await page.goto(base + '/users', { waitUntil: 'networkidle0' });
  const add = await page.$('xpath/.//button[contains(., "Nouvel utilisateur")]');
  if (add) {
    await add.click();
    await new Promise((r) => setTimeout(r, 400));
    await page.screenshot({ path: `${out}/${label}-new-user.png` });
    await page.keyboard.press('Escape');
  } else errors.push(`${label}: no "Nouvel utilisateur" button`);
  if (label === 'desktop') {
    await page.goto(base + '/users/lea/devices', { waitUntil: 'networkidle0' });
    for (const [label2, file] of [['QR pour un téléphone', 'enrolment-dialog'], ['Lien pour un navigateur', 'browser-link-dialog']]) {
      const [button] = await page.$$(`xpath/.//button[contains(., "${label2}")]`);
      if (!button) {
        errors.push(`no "${label2}" button`);
        continue;
      }
      await button.click();
      await new Promise((r) => setTimeout(r, 500));
      await page.screenshot({ path: `${out}/desktop-${file}.png` });
      await page.keyboard.press('Escape');
      await new Promise((r) => setTimeout(r, 500));
    }
    await page.screenshot({ path: `${out}/desktop-signin-links.png` });
  }
  await page.close();
}
await browser.close();
console.log(errors.length ? errors.join('\n') : 'no console errors');
