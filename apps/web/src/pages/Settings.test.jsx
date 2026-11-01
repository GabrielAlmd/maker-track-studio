import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import Settings from './Settings';
import toast from 'react-hot-toast';
import * as appConfigService from '../services/appConfigService';
import { ThemeProvider } from '../contexts/ThemeContext';
import { MemoryRouter, useLocation } from 'react-router-dom';
import { FeatureContext } from '../contexts/featureState';

vi.mock('../services/appConfigService', () => ({
  getAppConfig: vi.fn(),
  updateAppConfig: vi.fn(),
  exportDesktopData: vi.fn(),
  importDesktopData: vi.fn(),
}));


vi.mock('react-hot-toast', () => ({
  default: {
    success: vi.fn(),
    error: vi.fn(),
  },
}));

function LocationProbe() {
  const location = useLocation();
  return <output aria-label="Current settings URL">{location.pathname}{location.search}</output>;
}

describe('Settings page', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    window.localStorage.clear();
    delete document.documentElement.dataset.theme;
  });

  it('changes and persists the app theme', async () => {
    appConfigService.getAppConfig.mockResolvedValueOnce([]);
    const user = userEvent.setup();
    render(<MemoryRouter><ThemeProvider><Settings /></ThemeProvider></MemoryRouter>);
    const lightMode = await screen.findByRole('radio', { name: /light mode/i });
    expect(screen.getByRole('radio', { name: /dark mode/i })).toHaveAttribute('aria-checked', 'true');
    await user.click(lightMode);
    expect(lightMode).toHaveAttribute('aria-checked', 'true');
    expect(document.documentElement).toHaveAttribute('data-theme', 'light');
    expect(window.localStorage.getItem('maker-track-studio-theme')).toBe('light');
  });

  it('opens deep-linked sections and changes the section URL', async () => {
    appConfigService.getAppConfig.mockReset();
    appConfigService.getAppConfig.mockResolvedValue(configs);
    const user = userEvent.setup();
    render(<MemoryRouter initialEntries={['/settings?section=backups']}><Settings /><LocationProbe /></MemoryRouter>);
    expect(screen.getByText(/restore desktop backup/i)).toBeInTheDocument();
    await user.click(screen.getByRole('tab', { name: 'Costs & Calculations' }));
    expect(await screen.findByRole('radio', { name: /simple/i })).toBeInTheDocument();
    await user.click(screen.getByRole('tab', { name: 'Appearance' }));
    expect(await screen.findByRole('radio', { name: /dark mode/i })).toBeInTheDocument();
  });

  it('persists the Sales & Orders feature preference', async () => {
    const updateFeature = vi.fn().mockResolvedValue({ sales_orders: true });
    const value = { features: { sales_orders: false }, loading: false, saving: false, error: null, refetch: vi.fn(), updateFeature };
    const user = userEvent.setup();
    render(<FeatureContext.Provider value={value}><MemoryRouter initialEntries={['/settings?section=features']}><Settings /></MemoryRouter></FeatureContext.Provider>);

    expect(screen.getByText('Track orders, selling prices, production allocation, revenue and profit.')).toBeInTheDocument();
    await user.click(screen.getByRole('checkbox'));
    await waitFor(() => expect(updateFeature).toHaveBeenCalledWith('sales_orders', true));
    expect(toast.success).toHaveBeenCalledWith('Feature settings saved');
  });

  it.each([
    ['saved', { path: '/backups/backup.zip', cancelled: false }],
    ['cancelled', { path: '', cancelled: true }],
    ['failed', null],
  ])('handles a %s native export', async (outcome, result) => {
    appConfigService.getAppConfig.mockResolvedValueOnce([]);
    if (outcome === 'failed') {
      appConfigService.exportDesktopData.mockRejectedValueOnce('Could not write to the selected folder');
    } else {
      appConfigService.exportDesktopData.mockResolvedValueOnce(result);
    }
    const user = userEvent.setup();
    render(<MemoryRouter initialEntries={['/settings?section=backups']}><Settings /></MemoryRouter>);
    await user.click(await screen.findByRole('button', { name: /export data/i }));
    await waitFor(() => expect(appConfigService.exportDesktopData).toHaveBeenCalledOnce());
    expect(screen.getByRole('button', { name: /export data/i })).toBeEnabled();
    if (outcome === 'saved') {
      expect(toast.success).toHaveBeenCalledWith('Backup saved to /backups/backup.zip', { duration: 6000 });
    } else {
      expect(toast.success).not.toHaveBeenCalled();
    }
    if (outcome === 'failed') {
      expect(toast.error).toHaveBeenCalledWith('Could not write to the selected folder');
    } else {
      expect(toast.error).not.toHaveBeenCalled();
    }
  });

});

