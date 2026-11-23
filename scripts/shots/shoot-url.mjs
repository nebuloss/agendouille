// One page, as a phone sees it, to check a server-rendered page on screen.
// Usage: node shoot-url.mjs <url> <out.png> [accept-language]
import puppeteer from 'puppeteer-core';

const [url, out, lang] = process.argv.slice(2);
const browser = await puppeteer.launch({ executablePath: '/usr/bin/chromium', args: ['--no-sandbox'], headless: true });
const page = await browser.newPage();
await page.setUserAgent('Mozilla/5.0 (Linux; Android 14; Pixel 8) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0 Mobile Safari/537.36');
await page.setExtraHTTPHeaders({ 'Accept-Language': lang ?? 'fr' });
await page.setViewport({ width: 390, height: 844, isMobile: true, deviceScaleFactor: 2 });
await page.goto(url, { waitUntil: 'networkidle2' });
await new Promise((r) => setTimeout(r, 400));
await page.screenshot({ path: out, fullPage: true });
await browser.close();
console.log(`shot ${url}`);
