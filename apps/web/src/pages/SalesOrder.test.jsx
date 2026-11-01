import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import SalesOrder from '../modules/sales/pages/SalesOrder';
import * as salesService from '../modules/sales/services/salesService';
import * as modelService from '../services/modelService';
import * as printJobService from '../services/printJobService';

vi.mock('../modules/sales/services/salesService', () => ({
  createSalesChannel: vi.fn(), createSalesOrder: vi.fn(), deleteSalesOrder: vi.fn(), getCompatiblePrintJobs: vi.fn(), getPrintJobAllocations: vi.fn(),
  getSalesChannels: vi.fn(), getSalesOrder: vi.fn(), getSalesOrderAllocations: vi.fn(), replaceSalesOrderAllocations: vi.fn(),
  updateSalesOrder: vi.fn(), updateSalesOrderStatus: vi.fn(),
}));
vi.mock('../services/modelService', () => ({ getModels: vi.fn() }));
vi.mock('../services/printJobService', () => ({ getPrintJobCost: vi.fn() }));
vi.mock('react-hot-toast', () => ({ default: { success: vi.fn(), error: vi.fn() } }));

const models = [{ id: 'model-1', name: 'Enclosure' }, { id: 'model-2', name: 'Bracket' }];
const channels = [{ id: 'channel-1', name: 'Etsy', is_archived: false }];

function makeOrder(status = 'draft') {
  return {
    id: 'order-1', reference: 'WEB-1042', order_date: '2026-09-18T00:00:00Z', sales_channel_id: 'channel-1', sales_channel: channels[0],
    fulfilment_status: status, payment_status: 'paid', packaging_cost_cents: 100, shipping_cost_cents: 200, platform_fee_cents: 300, other_cost_cents: 0, notes: 'Handle carefully',
    items: [{ id: 'item-1', model_id: 'model-1', model_name_snapshot: 'Enclosure', quantity: 2, unit_price_cents: 2500 }],
    profitability: { state: 'calculated', revenue_cents: 5000, additional_costs_cents: 600, production_cost_cents: 1800, total_cost_cents: 2400, net_profit_cents: 2600, margin: { state: 'calculated', percent: 52 } },
  };
}

function allocationDocument(rows = []) {
  return { sales_order_id: 'order-1', items: [{ sales_order_item_id: 'item-1', quantity: 2, units_allocated: rows.reduce((sum, row) => sum + row.units_allocated, 0), units_available: 2, allocations: rows }] };
}

function renderOrder(path = '/sales/new') {
  return render(<MemoryRouter initialEntries={[path]}><Routes><Route path="/sales/new" element={<SalesOrder />} /><Route path="/sales/:id" element={<SalesOrder />} /><Route path="/sales" element={<p>Sales list</p>} /></Routes></MemoryRouter>);
}

