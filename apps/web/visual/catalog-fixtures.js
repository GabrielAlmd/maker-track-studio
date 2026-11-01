export const models = [
  { id: 'm1', name: 'Modular Enclosure', material_weight: 86.4, print_time: 245, waste_material: 4.2, is_multicolor: true, print_job_count: 18 },
  { id: 'm2', name: 'Workshop Bracket', material_weight: 42.8, print_time: 110, waste_material: 1.5, is_multicolor: false, print_job_count: 31 },
  { id: 'm3', name: 'Cable Clip Set', material_weight: 18.2, print_time: 55, waste_material: 0.8, is_multicolor: false, print_job_count: 22 },
];

export const filaments = [
  { id: 'f1', brand: 'Bambu Lab', type: 'PLA', color: 'Matte Black', avg_price: 24.99, personal_score: 5, times_used: 48, purchases: [{ id: 'fp1' }] },
  { id: 'f2', brand: 'Bambu Lab', type: 'PETG', color: 'Ocean Blue', avg_price: 29.5, personal_score: 4, times_used: 21, purchases: [{ id: 'fp2' }] },
  { id: 'f3', brand: 'Prusament', type: 'PLA', color: 'Galaxy Silver', avg_price: 31.9, personal_score: 5, times_used: 34, purchases: [{ id: 'fp3' }] },
];

export const printJobs = [
  {
    id: 'j1', printer_id: 'p1', printer_cost: { printer_name: 'A1 – Workshop' }, model_id: 'm1', model: { state: 'assigned', id: 'm1', name: 'Modular Enclosure' },
    print_date: '2026-09-17T10:00:00.000Z', units_printed: 4, labor_hours: 1.5,
    print_time_minutes: 245, estimated_cost: 18.42,
    filaments: [{ id: 'pjf1', filament_id: 'f1', weight_used: 86.4, filament: { state: 'available', id: 'f1', name: 'Bambu Lab PLA Matte Black' } }],
  },
  {
    id: 'j2', printer_id: 'p2', printer_cost: { printer_name: 'P1S – Enclosure' }, model_id: 'm3', model: { state: 'assigned', id: 'm3', name: 'Cable Clip Set' },
    print_date: '2026-09-16T14:30:00.000Z', units_printed: 12, labor_hours: 0.8,
    print_time_minutes: 165, estimated_cost: 9.76,
    filaments: [{ id: 'pjf2', filament_id: 'f2', weight_used: 54.6, filament: { state: 'available', id: 'f2', name: 'Bambu Lab PETG Ocean Blue' } }],
  },
];

export const purchases = [
  { id: 'p1', filament_id: 'f1', filament: filaments[0], price: 24.99, amount: 1, purchase_date: '2026-09-12T00:00:00.000Z' },
  { id: 'p2', filament_id: 'f2', filament: filaments[1], price: 59, amount: 2, purchase_date: '2026-09-08T00:00:00.000Z' },
  { id: 'p3', filament_id: 'f3', filament: filaments[2], price: 31.9, amount: 1, purchase_date: '2026-08-29T00:00:00.000Z' },
];

const migrationSchema = {
  sections: [
    { key: 'printers', label: 'Printers', fields: [
      { key: 'name', label: 'Name', required: true, aliases: ['printer_name'], example: 'Workshop A1' },
      { key: 'manufacturer', label: 'Manufacturer', required: true, aliases: ['maker'], example: 'Bambu Lab' },
      { key: 'model', label: 'Model', required: true, aliases: [], example: 'A1' },
      { key: 'notes', label: 'Notes', required: false, aliases: [], example: 'Primary printer' },
    ] },
    { key: 'filaments', label: 'Filaments', fields: [{ key: 'brand', label: 'Brand', required: true, aliases: [], example: 'Bambu Lab' }] },
    { key: 'purchases', label: 'Purchases', fields: [{ key: 'filament', label: 'Filament', required: true, aliases: [], example: 'Bambu Lab | PLA | Black' }] },
    { key: 'models', label: 'Models', fields: [{ key: 'name', label: 'Name', required: true, aliases: [], example: 'Desk organiser' }] },
    { key: 'print_jobs', label: 'Print Jobs', fields: [{ key: 'printer', label: 'Printer', required: true, aliases: [], example: 'Workshop A1' }] },
  ],
};

