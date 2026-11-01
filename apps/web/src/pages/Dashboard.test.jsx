import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import Dashboard from './Dashboard';
import { getDashboardSummary } from '../services/printJobService';
import { getSalesSummary } from '../modules/sales/services/salesService';
import { FeatureContext } from '../contexts/featureState';

vi.mock('../services/printJobService', () => ({
  getDashboardSummary: vi.fn(),
}));
vi.mock('../modules/sales/services/salesService', () => ({ getSalesSummary: vi.fn() }));

const summary = {
  total_models: 2,
  total_filaments: 3,
  total_purchases: 4,
  total_print_jobs: 5,
  cost_stats: {
    total_units_printed: 8,
    total_filament_weight_g: 1250,
    calculated_jobs: 3,
    total_filament_cost: 20,
    total_electricity_cost: 2.5,
    total_wear_cost: 5,
    total_labor_cost: 15,
    total_cost: 42.5,
  },
  expenses: {
    printer_cost: 899,
    filament_cost: 115,
    spare_parts_cost: 29.9,
    additional_cost: 50,
    other_cost: 6.1,
    total_expenses: 1100,
  },
  recent_print_jobs: [
    {
      id: 'complete-job',
      print_date: '2026-04-02T00:00:00.000Z',
      units_printed: 2,
      labor_hours: 1.5,
      printer_name: 'A1 – Workshop',
      model: { state: 'assigned', id: 'model-1', name: 'Enclosure' },
      cost: { state: 'calculated', total: 12.5 },
    },
    {
      id: 'not-calculated-job',
      print_date: '2026-04-01T00:00:00.000Z',
      units_printed: 1,
      labor_hours: null,
      printer_name: 'Not assigned',
      model: { state: 'deleted', name: 'Deleted model' },
      cost: { state: 'not_calculated' },
    },
    {
      id: 'incomplete-job',
      print_date: 'not-a-date',
      units_printed: undefined,
      labor_hours: undefined,
      model: { state: 'not_assigned', name: 'Model not assigned' },
    },
  ],
};

const businessSummary = {
  active_order_count: 2,
  completed_order_count: 4,
  unpaid_order_count: 1,
  revenue_cents: 20000,
  production_cost_cents: 6000,
  packaging_cost_cents: 500,
  shipping_cost_cents: 1000,
  platform_fee_cents: 500,
  other_cost_cents: 0,
  total_cost_cents: 8000,
  net_profit_cents: 12000,
  margin: { state: 'calculated', percent: 60 },
  recent_orders: [{
    id: 'order-1', reference: 'WEB-1042', order_date: '2026-09-18T00:00:00.000Z',
    sales_channel: { id: 'channel-1', name: 'Etsy' }, fulfilment_status: 'completed', payment_status: 'paid', revenue_cents: 8400,
  }],
};

function renderDashboard(salesOrders = false) {
  const features = { features: { sales_orders: salesOrders }, loading: false };
  return render(<FeatureContext.Provider value={features}><MemoryRouter><Dashboard /></MemoryRouter></FeatureContext.Provider>);
}

