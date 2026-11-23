// Renders the andouille icon as the Android launcher's PNGs, one per
// density, on dev-build (Chromium draws the SVG). The PNGs are committed.
// Usage: node android-icons.mjs <icon.svg> <android/res>
import puppeteer from 'puppeteer-core';
import { mkdirSync, readFileSync } from 'node:fs';

const [svgPath, res] = process.argv.slice(2);
const svg = readFileSync(svgPath, 'utf8');
const src = 'data:image/svg+xml;base64,' + Buffer.from(svg).toString('base64');
const browser = await puppeteer.launch({ executablePath: '/usr/bin/chromium', args: ['--no-sandbox'], headless: true });
const page = await browser.newPage();
for (const [density, px] of [['mdpi', 48], ['hdpi', 72], ['xhdpi', 96], ['xxhdpi', 144], ['xxxhdpi', 192]]) {
  await page.setViewport({ width: px, height: px });
  await page.setContent(`<body style="margin:0;background:transparent"><img src="${src}" width="${px}" height="${px}" style="display:block"></body>`);
  await new Promise((r) => setTimeout(r, 150));
  mkdirSync(`${res}/mipmap-${density}`, { recursive: true });
  await page.screenshot({ path: `${res}/mipmap-${density}/ic_launcher.png`, omitBackground: true });
}
await browser.close();
console.log('icons written');
