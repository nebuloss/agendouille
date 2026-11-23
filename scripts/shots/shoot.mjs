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
  await page.goto(base + '/admin/signin', { waitUntil: 'networkidle0' });
  await page.type('input[name=password]', password);
  await Promise.all([page.waitForNavigation({ waitUntil: 'networkidle0' }), page.click('button[type=submit]')]);
  for (const path of ['/admin', '/admin/users', '/admin/users/lea', '/admin/groups', '/admin/calendars', '/admin/activity', '/admin/settings', '/admin/backups']) {
    await page.goto(base + path, { waitUntil: 'networkidle0' });
    await new Promise((r) => setTimeout(r, 300));
    const name = path.replaceAll('/', '_').replace(/^_/, '') || 'root';
    await page.screenshot({ path: `${out}/${label}-${name}.png`, fullPage: label === 'desktop' });
  }
  if (label === 'desktop') {
    await page.goto(base + '/admin/users/lea', { waitUntil: 'networkidle0' });
    const [button] = await page.$$('xpath/.//button[contains(., "Créer un QR")]');
    if (button) {
      await button.click();
      await new Promise((r) => setTimeout(r, 500));
      await page.screenshot({ path: `${out}/desktop-enrolment-dialog.png` });
    } else errors.push('no enrolment button');
  }
  await page.close();
}
await browser.close();
console.log(errors.length ? errors.join('\n') : 'no console errors');
