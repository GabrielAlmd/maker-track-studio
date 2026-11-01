import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import Sales from '../modules/sales/pages/Sales';
import * as salesService from '../modules/sales/services/salesService';
import * as modelService from '../services/modelService';

vi.mock('../modules/sales/services/salesService', () => ({
  getSalesOrders: vi.fn(),
  getSalesChannels: vi.fn(),
  deleteSalesOrder: vi.fn(),
}));

vi.mock('../services/modelService', () => ({ getModels: vi.fn() }));

const order = {
  id: 'order-1',
  reference: 'WEB-1042',
  order_date: '2026-09-18T00:00:00.000Z',
  sales_channel: { id: 'channel-1', name: 'Etsy' },
  fulfilment_status: 'confirmed',
  payment_status: 'paid',
  items: [{ id: 'item-1', model_name_snapshot: 'Modular Enclosure', quantity: 3, unit_price_cents: 2500 }],
  profitability: {
    state: 'calculated', revenue_cents: 7500, additional_costs_cents: 500,
    production_cost_cents: 2800, total_cost_cents: 3300, net_profit_cents: 4200,
    margin: { state: 'calculated', percent: 56 },
  },
};

function renderSales() {
  return render(<MemoryRouter><Sales /></MemoryRouter>);
}

describe('Sales list', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    salesService.getSalesChannels.mockResolvedValue([{ id: 'channel-1', name: 'Etsy', is_archived: false }]);
    modelService.getModels.mockResolvedValue([{ id: 'model-1', name: 'Modular Enclosure' }]);
  });

  it('renders order, financial and lifecycle summaries from a server page', async () => {
    salesService.getSalesOrders.mockResolvedValue({ items: [order], total: 1, page: 1, page_size: 25 });

    renderSales();

    expect(await screen.findByRole('link', { name: 'WEB-1042' })).toHaveAttribute('href', '/sales/order-1');
    expect(screen.getAllByText('Etsy')).toHaveLength(1);
    expect(screen.getByText('Modular Enclosure')).toBeInTheDocument();
    expect(screen.getByText('1 item · 3 units')).toBeInTheDocument();
    expect(screen.getByText('€75.00')).toBeInTheDocument();
    expect(screen.getByText('€28.00')).toBeInTheDocument();
    expect(screen.getByText('€42.00')).toBeInTheDocument();
    expect(screen.getByText('Confirmed')).toBeInTheDocument();
    expect(screen.getByText('Paid')).toBeInTheDocument();
  });

  it('keeps unavailable production-derived values distinct from zero', async () => {
    salesService.getSalesOrders.mockResolvedValue({
      items: [{ ...order, profitability: { state: 'not_calculated', reason: 'A Print Job cost is unavailable', revenue_cents: 7500, additional_costs_cents: 500, production_cost_cents: null, total_cost_cents: null, net_profit_cents: null, margin: { state: 'not_calculated' } } }],
      total: 1, page: 1, page_size: 25,
    });

    renderSales();

    expect(await screen.findByText('Unavailable')).toBeInTheDocument();
    expect(screen.getAllByText('Not calculated')).toHaveLength(3);
    expect(screen.queryByText('€0.00')).not.toBeInTheDocument();
  });

  it('sends filters and pagination to the server', async () => {
    salesService.getSalesOrders.mockResolvedValue({ items: [order], total: 30, page: 1, page_size: 25 });
    const user = userEvent.setup();
    renderSales();

    await screen.findByText('WEB-1042');
    await user.click(screen.getByText('Filters'));
    await user.selectOptions(screen.getByLabelText('Fulfilment status'), 'confirmed');
    expect(screen.getByText('Filters (1)')).toBeInTheDocument();
    await waitFor(() => expect(salesService.getSalesOrders).toHaveBeenCalledWith(expect.objectContaining({ fulfilment_status: 'confirmed', page: 1, pageSize: 25 })));
    await user.click(screen.getByRole('button', { name: 'Next page' }));
    await waitFor(() => expect(salesService.getSalesOrders).toHaveBeenCalledWith(expect.objectContaining({ fulfilment_status: 'confirmed', page: 2, pageSize: 25 })));
  });

  it('shows an API error with a retry action', async () => {
    salesService.getSalesOrders.mockRejectedValueOnce(new Error('Request failed')).mockResolvedValueOnce({ items: [], total: 0, page: 1, page_size: 25 });
    const user = userEvent.setup();
    renderSales();

    expect(await screen.findByText('Could not load Sales Orders')).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Try again' }));
    await waitFor(() => expect(salesService.getSalesOrders).toHaveBeenCalledTimes(2));
  });

  it('shows deterministic loading and empty states', async () => {
    let resolveOrders;
    salesService.getSalesOrders.mockReturnValue(new Promise((resolve) => { resolveOrders = resolve; }));
    const view = renderSales();
    expect(screen.getByRole('status', { name: 'Loading Sales Orders' })).toBeInTheDocument();
    resolveOrders({ items: [], total: 0, page: 1, page_size: 25 });
    expect(await screen.findByText('No orders yet')).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Create first order' })).toHaveAttribute('href', '/sales/new');
    view.unmount();
  });

  it('renders the optional no-Channel state instead of missing-data copy', async () => {
    salesService.getSalesOrders.mockResolvedValue({ items: [{ ...order, sales_channel: null, sales_channel_id: null }], total: 1, page: 1, page_size: 25 });
    renderSales();

    expect(await screen.findByText('No sales channel')).toBeInTheDocument();
    expect(screen.queryByText('Data unavailable')).not.toBeInTheDocument();
  });
});
