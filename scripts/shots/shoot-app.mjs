// Screenshots of the family web app, on dev-build. Signs in with an
// enrollment link, then shoots every page and the dialogs. networkidle2,
// not 0: the open /api/live stream keeps one connection busy.
// Usage: node shoot-app.mjs <enrollment URL> <out dir>
import puppeteer from 'puppeteer-core';

const [enroll, out, pass] = process.argv.slice(2);
const base = new URL(enroll).origin;
const browser = await puppeteer.launch({ executablePath: '/usr/bin/chromium', args: ['--no-sandbox'], headless: true });
const errors = [];
const wait = (ms) => new Promise((r) => setTimeout(r, ms));
const page = await browser.newPage();
// The pages follow Accept-Language; the shots are of the French UI unless
// SHOTS_LANG says otherwise (the English pass in run.sh).
await page.setExtraHTTPHeaders({ 'Accept-Language': process.env.SHOTS_LANG ?? 'fr' });
page.on('console', (m) => { if (m.type() === 'error') errors.push(`console: ${m.text()}`); });
page.on('pageerror', (e) => errors.push(`page: ${e.message}`));
await page.emulateTimezone('America/New_York'); // the family zone wins over the browser's
// The certificate pass (the mode is not password): sign in, add a DAVx5
// device, shoot the dialog with its certificate, and stop.
if (pass === 'certs') {
  await page.setViewport({ width: 1280, height: 900 });
  await page.goto(enroll, { waitUntil: 'networkidle2' });
  await Promise.all([page.waitForNavigation({ waitUntil: 'networkidle2' }), page.click('button[type=submit]')]);
  await page.goto(base + '/devices', { waitUntil: 'networkidle2' });
  await wait(400);
  await page.screenshot({ path: `${out}/app-devices-certs.png` });
  await (await page.$('xpath/.//button[contains(., "Android (DAVx5)")]')).click();
  await wait(300);
  await (await page.$('xpath/.//dialog//button[contains(., "Ajouter")]')).click();
  await wait(600);
  await page.screenshot({ path: `${out}/app-device-added-certs.png` });
  await browser.close();
  console.log(errors.length ? errors.join('\n') : 'certs: no console errors');
  process.exit(0);
}

// Signed out first: the sign-in page, its QR and its countdown.
await page.setViewport({ width: 390, height: 844, isMobile: true });
await page.goto(base + '/', { waitUntil: 'networkidle2' });
await new Promise((r) => setTimeout(r, 1200));
await page.screenshot({ path: `${out}/app-signin.png` });
await page.setViewport({ width: 1280, height: 860 });
await page.goto(enroll, { waitUntil: 'networkidle2' });
await Promise.all([page.waitForNavigation({ waitUntil: 'networkidle2' }), page.click('button[type=submit]')]);

const shot = async (name, full = false) => {
  await wait(400);
  await page.screenshot({ path: `${out}/app-${name}.png`, fullPage: full });
};
const view = async (v) => {
  await page.click(`.seg[data-view="${v}"]`);
  await wait(400);
};
const close = async () => {
  await page.keyboard.press('Escape');
  await wait(200);
};

for (const [label, viewport] of [['desktop', { width: 1280, height: 860 }], ['phone', { width: 390, height: 844, isMobile: true }]]) {
  await page.setViewport(viewport);
  await page.goto(base + '/', { waitUntil: 'networkidle2' });
  for (const v of ['week', 'month', 'agenda']) {
    await view(v);
    await shot(`${label}-${v}`);
  }
  for (const p of ['calendars', 'devices', 'me']) {
    await page.goto(`${base}/${p}`, { waitUntil: 'networkidle2' });
    await shot(`${label}-${p}`, label === 'desktop');
  }
}

// Dialogs, on the desktop.
await page.setViewport({ width: 1280, height: 860 });
await page.goto(base + '/', { waitUntil: 'networkidle2' });
await view('week');
const judo = await page.$('xpath/.//button[contains(@class,"event") and contains(., "Judo")]');
if (judo) {
  await judo.click();
  await shot('sheet');
  const edit = await page.$('xpath/.//dialog//button[contains(., "Modifier")]');
  if (edit) {
    await edit.click();
    await shot('editor');
    const save = await page.$('xpath/.//dialog//button[contains(., "Enregistrer")]');
    await save.click();
    await shot('scope');
    await close();
  } else errors.push('no Modifier on the Judo sheet (is the shared calendar writable?)');
} else errors.push('no Judo in the week');
await page.goto(base + '/devices', { waitUntil: 'networkidle2' });
const add = await page.$('xpath/.//button[contains(., "Android (DAVx5)")]');
await add.click();
await wait(200);
await (await page.$('xpath/.//dialog//button[contains(., "Ajouter")]')).click();
await shot('device-added');
await browser.close();
console.log(errors.length ? errors.join('\n') : 'app: no console errors');
