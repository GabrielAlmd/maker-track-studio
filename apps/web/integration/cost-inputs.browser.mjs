// Real Chromium form submission with explicit synthetic HTTP fixtures. This
// exercises native validity; it is not an end-to-end test of the Go backend.
import { chromium, expect } from '@playwright/test';
import { createServer } from 'vite';
import { writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';

const server = await createServer({ server: { host: '127.0.0.1', port: 0, open: false } });
let browser;
const results = [];
try {
  await server.listen();
  const origin = `http://127.0.0.1:${server.httpServer.address().port}`;
  browser = await chromium.launch();
  for (const hours of [0, 0.5, 0.25, 1.5]) {
    const context = await browser.newContext({ locale: 'en-GB' });
    let submitted;
    const unexpectedRequests = [];
    const errors = [];
    let validity;
    try {
      await context.addInitScript(() => localStorage.setItem('maker-track-studio-locale', 'en'));
      const page = await context.newPage();
      page.on('pageerror', (error) => errors.push(error.message));
      await page.route('**/api/**', async (route) => {
        const request = route.request();
        const url = new URL(request.url());
        const path = url.pathname.slice(4);
        if (request.method() === 'POST' && path === '/print-jobs') {
          submitted = request.postDataJSON();
          await route.fulfill({ status: 201, json: { ...submitted, id: 'browser-job', filaments: [] } });
          return;
        }
        const pages = { items: submitted ? [{ ...submitted, id: 'browser-job', filaments: [] }] : [], total: submitted ? 1 : 0, page: 1, page_size: 25 };
        const responses = {
          '/health/ready': { status: 'ready' }, '/features': { sales_orders: false },
          '/print-jobs': pages, '/models': [], '/filaments': [],
          '/printers': [{ id: 'browser-printer', name: 'Browser fixture', is_default: true, is_archived: false }],
        };
        if (request.method() === 'GET' && path in responses) {
          await route.fulfill({ status: 200, json: responses[path] });
          return;
        }
        unexpectedRequests.push(`${request.method()} ${path}`);
        await route.fulfill({ status: 500, json: { error: 'Unexpected request in regression fixture' } });
      });
      await page.goto(`${origin}/#/print-jobs`);
      await page.getByRole('button', { name: /add first print job/i }).click();
      await page.locator('input[name="print_date"]').fill('2026-09-19');
      await page.locator('input[name="print_time_minutes"]').fill('60');
      await page.locator('input[name="units_printed"]').fill('3');
      await page.locator('input[name="labor_hours"]').fill(String(hours));
      validity = await page.locator('input[name="labor_hours"]').evaluate((element) => ({
        value: element.value, step: element.step, valid: element.checkValidity(),
        stepMismatch: element.validity.stepMismatch,
      }));
      await page.getByRole('button', { name: /create print job/i }).click();
      await expect.poll(() => submitted, { timeout: 2000 }).toEqual(expect.objectContaining({
        units_printed: 3, labor_hours: hours, print_time_minutes: 60,
      }));
      expect(unexpectedRequests).toEqual([]);
      expect(errors).toEqual([]);
      results.push({ hours, status: 'passed', validity, submitted });
    } catch (error) {
      results.push({ hours, status: 'failed', validity, error: error.message, unexpectedRequests, errors });
    } finally {
      await context.close();
    }
  }
} finally {
  await browser?.close();
  await server.close();
}
const report = process.env.REGRESSION_REPORT || join(tmpdir(), 'maker-browser-cost-results.json');
await writeFile(report, JSON.stringify(results, null, 2));
for (const result of results) console.log(`${result.status.toUpperCase()}: labor_hours=${result.hours}; native validity=${JSON.stringify(result.validity)}`);
console.log(`Report: ${report}`);
if (results.some((result) => result.status === 'failed')) process.exitCode = 1;
