// The calendar page's hover states and the event editor, light and dark,
// on dev-build. Usage: node shoot-editor.mjs <enrollment URL> <out dir>
import puppeteer from 'puppeteer-core';

const [enroll, out] = process.argv.slice(2);
const base = new URL(enroll).origin;
const browser = await puppeteer.launch({ executablePath: '/usr/bin/chromium', args: ['--no-sandbox'], headless: true });
const wait = (ms) => new Promise((r) => setTimeout(r, ms));
const page = await browser.newPage();
// The pages follow Accept-Language; the shots are of the French UI unless
// SHOTS_LANG says otherwise (the English pass in run.sh).
await page.setExtraHTTPHeaders({ 'Accept-Language': process.env.SHOTS_LANG ?? 'fr' });
const errors = [];
page.on('pageerror', (e) => errors.push(`page: ${e.message}`));
await page.setViewport({ width: 1280, height: 860 });
await page.goto(enroll, { waitUntil: 'networkidle2' });
await Promise.all([page.waitForNavigation({ waitUntil: 'networkidle2' }), page.click('button[type=submit]')]);

for (const scheme of ['light', 'dark']) {
  await page.emulateMediaFeatures([{ name: 'prefers-color-scheme', value: scheme }]);
  await page.goto(base + '/', { waitUntil: 'networkidle2' });
  await wait(400);
  await page.screenshot({ path: `${out}/editor-${scheme}-page.png` });
  await page.hover('button.fixed, button[class*="bg-primary-container"]');
  await wait(300);
  await page.screenshot({ path: `${out}/editor-${scheme}-hover-fab.png` });
  const add = await page.$('button[aria-label^="Nouvel événement le"]');
  if (add) {
    await add.hover();
    await wait(300);
    await page.screenshot({ path: `${out}/editor-${scheme}-hover-add.png` });
  }
  await page.click('button[class*="bg-primary-container"]');
  await wait(500);
  await page.screenshot({ path: `${out}/editor-${scheme}-modal.png` });
  // Annuler with an empty title must close the editor.
  await (await page.$('xpath/.//dialog//button[contains(., "Annuler")]')).click();
  await wait(300);
  const open = await page.$('dialog[open]');
  console.log(`${scheme}: after Annuler the editor is ${open ? 'STILL OPEN' : 'closed'}`);
  if (open) await page.keyboard.press('Escape');
}
// The language menu open over the calendar (the event button stays under it).
await page.emulateMediaFeatures([{ name: 'prefers-color-scheme', value: 'light' }]);
await page.goto(base + '/', { waitUntil: 'networkidle2' });
await page.click('[aria-haspopup="menu"]');
await wait(300);
await page.screenshot({ path: `${out}/menu-over-fab.png`, clip: { x: 640, y: 0, width: 640, height: 260 } });
await page.keyboard.press('Escape');
// The language menu, open, and the theme choice on Moi, light and dark.
for (const scheme of ['light', 'dark']) {
  await page.emulateMediaFeatures([{ name: 'prefers-color-scheme', value: scheme }]);
  await page.goto(base + '/me', { waitUntil: 'networkidle2' });
  await page.click('[aria-haspopup="menu"]');
  await wait(300);
  await page.screenshot({ path: `${out}/menu-${scheme}.png`, clip: { x: 600, y: 0, width: 680, height: 560 } });
}
await page.emulateMediaFeatures([{ name: 'prefers-color-scheme', value: 'light' }]);
await page.setViewport({ width: 390, height: 844, isMobile: true });
// On a phone: the devices page.
await page.goto(base + '/devices', { waitUntil: 'networkidle2' });
await page.screenshot({ path: `${out}/phone-devices.png` });
// A DAVx5 device added, its password in the dialog.
await (await page.$('xpath/.//button[contains(., "DAVx5")]')).click();
await wait(300);
await (await page.$('xpath/.//dialog//button[@value="ok"]')).click();
await wait(700);
await page.screenshot({ path: `${out}/phone-device-added.png` });
await page.evaluate(() => { for (const b of document.querySelectorAll('dialog[open] div')) b.scrollTop = b.scrollHeight; });
await wait(200);
await page.screenshot({ path: `${out}/phone-device-added-password.png` });
await page.keyboard.press('Escape');
await page.screenshot({ path: `${out}/phone-header.png`, clip: { x: 0, y: 0, width: 390, height: 80 } });
await page.goto(base + '/', { waitUntil: 'networkidle2' });
await page.click('button[class*="bg-primary-container"]');
await wait(500);
await page.screenshot({ path: `${out}/editor-phone-modal.png` });
await browser.close();
console.log(errors.length ? errors.join('\n') : 'no page errors');