export const salesChannels = [
  { id: 'sc1', name: 'Etsy', notes: '', is_archived: false },
  { id: 'sc2', name: 'Direct', notes: '', is_archived: false },
];

export const salesOrders = [
  {
    id: 'so1', reference: 'WEB-1042', order_date: '2026-09-18T00:00:00.000Z',
    sales_channel_id: 'sc1', sales_channel: salesChannels[0], fulfilment_status: 'preparing', payment_status: 'paid',
    packaging_cost_cents: 250, shipping_cost_cents: 300, platform_fee_cents: 200, other_cost_cents: 0, notes: 'Gift order — use recyclable packaging.',
    items: [{ id: 'soi1', model_id: 'm1', model_name_snapshot: 'Modular Enclosure', quantity: 2, unit_price_cents: 4200 }],
    profitability: {
      state: 'calculated', revenue_cents: 8400, additional_costs_cents: 750,
      production_cost_cents: 3684, total_cost_cents: 4434, net_profit_cents: 3966,
      margin: { state: 'calculated', percent: 47.21 },
    },
  },
  {
    id: 'so2', reference: 'DIRECT-031', order_date: '2026-09-14T00:00:00.000Z',
    sales_channel_id: 'sc2', sales_channel: salesChannels[1], fulfilment_status: 'completed', payment_status: 'paid',
    packaging_cost_cents: 120, shipping_cost_cents: 300, platform_fee_cents: 0, other_cost_cents: 0, notes: '',
    items: [{ id: 'soi2', model_id: 'm3', model_name_snapshot: 'Cable Clip Set', quantity: 6, unit_price_cents: 650 }],
    profitability: {
      state: 'calculated', revenue_cents: 3900, additional_costs_cents: 420,
      production_cost_cents: 976, total_cost_cents: 1396, net_profit_cents: 2504,
      margin: { state: 'calculated', percent: 64.21 },
    },
  },
];

const salesOrderAllocations = {
  sales_order_id: 'so1',
  items: [{
    sales_order_item_id: 'soi1', quantity: 2, units_allocated: 1, units_available: 1,
    allocations: [{
      id: 'soa1', sales_order_item_id: 'soi1', print_job_id: 'j1', units_allocated: 1,
      print_job: { id: 'j1', model_id: 'm1', units_printed: 4, units_allocated: 2, units_available: 2 },
    }],
  }],
};

const completedSalesOrderAllocations = {
  sales_order_id: 'so2',
  items: [{
    sales_order_item_id: 'soi2', quantity: 6, units_allocated: 6, units_available: 0,
    allocations: [{
      id: 'soa2', sales_order_item_id: 'soi2', print_job_id: 'j2', units_allocated: 6, production_cost_cents: 976,
      print_job: { id: 'j2', model_id: 'm3', units_printed: 12, units_allocated: 6, units_available: 6 },
    }],
  }],
};

const dashboardSummary = {
  total_models: 18,
  total_filaments: 27,
  total_purchases: 42,
  total_print_jobs: 126,
  cost_stats: {
    total_units_printed: 284,
    total_filament_weight_g: 18750,
    calculated_jobs: 119,
    total_filament_cost: 428.65,
    total_electricity_cost: 96.4,
    total_wear_cost: 183.75,
    total_labor_cost: 612,
    total_cost: 1320.8,
  },
  expenses: {
    printer_cost: 1598,
    filament_cost: 115.89,
    spare_parts_cost: 29.9,
    additional_cost: 74.5,
    other_cost: 18,
    total_expenses: 1836.29,
  },
  recent_print_jobs: printJobs.map((job) => ({
    ...job,
    cost: { state: 'calculated', total: job.estimated_cost },
  })),
};