const configs = [
  ['machine_cost_mode', '0'], ['printer_purchase_cost', '299'], ['printer_upfront_cost', '23'],
  ['printer_annual_maintenance', '30'], ['printer_lifetime_years', '3'],
  ['printer_uptime_percent', '50'], ['printer_power_watts', '150'],
  ['printer_buffer_factor', '1.3'], ['electricity_cost_kwh', '0.20'],
  ['printer_wear_cost_hour', '0.08'], ['labor_cost_hour', '15'],
  ['filament_waste_factor', '1.1'],
].map(([key, value]) => ({ key, value, type: 'number' }));

async function openSettings() {
  vi.clearAllMocks();
  appConfigService.getAppConfig.mockReset();
  appConfigService.getAppConfig.mockResolvedValue(configs);
  appConfigService.updateAppConfig.mockResolvedValue({});
  render(<MemoryRouter initialEntries={['/settings?section=calculations']}><Settings /></MemoryRouter>);
  await screen.findByRole('radio', { name: /Simple/ });
  return userEvent.setup();
}

it('shows grouped inputs, locks inactive fields immediately and discards drafts', async () => {
  const user = await openSettings();
  const purchase = screen.getByRole('textbox', { name: 'Printer Purchase Cost (€)' });
  const wear = screen.getByRole('textbox', { name: 'Printer Wear Cost (per hour)' });
  expect(purchase).toHaveAttribute('readonly');
  expect(wear).not.toHaveAttribute('readonly');
  expect(screen.getByRole('button', { name: 'Save changes' })).toBeDisabled();
  await user.click(screen.getByRole('radio', { name: /Advanced/ }));
  expect(purchase).not.toHaveAttribute('readonly');
  expect(wear).toHaveAttribute('readonly');
  expect(screen.getByText('€0.0798')).toBeVisible();
  await user.clear(purchase);
  await user.type(purchase, '600');
  expect(screen.getByText('€0.1095')).toBeVisible();
  expect(appConfigService.updateAppConfig).not.toHaveBeenCalled();
  await user.click(screen.getByRole('button', { name: 'Discard changes' }));
  expect(purchase).toHaveValue('299');
  expect(purchase).toHaveAttribute('readonly');
  expect(screen.getByRole('radio', { name: /Simple/ })).toBeChecked();
  expect(screen.getByText('€0.2800')).toBeVisible();
});

it('saves several fields together, accepts decimal commas and saves mode last', async () => {
  const user = await openSettings();
  await user.click(screen.getByRole('radio', { name: /Advanced/ }));
  const labor = screen.getByRole('textbox', { name: 'Labor Cost (per hour)' });
  await user.clear(labor);
  await user.type(labor, '18,50');
  const power = screen.getByRole('textbox', { name: 'Average Printer Power (W)' });
  await user.clear(power);
  await user.type(power, '200');
  await user.click(screen.getByRole('button', { name: 'Save changes' }));
  await waitFor(() => expect(toast.success).toHaveBeenCalledWith('New-printer cost defaults saved'));
  expect(appConfigService.updateAppConfig.mock.calls).toEqual([
    ['printer_power_watts', '200', 'number'], ['labor_cost_hour', '18.50', 'number'], ['machine_cost_mode', '1', 'number'],
  ]);
  expect(screen.getByRole('button', { name: 'Save changes' })).toBeDisabled();
  expect(labor).toHaveValue('18.50');
});

it.each([
  ['Estimated Life (years)', '0', 'Value must be at least 0.001'],
  ['Estimated Uptime (%)', '101', 'Uptime must not exceed 100'],
  ['Printer Cost Buffer Factor', '0.5', 'Buffer must be at least 1'],
  ['Labor Cost (per hour)', '-2', 'Value must be non-negative'],
  ['Labor Cost (per hour)', 'abc', 'Must be a valid number'],
])('validates %s before any writes', async (label, value, message) => {
  const user = await openSettings();
  await user.click(screen.getByRole('radio', { name: /Advanced/ }));
  const input = screen.getByRole('textbox', { name: label });
  await user.clear(input);
  await user.type(input, value);
  await user.click(screen.getByRole('button', { name: 'Save changes' }));
  expect(await screen.findByText(message)).toBeVisible();
  expect(appConfigService.updateAppConfig).not.toHaveBeenCalled();
});

