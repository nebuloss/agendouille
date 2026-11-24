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
// The pages follow Accept-Language; the shots are of the French UI unless
// SHOTS_LANG says otherwise (the English pass in run.sh).
await page.setExtraHTTPHeaders({ 'Accept-Language': process.env.SHOTS_LANG ?? 'fr' });
  page.on('console', (m) => { if (m.type() === 'error') errors.push(`${label} console: ${m.text()}`); });
  page.on('pageerror', (e) => errors.push(`${label} page: ${e.message}`));
  await page.setViewport(viewport);
  await page.goto(base + '/signin', { waitUntil: 'networkidle2' });
  await page.type('input[name=password]', password);
  await Promise.all([page.waitForNavigation({ waitUntil: 'networkidle2' }), page.click('button[type=submit]')]);
  for (const path of ['/', '/users', '/calendars', '/users/lea', '/users/lea/devices', '/users/lea/groups', '/users/lea/calendars', '/users/lea/signin', '/groups', '/groups/famille', '/groups/famille/members', '/users/lea/activity', '/calendars/maman~ecole', '/calendars/maman~ecole/shares', '/activity', '/settings', '/backups']) {
    await page.goto(base + path, { waitUntil: 'networkidle2' });
    await new Promise((r) => setTimeout(r, 300));
    const name = path.replaceAll('/', '_').replace(/^_/, '') || 'overview';
    await page.screenshot({ path: `${out}/${label}-${name}.png`, fullPage: label === 'desktop' });
  }
  // The path bar and the rail start on the same line, and the rail level
  // with the page, at every width (the header and the grid are built alike).
  await page.goto(base + '/users', { waitUntil: 'networkidle2' });
  for (const width of label === 'desktop' ? [800, 1280, 1920, 2560] : [390]) {
    await page.setViewport({ ...viewport, width });
    await new Promise((r) => setTimeout(r, 200));
    const m = await page.evaluate(() => {
      const box = (sel) => document.querySelector(sel)?.getBoundingClientRect();
      const trail = box('nav.trail'), rail = box('nav.rail'), content = box('#admin-content');
      return trail && rail && content ? { trailLeft: trail.left, railLeft: rail.left, trailRight: trail.right, contentRight: content.right, railTop: rail.top, contentTop: content.top } : null;
    });
    if (m === null) errors.push(`${label} ${width}: no trail, rail or content`);
    else if (Math.abs(m.trailLeft - m.railLeft) > 1 || Math.abs(m.trailRight - m.contentRight) > 1 || Math.abs(m.railTop - m.contentTop) > 1) {
      errors.push(`${label} ${width}: misaligned ${JSON.stringify(m)}`);
    }
  }
  await page.setViewport(viewport);
  // The notification centre, open.
  await page.goto(base + '/', { waitUntil: 'networkidle2' });
  await page.click('#bell-slot button');
  await new Promise((r) => setTimeout(r, 400));
  await page.screenshot({ path: `${out}/${label}-notices.png` });
  await page.keyboard.press('Escape');
  // Creating a user is a modal.
  await page.goto(base + '/users', { waitUntil: 'networkidle2' });
  const add = await page.$('xpath/.//button[contains(., "Ajouter un utilisateur")]');
  if (add) {
    await add.click();
    await new Promise((r) => setTimeout(r, 400));
    await page.screenshot({ path: `${out}/${label}-new-user.png` });
    await page.keyboard.press('Escape');
  } else errors.push(`${label}: no "Ajouter un utilisateur" button`);
  if (label === 'desktop') {
    await page.goto(base + '/users/lea/devices', { waitUntil: 'networkidle2' });
    await new Promise((r) => setTimeout(r, 1200)); // a live reload from the step before lands first
    for (const [label2, file] of [['Créer un QR code', 'enrollment-dialog'], ['Créer un lien pour navigateur', 'browser-link-dialog']]) {
      const [button] = await page.$$(`xpath/.//button[contains(., "${label2}")]`);
      if (!button) {
        errors.push(`no "${label2}" button`);
        continue;
      }
      await button.click();
      await new Promise((r) => setTimeout(r, 500));
      await page.screenshot({ path: `${out}/desktop-${file}.png` });
      await page.keyboard.press('Escape');
      await new Promise((r) => setTimeout(r, 1200)); // the new link's notice reloads the page
    }
    await page.screenshot({ path: `${out}/desktop-signin-links.png` });
    // The picker: a calendar's "add members", with a search typed.
    await page.goto(base + '/calendars/maman~ecole/shares', { waitUntil: 'networkidle2' });
    await new Promise((r) => setTimeout(r, 500));
    const addm = await page.$('xpath/.//button[contains(., "Ajouter un membre")]');
    if (addm) {
      await addm.click();
      await new Promise((r) => setTimeout(r, 400));
      await page.screenshot({ path: `${out}/desktop-picker.png` });
      await page.type('dialog input[type=search]', 'ma');
      await new Promise((r) => setTimeout(r, 200));
      await page.screenshot({ path: `${out}/desktop-picker-search.png` });
      await page.keyboard.press('Escape');
    } else errors.push('no "Ajouter un membre" button');
    // On a phone, the rail folded then unfolded.
    await page.setViewport({ width: 390, height: 844, isMobile: true });
    await page.goto(base + '/users/lea', { waitUntil: 'networkidle2' });
    await new Promise((r) => setTimeout(r, 400));
    await page.screenshot({ path: `${out}/phone-rail-folded.png` });
    await page.click('.rail-toggle');
    await new Promise((r) => setTimeout(r, 300));
    await page.screenshot({ path: `${out}/phone-rail-open.png` });
    await page.click('.rail-toggle');
    await page.setViewport({ width: 1280, height: 860 });
    // The devices page (sign-in policy), and a field error of ours.
    await page.goto(base + '/users/lea/devices', { waitUntil: 'networkidle2' });
    await new Promise((r) => setTimeout(r, 500));
    await page.screenshot({ path: `${out}/desktop-user-devices.png`, fullPage: true });
    const addv = await page.$('xpath/.//button[contains(., "Ajouter un appareil")]');
    if (addv) {
      await addv.click();
      await new Promise((r) => setTimeout(r, 300));
      await (await page.$('xpath/.//dialog//button[@type="submit"]')).click();
      await new Promise((r) => setTimeout(r, 300));
      await page.screenshot({ path: `${out}/desktop-field-error.png` });
      await page.keyboard.press('Escape');
    }
    // The admin adds a device for someone: its manual settings.
    await page.goto(base + '/users/lea/devices', { waitUntil: 'networkidle2' });
    await new Promise((r) => setTimeout(r, 500));
    const addd = await page.$('xpath/.//button[contains(., "Ajouter un appareil")]');
    if (addd) {
      await addd.click();
      await new Promise((r) => setTimeout(r, 300));
      await page.type('dialog input[required]', 'Portable de Léa');
      await (await page.$('xpath/.//dialog//button[@type="submit"]')).click();
      await new Promise((r) => setTimeout(r, 700));
      await page.screenshot({ path: `${out}/desktop-device-added.png` });
      await page.keyboard.press('Escape');
    } else errors.push('no "Ajouter un appareil" button');
    // Deleting a user who owns a calendar: its new owner is chosen.
    await page.goto(base + '/users/lea', { waitUntil: 'networkidle2' });
    await new Promise((r) => setTimeout(r, 500));
    const del = await page.$('xpath/.//button[contains(., "Supprimer l")]');
    if (del) {
      await del.click();
      await new Promise((r) => setTimeout(r, 400));
      await page.screenshot({ path: `${out}/desktop-delete-user.png` });
      await page.keyboard.press('Escape');
    } else errors.push('no delete user button');
  }
  await page.close();
}
await browser.close();
console.log(errors.length ? errors.join('\n') : 'no console errors');