describe('Dashboard page', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    getSalesSummary.mockResolvedValue(businessSummary);
  });

  it('prioritizes operational KPIs and groups secondary cost and inventory information', async () => {
    getDashboardSummary.mockResolvedValue(summary);
    renderDashboard();

    expect(await screen.findByRole('heading', { name: 'Print Jobs' })).toBeInTheDocument();
    expect(screen.getByRole('heading', { name: 'Units Printed' })).toBeInTheDocument();
    expect(screen.getByRole('heading', { name: 'Estimated Cost' })).toBeInTheDocument();
    expect(screen.getByRole('heading', { name: 'Cost Breakdown' })).toBeInTheDocument();
    expect(screen.getByRole('heading', { name: 'Inventory' })).toBeInTheDocument();
    expect(screen.getByText('Models')).toBeInTheDocument();
    expect(screen.getByText('Filament Used')).toBeInTheDocument();
    expect(screen.getByText('€42.50')).toBeInTheDocument();
    expect(screen.getByText('Based on 3 calculated print jobs')).toBeInTheDocument();
    expect(screen.getByText('Filament')).toBeInTheDocument();
    expect(screen.getByText('€20.00')).toBeInTheDocument();
    expect(screen.getByText('1.25 kg')).toBeInTheDocument();
  });

  it.each(['dark', 'light'])('keeps the same metric hierarchy in %s mode', async (theme) => {
    document.documentElement.dataset.theme = theme;
    getDashboardSummary.mockResolvedValue(summary);
    renderDashboard();

    await screen.findByRole('heading', { name: 'Inventory' });
    expect(document.querySelectorAll('.dashboard-kpi-card')).toHaveLength(4);
    expect(document.querySelectorAll('.dashboard-secondary-panel')).toHaveLength(2);
    expect(document.querySelectorAll('.dashboard-inventory-row')).toHaveLength(3);
  });

  it('shows recorded expenses by category separately from production costs', async () => {
    getDashboardSummary.mockResolvedValue(summary);
    renderDashboard();

    const panel = await screen.findByRole('region', { name: 'Expenses' });
    expect(within(panel).getByText('€1,100.00')).toBeInTheDocument();
    expect(within(panel).getByText('Printer acquisition')).toBeInTheDocument();
    expect(within(panel).getByText('€899.00')).toBeInTheDocument();
    expect(within(panel).getByText('81.7%')).toBeInTheDocument();
    expect(within(panel).getByText(/separate from calculated production cost/i)).toBeInTheDocument();
  });

  it('shows a safe empty state when no expenses are recorded', async () => {
    getDashboardSummary.mockResolvedValue({
      ...summary,
      expenses: { printer_cost: 0, filament_cost: 0, spare_parts_cost: 0, additional_cost: 0, other_cost: 0, total_expenses: 0 },
    });
    renderDashboard();

    const panel = await screen.findByRole('region', { name: 'Expenses' });
    expect(within(panel).getByText('€0.00')).toBeInTheDocument();
    expect(within(panel).getByText('No recorded expenses')).toBeInTheDocument();
  });

  it('offers spreadsheet migration when the workspace is empty', async () => {
    getDashboardSummary.mockResolvedValue({
      ...summary,
      total_models: 0, total_filaments: 0, total_purchases: 0, total_print_jobs: 0,
      recent_print_jobs: [],
    });
    renderDashboard();

    expect(await screen.findByRole('heading', { name: 'Already have spreadsheet records?' })).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Start data migration' })).toHaveAttribute('href', '/settings?section=migration');
  });

  it('uses the print-job representation, safe model links, and cost status indicators', async () => {
    getDashboardSummary.mockResolvedValue(summary);
    renderDashboard();

    expect(await screen.findByRole('link', { name: 'Enclosure' })).toHaveAttribute('href', '/models');
    expect(screen.queryByRole('link', { name: 'Deleted model' })).not.toBeInTheDocument();
    expect(screen.getByText('Complete')).toBeInTheDocument();
    expect(screen.getAllByText('Not calculated')).toHaveLength(3);
    expect(screen.getByText('Incomplete')).toBeInTheDocument();
    expect(screen.getByRole('columnheader', { name: 'Status' })).toBeInTheDocument();
    expect(screen.getByRole('columnheader', { name: 'Printer' })).toBeInTheDocument();
    expect(screen.getByText('A1 – Workshop')).toBeInTheDocument();
    expect(screen.getAllByText('Not recorded')).toHaveLength(2);
    expect(screen.getAllByText('Data unavailable')).toHaveLength(2);
  });

  it('does not present aggregate zeroes as calculated costs', async () => {
    getDashboardSummary.mockResolvedValue({
      ...summary,
      cost_stats: { ...summary.cost_stats, calculated_jobs: 0, total_cost: 0 },
    });
    renderDashboard();

    expect(await screen.findByText('No calculated print jobs')).toBeInTheDocument();
    expect(screen.queryByText('€0.00')).not.toBeInTheDocument();
  });

  it('keeps loading, empty, and error states available', async () => {
    let resolveSummary;
    getDashboardSummary.mockReturnValue(new Promise((resolve) => { resolveSummary = resolve; }));
    const { unmount } = renderDashboard();
    expect(screen.getByRole('status', { name: 'Loading dashboard' })).toBeInTheDocument();

    resolveSummary({ ...summary, recent_print_jobs: [] });
    expect(await screen.findByText('No print jobs yet')).toBeInTheDocument();

    unmount();
    getDashboardSummary.mockRejectedValueOnce(new Error('Network unavailable'));
    renderDashboard();
    await waitFor(() => expect(screen.getByRole('alert')).toHaveTextContent('Network unavailable'));
  });

  it('retries after a dashboard error', async () => {
    getDashboardSummary.mockRejectedValueOnce(new Error('Network unavailable')).mockResolvedValueOnce(summary);
    const user = userEvent.setup();
    renderDashboard();

    await user.click(await screen.findByRole('button', { name: 'Try again' }));
    await waitFor(() => expect(screen.getByText('€42.50')).toBeInTheDocument());
    expect(getDashboardSummary).toHaveBeenCalledTimes(2);
  });

  it('does not fetch or render business metrics while Sales is disabled', async () => {
    getDashboardSummary.mockResolvedValue(summary);
    renderDashboard(false);
    await screen.findByRole('heading', { name: 'Production' });
    expect(screen.queryByRole('heading', { name: 'Business' })).not.toBeInTheDocument();
    expect(getSalesSummary).not.toHaveBeenCalled();
  });

  it('shows business KPIs and recent Orders when Sales is enabled', async () => {
    getDashboardSummary.mockResolvedValue(summary);
    renderDashboard(true);

    expect(await screen.findByRole('heading', { name: 'Orders' })).toBeInTheDocument();
    expect(screen.getByRole('heading', { name: 'Business' })).toBeInTheDocument();
    expect(screen.getByRole('heading', { name: 'Revenue' })).toBeInTheDocument();
    expect(screen.getByRole('heading', { name: 'Net Profit' })).toBeInTheDocument();
    expect(screen.getByRole('heading', { name: 'Margin' })).toBeInTheDocument();
    expect(screen.getByText('4 completed · 1 unpaid')).toBeInTheDocument();
    expect(screen.getByText('€200.00')).toBeInTheDocument();
    expect(screen.getByText('€120.00')).toBeInTheDocument();
    expect(screen.getByText('60.0%')).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'WEB-1042' })).toHaveAttribute('href', '/sales/order-1');
    expect(screen.getByText('Etsy')).toBeInTheDocument();
    expect(getSalesSummary).toHaveBeenCalledOnce();
  });

  it('keeps Production usable and retries only the failed Sales summary', async () => {
    getDashboardSummary.mockResolvedValue(summary);
    getSalesSummary.mockRejectedValueOnce(new Error('Sales unavailable')).mockResolvedValueOnce(businessSummary);
    const user = userEvent.setup();
    renderDashboard(true);

    expect(await screen.findByText('€42.50')).toBeInTheDocument();
    const alert = await screen.findByRole('alert');
    expect(alert).toHaveTextContent('Could not load business metrics');
    expect(alert).toHaveTextContent('Sales unavailable');
    await user.click(screen.getByRole('button', { name: 'Try again' }));
    expect(await screen.findByRole('link', { name: 'WEB-1042' })).toBeInTheDocument();
    expect(getDashboardSummary).toHaveBeenCalledOnce();
    expect(getSalesSummary).toHaveBeenCalledTimes(2);
  });

  it('shows scoped Business loading and empty states', async () => {
    let resolveSales;
    getDashboardSummary.mockResolvedValue(summary);
    getSalesSummary.mockReturnValue(new Promise((resolve) => { resolveSales = resolve; }));
    renderDashboard(true);

    expect(await screen.findByRole('status', { name: 'Loading business metrics' })).toBeInTheDocument();
    resolveSales({ ...businessSummary, recent_orders: [] });
    expect(await screen.findByText('No recent orders')).toBeInTheDocument();
    expect(screen.getByText('Create an order to start tracking business activity.')).toBeInTheDocument();
  });
});
