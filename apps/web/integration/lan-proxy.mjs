import { Buffer } from 'node:buffer';
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
async function downloadFile(page, button) {
  const pending = page.waitForEvent('download');
  await button.click();
  const download = await pending;
  const chunks = [];
  for await (const chunk of await download.createReadStream()) chunks.push(chunk);
  return { name: download.suggestedFilename(), content: Buffer.concat(chunks) };
}

async function exerciseQuickSale(page) {
  const created = await page.evaluate(async () => {
    const response = await fetch('/api/models', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ name: 'Synthetic clamp', material_weight: 20, print_time: 30 }),
    });
    return response.status;
  });
  expect(created).toBe(201);
  await page.getByRole('link', { name: 'Quick Sale', exact: true }).click();
  await page.getByLabel('Sale reference').fill('Synthetic counter sale');
  await page.getByLabel('Model', { exact: true }).selectOption({ label: 'Synthetic clamp' });
  await page.getByLabel('Unit price (EUR cents)').fill('1299');
  await page.getByLabel('Quantity', { exact: true }).fill('0');
  await page.getByRole('button', { name: 'Confirm sale' }).click();
  await expect(page.getByLabel('Quantity', { exact: true })).toHaveAttribute('aria-invalid', 'true');
  await page.getByLabel('Quantity', { exact: true }).fill('2');
  await page.getByRole('button', { name: 'Confirm sale' }).click();
  await expect(page.getByRole('heading', { name: 'Synthetic counter sale' })).toBeVisible();
  await expect(page.getByText('Walk-in customer')).toBeVisible();
  const saleId = new URL(page.url()).hash.split('/').at(-1);
  await page.getByRole('link', { name: 'Financials', exact: true }).click();
  await expect(page.getByText('€25.98', { exact: true }).filter({ visible: true }).first()).toBeVisible();
  await page.getByRole('link', { name: 'History', exact: true }).click();
  await expect(page.getByText('Quick Sale confirmed', { exact: true })).toBeVisible();
  return saleId;
}

