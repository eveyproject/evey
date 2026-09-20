// Rasterize the Play Store SVG sources to PNG with the Playwright chromium that
// ships with tests/e2e - the only renderer available on this machine.
//   node evey-brand/play/render.mjs
import { chromium } from '../../tests/e2e/node_modules/playwright/index.mjs';
import { readFileSync, writeFileSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const here = dirname(fileURLToPath(import.meta.url));
const repo = resolve(here, '../..');

// Inter is not installed system-wide; embed the web app's own woff2 so the
// tagline renders with the product font instead of a fallback.
const font = readFileSync(resolve(repo, 'frontend/apps/web/public/fonts/inter-latin.woff2')).toString('base64');

const TARGETS = [
  { svg: 'play-icon-512.svg', png: 'play-icon-512.png', w: 512, h: 512 },
  { svg: 'play-feature-graphic.svg', png: 'play-feature-graphic-1024x500.png', w: 1024, h: 500 },
  { svg: 'play-tv-banner.svg', png: 'play-tv-banner-1280x720.png', w: 1280, h: 720 },
];

const browser = await chromium.launch();
for (const t of TARGETS) {
  const svg = readFileSync(resolve(here, t.svg), 'utf8');
  const page = await browser.newPage({ viewport: { width: t.w, height: t.h }, deviceScaleFactor: 1 });
  await page.setContent(`<!doctype html><meta charset="utf-8"><style>
    @font-face{font-family:'Inter';font-weight:400 700;font-style:normal;
      src:url(data:font/woff2;base64,${font}) format('woff2')}
    html,body{margin:0;padding:0;background:#000}
    svg{display:block}
  </style>${svg}`);
  await page.evaluate(() => document.fonts.ready);
  const buf = await page.locator('svg').screenshot({ omitBackground: false });
  writeFileSync(resolve(here, t.png), buf);
  await page.close();
  console.log(`${t.png} ${t.w}x${t.h}`);
}
await browser.close();