const salesSummary = {
  active_order_count: 1,
  completed_order_count: 1,
  unpaid_order_count: 0,
  revenue_cents: 3900,
  production_cost_cents: 976,
  packaging_cost_cents: 120,
  shipping_cost_cents: 300,
  platform_fee_cents: 0,
  other_cost_cents: 0,
  total_cost_cents: 1396,
  net_profit_cents: 2504,
  margin: { state: 'calculated', percent: 64.21 },
  recent_orders: salesOrders.map((order) => ({
    id: order.id,
    reference: order.reference,
    order_date: order.order_date,
    sales_channel: order.sales_channel,
    fulfilment_status: order.fulfilment_status,
    payment_status: order.payment_status,
    revenue_cents: order.profitability.revenue_cents,
  })),
};

const appConfig = [
  ['machine_cost_mode', '0', 'number'],
  ['printer_purchase_cost', '899', 'number'],
  ['printer_equipment_costs', JSON.stringify([{ description: 'Hardened nozzle', date: '2026-06-12', amount: 29.9, category: 'spare_parts' }]), 'json'],
  ['printer_annual_maintenance', '75', 'number'],
  ['printer_lifetime_years', '5', 'number'],
  ['printer_uptime_percent', '35', 'number'],
  ['printer_power_watts', '180', 'number'],
  ['printer_buffer_factor', '1.2', 'number'],
  ['electricity_cost_kwh', '0.24', 'number'],
  ['printer_wear_cost_hour', '0.18', 'number'],
  ['labor_cost_hour', '18', 'number'],
  ['filament_waste_factor', '1.08', 'number'],
].map(([key, value, type]) => ({ key, value, type }));

const printerCostConfig = Object.fromEntries(appConfig.filter((row) => row.type === 'number').map((row) => [row.key, Number(row.value)]));
export const printers = [
  { id: 'p1', name: 'A1 – Workshop', manufacturer: 'Bambu Lab', model: 'A1', notes: 'Primary machine near the assembly bench.', is_default: true, is_archived: false, cost_config: printerCostConfig, updated_at: '2026-09-18T10:00:00Z' },
  { id: 'p2', name: 'P1S – Enclosure', manufacturer: 'Bambu Lab', model: 'P1S', notes: 'Enclosed printer for engineering materials.', is_default: false, is_archived: false, cost_config: { ...printerCostConfig, printer_purchase_cost: 699 }, updated_at: '2026-09-18T10:00:00Z' },
  { id: 'p3', name: 'MK3 – Retired', manufacturer: 'Prusa', model: 'MK3S+', notes: '', is_default: false, is_archived: true, cost_config: printerCostConfig, updated_at: '2026-09-18T10:00:00Z' },
];

const cost = {
  state: 'calculated', filament_cost: 4.95, electricity_cost: 0.78,
  wear_cost: 1.42, labor_cost: 11.25, total_cost: 18.4,
};

function page(items) {
  return { items, total: items.length, page: 1, page_size: 25 };
}

