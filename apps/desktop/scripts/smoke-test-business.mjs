// Run against the packaged application's disposable smoke-test workspace.
import assert from 'node:assert/strict';
import { randomUUID } from 'node:crypto';

const base = 'http://127.0.0.1:8080';
async function request(path, method, status, body) {
  const response = await fetch(base + path, {
    method, body: body instanceof FormData ? body : JSON.stringify(body),
    headers: body && !(body instanceof FormData) ? { 'Content-Type': 'application/json' } : undefined,
    signal: AbortSignal.timeout(10_000),
  });
  assert.equal(response.status, status, `${method} ${path}`);
  return response;
}

await request('/features', 'PUT', 200, { sales_orders: true });
const model = await (await request('/models', 'POST', 201, {
  name: 'Packaged smoke model', print_time: 30,
  material_requirements: [
    { material_type: 'PLA', color: 'Blue', part_weight_g: 16, waste_weight_g: 0 },
    { material_type: 'PLA', color: 'Orange', part_weight_g: 4, source_tool: 3 },
  ],
})).json();
assert.equal(model.material_weight, 20);
assert.equal(model.material_summary.color_status, 'multiple');
const path = '/v2/business/quick-sales';
const cost = { source: 'manual', manual_unit_cents: 500, target: { mode: 'margin', value: 4000 } };
const estimateInput = { model_id: model.id, quantity: 2, cost };
const estimate = await (await request(`${path}/estimate`, 'POST', 200, estimateInput)).json();
assert.equal(estimate.suggested_unit_price_cents, 833);
const command = { ...estimateInput, operation_id: randomUUID(), reference: 'Packaged smoke sale',
  unit_price_cents: 1250, estimate_token: estimate.token };
const sale = await (await request(path, 'POST', 201, command)).json();
const order = `/sales-orders/${sale.id}`;
const original = await (await request(order, 'GET', 200)).json();
assert.equal(original.source, 'quick_sale');
assert.equal(original.profitability.revenue_cents, 2500);
const finance = `/v2/business/orders/${sale.id}/finance`;
assert.equal((await (await request(finance, 'GET', 200)).json()).estimated_profit_cents, 1500);

await request('/features', 'PUT', 200, { sales_orders: false });
await request(path, 'POST', 409, { ...command, operation_id: randomUUID() });
const archive = await (await request('/desktop-data/export', 'GET', 200)).blob();
const restore = new FormData();
restore.set('file', archive, 'packaged-smoke.zip');
await request('/desktop-data/import', 'POST', 200, restore);
assert.deepEqual(await (await request(`/models/${model.id}`, 'GET', 200)).json(), model);
const features = await (await request('/features', 'GET', 200)).json();
assert.equal(features.sales_orders, false);
await request('/features', 'PUT', 200, { sales_orders: true });
assert.deepEqual(await (await request(path, 'POST', 201, command)).json(), sale);
assert.deepEqual(await (await request(order, 'GET', 200)).json(), original);
assert.equal((await (await request(finance, 'GET', 200)).json()).estimated_profit_cents, 1500);
console.log('Packaged Model materials, Quick Sale, feature toggle and SQLite backup/restore passed');
