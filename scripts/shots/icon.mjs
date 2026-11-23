// Renders the andouille icon at the sizes a browser uses, on dev-build.
// Usage: node icon.mjs <icon.svg> <out.png>
import puppeteer from 'puppeteer-core';
import { readFileSync } from 'node:fs';

const [svgPath, out] = process.argv.slice(2);
const svg = readFileSync(svgPath, 'utf8');
const src = 'data:image/svg+xml;base64,' + Buffer.from(svg).toString('base64');
const browser = await puppeteer.launch({ executablePath: '/usr/bin/chromium', args: ['--no-sandbox'], headless: true });
const page = await browser.newPage();
await page.setViewport({ width: 420, height: 280 });
await page.setContent(`<body style="margin:0;padding:12px;background:#ddd;display:flex;gap:16px;align-items:flex-end">
  <img src="${src}" width="256" height="256"><img src="${src}" width="64" height="64">
  <img src="${src}" width="32" height="32"><img src="${src}" width="16" height="16"></body>`);
await new Promise((r) => setTimeout(r, 300));
await page.screenshot({ path: out });
await browser.close();