export async function installApiFixtures(pageContext) {
  await pageContext.addInitScript(() => {
    window.go = {
      main: {
        App: {
          GetVersion: async () => '1.0.0',
          ExportDesktopData: async () => '/Documents/makertrack-backup.zip',
          ExportReport: async () => 'makertrack-report.zip',
        },
      },
    };
  });

  await pageContext.route('**/api/**', async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    const apiPath = url.pathname.replace(/^\/api/, '');
    const corsHeaders = {
      'Access-Control-Allow-Origin': 'http://127.0.0.1:4173',
      'Access-Control-Allow-Headers': 'Content-Type',
      'Access-Control-Allow-Methods': 'GET, POST, PUT, DELETE, OPTIONS',
    };

    if (request.method() === 'OPTIONS') {
      await route.fulfill({ status: 204, headers: corsHeaders });
      return;
    }

    if (request.method() === 'GET' && apiPath === '/health/ready') {
      await route.fulfill({ status: 200, headers: corsHeaders, json: { status: 'ready' } });
      return;
    }

    if (request.method() === 'POST' && apiPath === '/data-migration/inspect') {
      await route.fulfill({ status: 200, headers: corsHeaders, json: { format: 'csv', sources: [{ name: 'printers', columns: ['Name', 'Manufacturer', 'Model', 'Notes'], row_count: 2, sample_rows: [['Workshop A1', 'Bambu Lab', 'A1', 'Primary printer'], ['Enclosed P1S', 'Bambu Lab', 'P1S', 'Engineering materials']] }] } });
      return;
    }

    if (request.method() === 'POST' && apiPath === '/data-migration/preview') {
      await route.fulfill({ status: 200, headers: corsHeaders, json: { can_import: true, sections: [{ section: 'printers', source: 'printers', effective_mapping: { name: 'Name', manufacturer: 'Manufacturer', model: 'Model', notes: 'Notes' }, summary: { total: 2, ready: 1, warnings: 0, invalid: 0, duplicates: 1, skipped: 1 }, rows: [{ row: 2, status: 'ready', action: 'create', values: {}, issues: [] }, { row: 3, status: 'duplicate', action: 'skip', values: {}, issues: [] }] }] } });
      return;
    }

    if (request.method() !== 'GET') {
      await route.fulfill({ status: 200, headers: corsHeaders, json: { ok: true } });
      return;
    }

    const responses = new Map([
      ['/features', { sales_orders: true }],
      ['/print-jobs/dashboard-summary', dashboardSummary],
      ['/sales/summary', salesSummary],
      ['/filament-types', ['PLA', 'PETG', 'ABS', 'TPU']],
      ['/filament-brands', ['Bambu Lab', 'Prusament']],
      ['/app-config', appConfig],
      ['/data-migration/schema', migrationSchema],
      ['/printers', printers],
      ['/sales-channels', salesChannels],
      ['/sales-orders', page(salesOrders)],
    ]);

    let body = responses.get(apiPath);
    if (apiPath === '/models') body = url.search ? page(models) : models;
    if (apiPath === '/filaments') body = url.search ? page(filaments) : filaments;
    if (apiPath === '/print-jobs') body = page(url.searchParams.get('model_id') ? printJobs.filter((job) => job.model_id === url.searchParams.get('model_id')) : printJobs);
    if (apiPath === '/filament-purchases') body = page(purchases);
    if (apiPath === '/sales-orders') {
      const fulfilment = url.searchParams.get('fulfilment_status');
      const payment = url.searchParams.get('payment_status');
      const channel = url.searchParams.get('sales_channel_id');
      const model = url.searchParams.get('model_id');
      body = page(salesOrders.filter((order) =>
        (!fulfilment || order.fulfilment_status === fulfilment)
        && (!payment || order.payment_status === payment)
        && (!channel || order.sales_channel_id === channel)
        && (!model || order.items.some((item) => item.model_id === model))));
    }
    if (apiPath === '/sales-orders/so1') body = salesOrders[0];
    if (apiPath === '/sales-orders/so1/allocations') body = salesOrderAllocations;
    if (apiPath === '/sales-orders/so2') body = salesOrders[1];
    if (apiPath === '/sales-orders/so2/allocations') body = completedSalesOrderAllocations;
    if (apiPath === '/print-jobs/j1/allocations') body = { print_job: { id: 'j1', model_id: 'm1', units_printed: 4, units_allocated: 2, units_available: 2 }, used_by: [{ allocation_id: 'soa1', sales_order_id: 'so1', sales_order_item_id: 'soi1', order_reference: 'WEB-1042', units_allocated: 1 }] };
    if (apiPath === '/print-jobs/j2/allocations') body = { print_job: { id: 'j2', model_id: 'm3', units_printed: 12, units_allocated: 6, units_available: 6 }, used_by: [{ allocation_id: 'soa2', sales_order_id: 'so2', sales_order_item_id: 'soi2', order_reference: 'DIRECT-031', units_allocated: 6 }] };
    if (/^\/print-jobs\/[^/]+\/cost$/.test(apiPath)) body = cost;

    if (body !== undefined) {
      await route.fulfill({ status: 200, headers: corsHeaders, json: body });
      return;
    }

    await route.fulfill({ status: 404, headers: corsHeaders, json: { error: 'Not available in screenshot fixture' } });
  });
}