it('preserves drafts on partial failure and retries only remaining changes', async () => {
  const user = await openSettings();
  const electricity = screen.getByRole('textbox', { name: 'Electricity Cost (per hour)' });
  const labor = screen.getByRole('textbox', { name: 'Labor Cost (per hour)' });
  await user.clear(electricity);
  await user.type(electricity, '0.3');
  await user.clear(labor);
  await user.type(labor, '20');
  appConfigService.updateAppConfig.mockResolvedValueOnce({}).mockRejectedValueOnce(new Error('offline'));
  await user.click(screen.getByRole('button', { name: 'Save changes' }));
  expect(await screen.findByRole('alert')).toHaveTextContent('Could not save all changes');
  expect(labor).toHaveValue('20');
  expect(toast.success).not.toHaveBeenCalled();
  await user.click(screen.getByRole('button', { name: 'Save changes' }));
  await waitFor(() => expect(toast.success).toHaveBeenCalled());
  expect(appConfigService.updateAppConfig.mock.calls).toEqual([
    ['electricity_cost_kwh', '0.3', 'number'], ['labor_cost_hour', '20', 'number'], ['labor_cost_hour', '20', 'number'],
  ]);
});

it('keeps tooltip help available for locked inputs', async () => {
  const user = await openSettings();
  const help = screen.getByRole('button', { name: 'Help for Average Printer Power (W)' });
  await user.hover(help);
  expect(screen.getByRole('tooltip')).toBeVisible();
  await user.unhover(help);
  expect(screen.queryByRole('tooltip')).not.toBeInTheDocument();
  await user.click(help);
  expect(help).toHaveAttribute('aria-describedby', screen.getByRole('tooltip').id);
  await user.keyboard('{Escape}');
  expect(screen.queryByRole('tooltip')).not.toBeInTheDocument();
});

it('does not display a zero estimate for an empty advanced input', async () => {
  const user = await openSettings();
  await user.click(screen.getByRole('radio', { name: /Advanced/ }));
  await user.clear(screen.getByRole('textbox', { name: 'Printer Purchase Cost (€)' }));
  expect(within(screen.getByRole('complementary', { name: 'Machine cost preview' })).getByText('Not calculated')).toBeVisible();
});

it('migrates the old amount, adds, edits and removes equipment expenses', async () => {
  const user = await openSettings();
  expect(screen.getByRole('textbox', { name: 'Description 1' })).toHaveValue('Previous additional costs');
  expect(screen.getByRole('combobox', { name: 'Category 1' })).toHaveValue('additional');
  expect(screen.getByRole('textbox', { name: 'Amount (€) 1' })).toHaveValue('23');
  expect(screen.getByRole('button', { name: 'Add expense' })).toBeDisabled();
  await user.click(screen.getByRole('radio', { name: /Advanced/ }));
  await user.click(screen.getByRole('button', { name: 'Add expense' }));
  await user.type(screen.getByRole('textbox', { name: 'Description 2' }), 'PEI plate');
  await user.selectOptions(screen.getByRole('combobox', { name: 'Category 2' }), 'spare_parts');
  await user.type(screen.getByLabelText('Date (optional) 2'), '2026-09-17');
  await user.type(screen.getByRole('textbox', { name: 'Amount (€) 2' }), '25,50');
  expect(screen.getByText('Equipment total: €48.50')).toBeVisible();
  expect(screen.getByText('€347.50')).toBeVisible();
  await user.click(screen.getByRole('button', { name: 'Save changes' }));
  await waitFor(() => expect(toast.success).toHaveBeenCalled());
  const call = appConfigService.updateAppConfig.mock.calls.find(([key]) => key === 'printer_equipment_costs');
  expect(JSON.parse(call[1])).toEqual([
    { description: 'Previous additional costs', date: '', amount: 23, category: 'additional' },
    { description: 'PEI plate', date: '2026-09-17', amount: 25.5, category: 'spare_parts' },
  ]);
  expect(call[2]).toBe('json');
  const amount = screen.getByRole('textbox', { name: 'Amount (€) 2' });
  await user.clear(amount);
  await user.type(amount, '30');
  expect(screen.getByText('Equipment total: €53.00')).toBeVisible();
  await user.click(screen.getByRole('button', { name: 'Remove expense 1' }));
  expect(screen.getByText('Equipment total: €30.00')).toBeVisible();
  await user.click(screen.getByRole('button', { name: 'Remove expense 1' }));
  expect(screen.getByText('Equipment total: €0.00')).toBeVisible();
  await user.click(screen.getByRole('button', { name: 'Save changes' }));
  await waitFor(() => expect(appConfigService.updateAppConfig).toHaveBeenCalledWith('printer_equipment_costs', '[]', 'json'));
});

it('validates equipment descriptions and amounts before saving', async () => {
  const user = await openSettings();
  await user.click(screen.getByRole('radio', { name: /Advanced/ }));
  await user.click(screen.getByRole('button', { name: 'Add expense' }));
  await user.type(screen.getByRole('textbox', { name: 'Amount (€) 2' }), '-1');
  await user.click(screen.getByRole('button', { name: 'Save changes' }));
  expect(await screen.findByText('Description is required')).toBeVisible();
  expect(screen.getByText('Value must be non-negative')).toBeVisible();
  expect(appConfigService.updateAppConfig).not.toHaveBeenCalled();
});