describe('Sales Order workflow', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    salesService.getSalesChannels.mockResolvedValue(channels);
    modelService.getModels.mockResolvedValue(models);
    salesService.getCompatiblePrintJobs.mockImplementation(async (modelId) => ({ items: [{ id: modelId === 'model-1' ? 'job-1' : 'job-2', model_id: modelId, print_date: '2026-09-17T00:00:00Z', units_printed: 10 }], total: 1, page: 1, page_size: 100 }));
    printJobService.getPrintJobCost.mockResolvedValue({ state: 'not_calculated' });
    salesService.getPrintJobAllocations.mockImplementation(async (jobId) => ({ print_job: { id: jobId, model_id: jobId === 'job-1' ? 'model-1' : 'model-2', units_printed: 10, units_allocated: 2, units_available: 8 }, used_by: [] }));
    salesService.getSalesOrder.mockResolvedValue(makeOrder());
    salesService.getSalesOrderAllocations.mockResolvedValue(allocationDocument());
    salesService.replaceSalesOrderAllocations.mockResolvedValue(allocationDocument());
    salesService.updateSalesOrder.mockResolvedValue(makeOrder());
    salesService.updateSalesOrderStatus.mockResolvedValue(makeOrder('confirmed'));
    salesService.deleteSalesOrder.mockResolvedValue();
  });

  it('creates an order with multiple items and remaps allocations to returned item IDs', async () => {
    const created = {
      ...makeOrder(), id: 'created-order', reference: 'NEW-1', payment_status: 'unpaid',
      items: [
        { id: 'created-item-1', model_id: 'model-1', model_name_snapshot: 'Enclosure', quantity: 2, unit_price_cents: 1200 },
        { id: 'created-item-2', model_id: 'model-2', model_name_snapshot: 'Bracket', quantity: 3, unit_price_cents: 800 },
      ],
    };
    salesService.createSalesOrder.mockResolvedValue(created);
    salesService.getSalesOrder.mockResolvedValue(created);
    salesService.getSalesOrderAllocations.mockResolvedValue({ sales_order_id: 'created-order', items: [] });
    const user = userEvent.setup();
    renderOrder();

    await screen.findByRole('heading', { name: 'New order' });
    await user.type(screen.getByLabelText('Reference'), 'NEW-1');
    await user.selectOptions(screen.getByLabelText('Sales channel'), 'channel-1');
    await user.selectOptions(screen.getAllByLabelText('Model')[0], 'model-1');
    fireEvent.change(screen.getAllByLabelText('Quantity')[0], { target: { value: '2' } });
    await user.type(screen.getAllByLabelText('Unit selling price (€)')[0], '12');
    await user.click(screen.getByRole('button', { name: 'Add item' }));
    await user.selectOptions(screen.getAllByLabelText('Model')[1], 'model-2');
    fireEvent.change(screen.getAllByLabelText('Quantity')[1], { target: { value: '3' } });
    await user.type(screen.getAllByLabelText('Unit selling price (€)')[1], '8');

    await user.type(await screen.findByLabelText('Units assigned to Print Job job-1'), '2');
    await user.type(await screen.findByLabelText('Units assigned to Print Job job-2'), '1');
    await user.click(screen.getByRole('button', { name: 'Create order' }));

    await waitFor(() => expect(salesService.createSalesOrder).toHaveBeenCalledWith(expect.objectContaining({
      reference: 'NEW-1', sales_channel_id: 'channel-1', fulfilment_status: 'draft', payment_status: 'unpaid',
      items: [{ model_id: 'model-1', quantity: 2, unit_price_cents: 1200 }, { model_id: 'model-2', quantity: 3, unit_price_cents: 800 }],
    })));
    expect(salesService.replaceSalesOrderAllocations).toHaveBeenCalledWith('created-order', [
      { sales_order_item_id: 'created-item-1', print_job_id: 'job-1', units_allocated: 2 },
      { sales_order_item_id: 'created-item-2', print_job_id: 'job-2', units_allocated: 1 },
    ]);
  });

  it('validates required order and item fields before calling the API', async () => {
    const user = userEvent.setup();
    renderOrder();
    await screen.findByRole('heading', { name: 'New order' });
    await user.click(screen.getByRole('button', { name: 'Create order' }));

    expect(await screen.findByText('Reference is required')).toBeInTheDocument();
    expect(screen.getByText('Model is required')).toBeInTheDocument();
    expect(screen.getByText('Enter a valid unit selling price')).toBeInTheDocument();
    expect(salesService.createSalesOrder).not.toHaveBeenCalled();
  });

  it('keeps completed order values readable and frozen without edit or destructive actions', async () => {
    salesService.getSalesOrder.mockResolvedValue(makeOrder('completed'));
    renderOrder('/sales/order-1');

    expect(await screen.findByText('Historical financial values are frozen.')).toBeInTheDocument();
    expect(screen.getByText('Frozen cost')).toBeInTheDocument();
    expect(screen.getByText('€18.00')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Save changes' })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Delete order' })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Cancel order' })).not.toBeInTheDocument();
  });

  it('offers permanent deletion only for drafts and cancellation for active orders', async () => {
    const user = userEvent.setup();
    const confirm = vi.spyOn(window, 'confirm').mockReturnValue(true);
    const view = renderOrder('/sales/order-1');
    expect(await screen.findByRole('button', { name: 'Delete order' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Cancel order' })).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Delete order' }));
    await waitFor(() => expect(salesService.deleteSalesOrder).toHaveBeenCalledWith('order-1'));

    view.unmount();
    salesService.getSalesOrder.mockResolvedValue(makeOrder('confirmed'));
    renderOrder('/sales/order-1');
    expect(await screen.findByRole('button', { name: 'Cancel order' })).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Delete order' })).not.toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Cancel order' }));
    await waitFor(() => expect(salesService.updateSalesOrderStatus).toHaveBeenCalledWith('order-1', { fulfilment_status: 'cancelled' }));
    confirm.mockRestore();
  });

  it('saves allocation changes for a non-draft active order', async () => {
    salesService.getSalesOrder.mockResolvedValue(makeOrder('confirmed'));
    salesService.getSalesOrderAllocations.mockResolvedValue(allocationDocument([{ id: 'allocation-1', sales_order_item_id: 'item-1', print_job_id: 'job-1', units_allocated: 1, print_job: { id: 'job-1', model_id: 'model-1', units_printed: 10, units_allocated: 2, units_available: 8 } }]));
    const user = userEvent.setup();
    renderOrder('/sales/order-1');

    const allocation = await screen.findByLabelText('Units assigned to Print Job job-1');
    await user.clear(allocation);
    await user.type(allocation, '2');
    await user.click(screen.getByRole('button', { name: 'Save allocations' }));
    await waitFor(() => expect(salesService.replaceSalesOrderAllocations).toHaveBeenCalledWith('order-1', [{ sales_order_item_id: 'item-1', print_job_id: 'job-1', units_allocated: 2 }]));
  });

  it('suggests profit on per-piece cost, supports over 100%, and saves the chosen override', async () => {
    const created = { ...makeOrder(), id: 'no-channel-order', sales_channel_id: null, sales_channel: null, items: [{ id: 'created-item', model_id: 'model-1', model_name_snapshot: 'Enclosure', quantity: 1, unit_price_cents: 2600 }] };
    salesService.createSalesOrder.mockResolvedValue(created);
    printJobService.getPrintJobCost.mockResolvedValue({ state: 'calculated', total_cost: 100 });
    const user = userEvent.setup();
    renderOrder();

    await screen.findByRole('heading', { name: 'New order' });
    expect(screen.getByLabelText('Sales channel')).toHaveValue('');
    await user.type(screen.getByLabelText('Reference'), 'NO-CHANNEL');
    await user.selectOptions(screen.getByLabelText('Model'), 'model-1');
    await user.type(screen.getByLabelText('Target profit (%)'), '150');
    expect(await screen.findByText('Unit cost: €10.00')).toBeInTheDocument();
    expect(screen.getByText('Suggested unit price: €25.00')).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Use suggested price' }));
    expect(screen.getByLabelText('Unit selling price (€)')).toHaveValue(25);

    await user.clear(screen.getByLabelText('Target profit (%)'));
    await user.type(screen.getByLabelText('Target profit (%)'), '100');
    expect(screen.getByText('Suggested unit price: €20.00')).toBeInTheDocument();
    expect(screen.getByLabelText('Unit selling price (€)')).toHaveValue(25);
    await user.clear(screen.getByLabelText('Unit selling price (€)'));
    await user.type(screen.getByLabelText('Unit selling price (€)'), '26');
    await user.click(screen.getByRole('button', { name: 'Create order' }));

    await waitFor(() => expect(salesService.createSalesOrder).toHaveBeenCalledWith(expect.objectContaining({
      sales_channel_id: null,
      items: [{ model_id: 'model-1', quantity: 1, unit_price_cents: 2600 }],
    })));
  });

  it('shows the profit-on-cost explanation in an accessible tooltip', async () => {
    const user = userEvent.setup();
    renderOrder();

    await screen.findByRole('heading', { name: 'New order' });
    await user.click(screen.getByRole('button', { name: 'Help for Target profit (%)' }));
    expect(screen.getByRole('tooltip')).toHaveTextContent('Adds this percentage to the latest compatible Print Job’s cost per piece. It is only a suggestion and is not saved with the sale.');
  });

  it('rejects a negative target profit percentage', async () => {
    const user = userEvent.setup();
    renderOrder();
    await screen.findByRole('heading', { name: 'New order' });
    await user.type(screen.getByLabelText('Reference'), 'INVALID-TARGET');
    await user.selectOptions(screen.getByLabelText('Model'), 'model-1');
    fireEvent.change(screen.getByLabelText('Target profit (%)'), { target: { value: '-1' } });
    await user.type(screen.getByLabelText('Unit selling price (€)'), '10');
    await user.click(screen.getByRole('button', { name: 'Create order' }));

    expect(await screen.findByText('Amount cannot be negative')).toBeInTheDocument();
    expect(salesService.createSalesOrder).not.toHaveBeenCalled();
  });

  it('creates a Sales Channel from the Order form and selects it', async () => {
    salesService.createSalesChannel.mockResolvedValue({ id: 'channel-new', name: 'Craft fair', notes: 'In person', is_archived: false });
    const user = userEvent.setup();
    renderOrder();

    await screen.findByRole('heading', { name: 'New order' });
    await user.click(screen.getByRole('button', { name: 'Add Sales Channel' }));
    await user.type(screen.getByLabelText('Channel name'), 'Craft fair');
    await user.type(screen.getByLabelText('Channel notes (optional)'), 'In person');
    await user.click(screen.getByRole('button', { name: 'Create channel' }));

    await waitFor(() => expect(salesService.createSalesChannel).toHaveBeenCalledWith({ name: 'Craft fair', notes: 'In person' }));
    await waitFor(() => expect(screen.getByLabelText('Sales channel')).toHaveValue('channel-new'));
    expect(screen.getByRole('option', { name: 'Craft fair' })).toBeInTheDocument();
  });

  it('does not offer archived Channels for a new Order', async () => {
    salesService.getSalesChannels.mockImplementation(async (state) => state === 'active' ? channels : [...channels, { id: 'archived', name: 'Old shop', is_archived: true }]);
    renderOrder();

    await screen.findByRole('heading', { name: 'New order' });
    expect(salesService.getSalesChannels).toHaveBeenCalledWith('active');
    expect(screen.queryByRole('option', { name: /Old shop/ })).not.toBeInTheDocument();
  });

  it('renders an existing Order with no Sales Channel clearly', async () => {
    salesService.getSalesOrder.mockResolvedValue({ ...makeOrder('completed'), sales_channel_id: null, sales_channel: null });
    renderOrder('/sales/order-1');

    expect(await screen.findByText('No sales channel')).toBeInTheDocument();
    expect(screen.queryByText('Data unavailable')).not.toBeInTheDocument();
  });

  it('preserves an archived Channel while editing its existing draft Order', async () => {
    const archived = { id: 'channel-1', name: 'Etsy', is_archived: true };
    salesService.getSalesChannels.mockResolvedValue([archived]);
    salesService.getSalesOrder.mockResolvedValue({ ...makeOrder(), sales_channel: archived });
    const user = userEvent.setup();
    renderOrder('/sales/order-1');

    expect(await screen.findByRole('option', { name: 'Etsy (archived)' })).toBeInTheDocument();
    expect(screen.getByLabelText('Sales channel')).toHaveValue('channel-1');
    await user.click(screen.getByRole('button', { name: 'Save changes' }));
    await waitFor(() => expect(salesService.updateSalesOrder).toHaveBeenCalledWith('order-1', expect.objectContaining({ sales_channel_id: 'channel-1' })));
  });

  it('retries when Order loading fails', async () => {
    salesService.getSalesChannels.mockRejectedValueOnce(new Error('Offline')).mockResolvedValueOnce(channels);
    const user = userEvent.setup();
    renderOrder();

    expect(await screen.findByRole('alert')).toHaveTextContent('Offline');
    await user.click(screen.getByRole('button', { name: 'Try again' }));
    expect(await screen.findByRole('heading', { name: 'New order' })).toBeInTheDocument();
    expect(salesService.getSalesChannels).toHaveBeenCalledTimes(2);
  });

  it('updates refunded payment and advances fulfilment through the lifecycle controls', async () => {
    const user = userEvent.setup();
    renderOrder('/sales/order-1');

    await screen.findByRole('heading', { name: 'WEB-1042' });
    await user.selectOptions(screen.getByLabelText('Payment status'), 'refunded');
    await user.click(screen.getByRole('button', { name: 'Update payment' }));
    await waitFor(() => expect(salesService.updateSalesOrderStatus).toHaveBeenCalledWith('order-1', { payment_status: 'refunded' }));
    await user.click(screen.getByRole('button', { name: 'Move to Confirmed' }));
    await waitFor(() => expect(salesService.updateSalesOrderStatus).toHaveBeenCalledWith('order-1', { fulfilment_status: 'confirmed' }));
  });

  it('blocks an item over-allocation before calling the API', async () => {
    const user = userEvent.setup();
    renderOrder();

    await screen.findByRole('heading', { name: 'New order' });
    await user.type(screen.getByLabelText('Reference'), 'OVER-ALLOCATED');
    await user.selectOptions(screen.getByLabelText('Model'), 'model-1');
    await user.type(screen.getByLabelText('Unit selling price (€)'), '10');
    await user.type(await screen.findByLabelText('Units assigned to Print Job job-1'), '2');
    await user.click(screen.getByRole('button', { name: 'Create order' }));

    expect(await screen.findByText('Assigned units cannot exceed the ordered quantity of 1')).toBeInTheDocument();
    expect(salesService.createSalesOrder).not.toHaveBeenCalled();
  });
});