async function exerciseBusinessOperations(page) {
  await page.goto(`${origin}/#/settings?section=features`);
  const feature = page.getByRole('checkbox', { name: 'Business Operations' });
  await feature.click();
  await expect(feature).toBeChecked();
  await page.goto(`${origin}/#/business/customers/new`);
  await page.getByLabel('Name', { exact: true }).fill('Synthetic workshop');
  await page.getByRole('button', { name: 'Save changes' }).click();
  await expect(page.getByRole('heading', { name: 'Synthetic workshop' })).toBeVisible();

  await page.goto(`${origin}/#/business/budgets/new`);
  await page.getByLabel('Title', { exact: true }).fill('Synthetic housing');
  await page.getByLabel('Customer', { exact: true }).selectOption({ label: 'Synthetic workshop' });
  await page.getByLabel('Name', { exact: true }).fill('Housing');
  const quantity = page.getByLabel('Quantity', { exact: true });
  await quantity.fill('0');
  await page.getByRole('button', { name: 'Save changes' }).click();
  await expect(quantity).toHaveAttribute('aria-invalid', 'true');
  await quantity.fill('2');
  await page.getByLabel('Post-processing per unit (EUR cents)').fill('250');
  await page.getByRole('button', { name: 'Save changes' }).click();
  await expect(page.getByText('Internal estimate €5.00')).toBeVisible();
  await page.getByRole('link', { name: 'Create proposal from budget' }).click();

  await page.getByLabel('Valid until').fill('2099-12-31');
  await page.getByLabel('Amount (EUR cents)', { exact: true }).fill('1200');
  await page.getByRole('button', { name: 'Save draft revision' }).click();
  await page.getByRole('button', { name: 'Issue proposal' }).click();
  await page.getByRole('link', { name: 'View issued document' }).click();
  await expect(page.getByText('Proposal total €24.00')).toBeVisible();
  await expect(page.getByText('Internal estimate', { exact: true })).toHaveCount(0);
  const proposalFile = await downloadFile(page, page.getByRole('button', { name: 'Download proposal PDF' }));
  expect(proposalFile.name).toMatch(/^proposal-[a-f0-9-]+-r2\.pdf$/);
  expect(proposalFile.content.subarray(0, 5).toString()).toBe('%PDF-');
  await page.getByLabel('Customer PO').fill('SYNTH-PO-1');
  await page.getByRole('button', { name: 'Confirm customer acceptance' }).click();
  await expect(page).toHaveURL(/#\/business\/orders\/[a-f0-9-]+$/);
  const orderId = new URL(page.url()).hash.split('/').at(-1);
  await expect(page.getByText('SYNTH-PO-1', { exact: true })).toBeVisible();

  await page.getByText('Correct order terms', { exact: true }).click();
  await page.locator('details').getByLabel('Customer PO').fill('SYNTH-PO-2');
  await page.getByLabel('Reason').fill('Synthetic correction');
  await page.getByRole('button', { name: 'Save changes' }).click();
  await expect(page.getByText('SYNTH-PO-2', { exact: true })).toBeVisible();

  await page.goto(`${origin}/#/sales/${orderId}?section=history`);
  await expect(page).toHaveURL(/#\/business\/orders\/[a-f0-9-]+\?section=history$/);
  const amendment = page.getByRole('listitem').filter({ hasText: 'Order amended · revision 2' });
  await expect(amendment.getByText('Synthetic correction', { exact: true })).toBeVisible();
  await page.getByRole('button', { name: 'Previous revision' }).click();
  await expect(page.getByText('SYNTH-PO-1', { exact: true })).toBeVisible();
  await page.getByRole('link', { name: 'Documents', exact: true }).click();
  await page.getByLabel('Attachment file').setInputFiles({
    name: 'Synthetic issued proposal.pdf', mimeType: 'application/pdf', buffer: proposalFile.content,
  });
  await page.getByRole('button', { name: 'Add attachment' }).click();
  const attachmentFile = await downloadFile(page, page.getByRole('button', { name: 'Download Synthetic issued proposal.pdf' }));
  expect(attachmentFile.name).toBe('Synthetic issued proposal.pdf');
  expect(attachmentFile.content.equals(proposalFile.content)).toBe(true);
  await page.getByRole('link', { name: 'History', exact: true }).click();
  const attachmentEvent = page.getByRole('listitem').filter({ hasText: 'Attachment added' });
  await expect(attachmentEvent.getByText('Synthetic issued proposal.pdf', { exact: true })).toBeVisible();
  await page.getByRole('link', { name: 'Documents', exact: true }).click();
  await page.getByRole('link', { name: 'View accepted proposal' }).click();
  await expect(page.getByText('Proposal total €24.00')).toBeVisible();

  // SQLite must reject repeat acceptance while retaining the original order.
  const conflict = page.waitForResponse((response) =>
    response.url().endsWith('/api/v2/business/orders') && response.request().method() === 'POST');
  await page.getByRole('button', { name: 'Confirm customer acceptance' }).click();
  expect((await conflict).status()).toBe(409);
  await expect(page.getByRole('alert')).toContainText('this action is no longer available');
  await page.goto(`${origin}/#/business/orders`);
  await expect(page.getByRole('link', { name: 'Synthetic housing' })).toHaveCount(1);
  const saleId = await exerciseQuickSale(page);

  await page.goto(`${origin}/#/settings?section=features`);
  await feature.click();
  await expect(feature).not.toBeChecked();
  await expect(page.getByRole('link', { name: 'Business Operations' })).toHaveCount(0);
  await page.goto(`${origin}/#/sales/${orderId}`);
  await expect(page).toHaveURL(/#\/dashboard$/);
  await page.getByRole('link', { name: 'Inventory', exact: true }).click();
  await expect(page.getByRole('heading', { name: 'Inventory', exact: true })).toBeVisible();

  await page.goto(`${origin}/#/settings?section=features`);
  await feature.click();
  await expect(feature).toBeChecked();
  await page.goto(`${origin}/#/business/orders/${saleId}`);
  await expect(page.getByRole('heading', { name: 'Synthetic counter sale' })).toBeVisible();
}

try {
  const context = await browser.newContext({
    httpCredentials: { username: 'maker', password },
    locale: 'en-US',
  });
  await context.addInitScript(() => {
    localStorage.setItem('maker-track-studio-locale', 'en');
  });
  const page = await context.newPage();
  page.setDefaultTimeout(8_000);
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
  await exerciseBusinessOperations(page);
  expect(errors).toEqual([]);
  await context.close();
  console.log('PASS: Chromium authenticated UI, readiness, API writes and Business Operations');
} finally {
  await browser.close();
}
