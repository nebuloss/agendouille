// One page, as a phone (or a computer) sees it, to check a server-rendered
// page on screen. The server tells them apart by User-Agent too.
// Usage: node shoot-url.mjs <url> <out.png> [accept-language] [phone|desktop]
import puppeteer from 'puppeteer-core';

const [url, out, lang, device = 'phone'] = process.argv.slice(2);
const browser = await puppeteer.launch({ executablePath: '/usr/bin/chromium', args: ['--no-sandbox'], headless: true });
const page = await browser.newPage();
if (device === 'desktop') {
  await page.setUserAgent('Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0 Safari/537.36');
  await page.setViewport({ width: 1280, height: 900 });
} else {
  await page.setUserAgent('Mozilla/5.0 (Linux; Android 14; Pixel 8) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0 Mobile Safari/537.36');
  await page.setViewport({ width: 390, height: 844, isMobile: true, deviceScaleFactor: 2 });
}
await page.setExtraHTTPHeaders({ 'Accept-Language': lang ?? 'fr' });
await page.goto(url, { waitUntil: 'networkidle2' });
await new Promise((r) => setTimeout(r, 400));
await page.screenshot({ path: out, fullPage: true });
await browser.close();
console.log(`shot ${url}`);
