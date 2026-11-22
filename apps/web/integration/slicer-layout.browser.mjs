// Read-only real-project check. All APIs except inspection use synthetic fixtures.
// Usage: node integration/slicer-layout.browser.mjs FILE LOOPBACK_INSPECT_URL PLATES ITEMS
import { chromium, expect } from '@playwright/test';
import { createServer } from 'vite';
import { installApiFixtures } from '../visual/catalog-fixtures.js';

const [file, endpoint, plateCount, itemCount] = process.argv.slice(2);
if (!file || !endpoint || !plateCount || !itemCount) throw new Error('Provide file, loopback inspection URL and expected plate/item counts');
const target = new URL(endpoint);
if (target.protocol !== 'http:' || target.hostname !== '127.0.0.1') throw new Error('Inspection handler must be on loopback');
const server = await createServer({ server: { host: '127.0.0.1', port: 0, open: false, proxy: { '/api': { target: target.origin, rewrite: path => path.replace(/^\/api/, '') } } } });
let browser;
try {
  await server.listen();
  browser = await chromium.launch();
  const context = await browser.newContext();
  await context.addInitScript(() => localStorage.setItem('maker-track-studio-locale', 'en'));
  const page = await context.newPage();
  const errors = [];
  page.on('pageerror', error => errors.push(error.message));
  await installApiFixtures(context);
  await page.route('**/api/v2/production/slicer/inspect', route => route.continue());
  let writes = 0;
  await page.route('**/api/v2/production/jobs', async route => { writes++; await route.abort(); });
  await page.goto(`http://127.0.0.1:${server.httpServer.address().port}/#/print-jobs`);
  await page.getByRole('button', { name: 'Plan production' }).click();
  const responsePromise = page.waitForResponse(response => response.url().includes('/v2/production/slicer/inspect'));
  await page.getByLabel('Import slicer estimates').setInputFiles(file);
  const response = await responsePromise;
  expect(response.status()).toBe(200);
  const inspected = await response.json();
  await expect(page.getByText('3MF plate 1', { exact: false })).toBeVisible();
  expect(inspected.plates).toHaveLength(Number(plateCount));
  expect(inspected.plates.reduce((sum, plate) => sum + plate.items.length, 0)).toBe(Number(itemCount));
  await expect(page.getByLabel('planned', { exact: true })).toHaveCount(Number(itemCount) + 1);
  for (const plate of inspected.plates) {
    expect(plate.estimated_time_minutes).toBeNull();
    expect(plate.materials).toEqual([]);
  }
  expect(errors).toEqual([]);
  expect(writes).toBe(0);
  console.log(JSON.stringify({ passed: true, plates: Number(plateCount), modelInstances: Number(itemCount), estimatesUnknown: true, productionWrites: writes }));
} finally {
  await browser?.close();
  await server.close();
}
