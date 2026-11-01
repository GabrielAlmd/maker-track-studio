import { chromium, expect } from '@playwright/test';

// Test-only generated credentials arrive over stdin, never command arguments.
let input = '';
for await (const chunk of process.stdin) input += chunk;
const { origin, password, certificatePin } = JSON.parse(input);
const browser = await chromium.launch({
  // Trust only this disposable server's public key. Never disable TLS validation
  // globally; the Python client separately verifies its private CA chain.
  args: [`--ignore-certificate-errors-spki-list=${certificatePin}`],
});
try {
  const context = await browser.newContext({
    httpCredentials: { username: 'maker', password },
    locale: 'en-US',
  });
  await context.addInitScript(() => {
    localStorage.setItem('maker-track-studio-locale', 'en');
  });
  const page = await context.newPage();
  const errors = [];
  page.on('pageerror', (error) => errors.push(error.message));
  const ready = page.waitForResponse((response) => (
    response.url() === `${origin}/api/health/ready` && response.status() === 200
  ));
  await page.goto(`${origin}/#/printers`);
  await ready;
  await expect(page.getByText('Proxy fixture', { exact: true })).toBeVisible();

  // Browser-generated Origin and cached HTTP credentials must support writes.
  const created = await page.evaluate(async () => {
    const response = await fetch('/api/printers', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ name: 'Browser fixture', manufacturer: 'Test', model: 'Fixture' }),
    });
    return response.status;
  });
  expect(created).toBe(201);
  await page.reload();
  await expect(page.getByText('Browser fixture', { exact: true })).toBeVisible();
  expect(errors).toEqual([]);
  await context.close();
  console.log('PASS: Chromium authenticated UI, readiness, API writes and refresh');
} finally {
  await browser.close();
}
