import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import Printers from './Printers';
import { COST_SETTINGS } from './costSettings';
import * as printerService from '../services/printerService';
import * as appConfigService from '../services/appConfigService';
import toast from 'react-hot-toast';

vi.mock('../services/printerService', () => ({
  getPrinters: vi.fn(), createPrinter: vi.fn(), updatePrinter: vi.fn(),
  setDefaultPrinter: vi.fn(), archivePrinter: vi.fn(), restorePrinter: vi.fn(),
  updatePrinterCostConfig: vi.fn(),
}));
vi.mock('../services/appConfigService', () => ({ getAppConfig: vi.fn() }));
vi.mock('react-hot-toast', () => ({ default: { success: vi.fn(), error: vi.fn() } }));

const costConfig = Object.fromEntries(Object.keys(COST_SETTINGS).map((key) => [key, key === 'machine_cost_mode' ? 0 : key === 'filament_waste_factor' || key === 'printer_buffer_factor' ? 1.1 : key === 'printer_lifetime_years' ? 3 : key === 'printer_uptime_percent' ? 50 : 10]));
const printers = [
  { id: 'p1', name: 'A1 – Workshop', manufacturer: 'Bambu Lab', model: 'A1', notes: '', is_default: true, is_archived: false, cost_config: costConfig, updated_at: '1' },
  { id: 'p2', name: 'A1 – Office', manufacturer: 'Bambu Lab', model: 'A1', notes: 'Second physical unit', is_default: false, is_archived: false, cost_config: costConfig, updated_at: '1' },
  { id: 'p3', name: 'Old machine', manufacturer: 'Prusa', model: 'MK3', notes: '', is_default: false, is_archived: true, cost_config: costConfig, updated_at: '1' },
];

describe('Printers page', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    printerService.getPrinters.mockResolvedValue(printers);
    appConfigService.getAppConfig.mockResolvedValue(Object.entries(costConfig).map(([key, value]) => ({ key, value: String(value), type: 'number' })));
  });

  it('distinguishes physical printers with the same model and manages lifecycle', async () => {
    const user = userEvent.setup();
    render(<Printers />);
    expect(await screen.findByText('A1 – Workshop')).toBeInTheDocument();
    expect(screen.getByText('A1 – Office')).toBeInTheDocument();
    expect(screen.getAllByText('Bambu Lab · A1')).toHaveLength(2);
    await user.click(screen.getAllByRole('button', { name: /set default/i })[0]);
    await waitFor(() => expect(printerService.setDefaultPrinter).toHaveBeenCalledWith('p2'));
    await user.click(screen.getByRole('button', { name: /restore/i }));
    await waitFor(() => expect(printerService.restorePrinter).toHaveBeenCalledWith('p3'));
  });

  it('creates a trimmed physical printer profile', async () => {
    printerService.createPrinter.mockResolvedValue({ id: 'p4' });
    const user = userEvent.setup();
    render(<Printers />);
    await user.click(await screen.findByRole('button', { name: 'Add Printer' }));
    await user.type(screen.getByLabelText('Name or nickname'), '  P1S – Enclosure  ');
    await user.type(screen.getByLabelText('Manufacturer'), '  Bambu Lab  ');
    await user.type(screen.getByLabelText('Model'), '  P1S  ');
    await user.click(screen.getByRole('button', { name: 'Create printer' }));
    await waitFor(() => expect(printerService.createPrinter).toHaveBeenCalledWith({
      name: 'P1S – Enclosure', manufacturer: 'Bambu Lab', model: 'P1S', notes: '',
    }));
  });

  it('previews an opt-in preset, allows editing it, and saves hardware values with workspace economics', async () => {
    printerService.createPrinter.mockResolvedValue({ id: 'p4' });
    const user = userEvent.setup();
    render(<Printers />);
    await user.click(await screen.findByRole('button', { name: 'Add Printer' }));
    await user.selectOptions(screen.getByLabelText('Choose a printer preset'), 'bambu-lab-a1-v1');
    expect(screen.getByText(/Example purchase price: 299 EUR/)).toBeInTheDocument();
    expect(printerService.createPrinter).not.toHaveBeenCalled();
    await user.click(screen.getByRole('button', { name: 'Apply preset values' }));
    const purchase = screen.getByLabelText('Printer Purchase Cost (€)');
    await user.clear(purchase);
    await user.type(purchase, '320');
    await user.type(screen.getByLabelText('Name or nickname'), 'My A1');
    await user.click(screen.getByRole('button', { name: 'Create printer' }));
    await waitFor(() => expect(printerService.createPrinter).toHaveBeenCalledWith(expect.objectContaining({
      manufacturer: 'Bambu Lab', model: 'A1', cost_config: expect.objectContaining({
        machine_cost_mode: 1, printer_purchase_cost: 320, printer_power_watts: 150,
      }),
    })));
    expect(printerService.createPrinter.mock.calls[0][0].cost_config).not.toHaveProperty('labor_cost_hour');
  });

  it('requires explicit confirmation before applying a preset to an existing profile', async () => {
    const user = userEvent.setup();
    render(<Printers />);
    await screen.findByText('A1 – Workshop');
    await user.click(screen.getAllByRole('button', { name: 'Edit costs' })[0]);
    await user.selectOptions(screen.getByLabelText('Choose a printer preset'), 'bambu-lab-p1s-v1');
    await user.click(screen.getByRole('button', { name: 'Apply preset values' }));
    expect(screen.getByRole('dialog', { name: 'Review preset changes' })).toBeInTheDocument();
    expect(printerService.updatePrinterCostConfig).not.toHaveBeenCalled();
    await user.click(screen.getByRole('button', { name: 'Apply these changes to this profile' }));
    await user.click(screen.getByRole('button', { name: 'Save printer costs' }));
    await waitFor(() => expect(printerService.updatePrinterCostConfig).toHaveBeenCalledWith('p1', expect.objectContaining({
      printer_purchase_cost: 599, printer_power_watts: 180, labor_cost_hour: costConfig.labor_cost_hour,
    })));
  });

  it('offers the complete A1 reference values as a separate explicit choice', async () => {
    printerService.createPrinter.mockResolvedValue({ id: 'p4' });
    const user = userEvent.setup();
    render(<Printers />);
    await user.click(await screen.findByRole('button', { name: 'Add Printer' }));
    await user.selectOptions(screen.getByLabelText('Choose a printer preset'), 'bambu-lab-a1-v1');
    await user.click(screen.getByRole('checkbox', { name: /Use complete A1 reference profile/ }));
    await user.click(screen.getByRole('button', { name: 'Apply preset values' }));
    expect(screen.getByLabelText('Labor Cost (per hour)')).toHaveValue(25);
    expect(screen.getByLabelText('Printer Wear Cost (per hour)')).toHaveValue(0.08);
    await user.type(screen.getByLabelText('Name or nickname'), 'Reference A1');
    await user.click(screen.getByRole('button', { name: 'Create printer' }));
    await waitFor(() => expect(printerService.createPrinter).toHaveBeenCalledWith(expect.objectContaining({
      cost_config: expect.objectContaining({
        electricity_cost_kwh: 0.2, filament_waste_factor: 1.1, labor_cost_hour: 25,
        printer_wear_cost_hour: 0.08, margin_tier_1: 80, margin_tier_2: 90, margin_tier_3: 99,
      }),
    })));
  });

  it('saves a complete numeric cost configuration without changing other printers', async () => {
    printerService.updatePrinterCostConfig.mockResolvedValue(printers[0]);
    const user = userEvent.setup();
    render(<Printers />);
    await screen.findByText('A1 – Workshop');
    await user.click(screen.getAllByRole('button', { name: 'Edit costs' })[0]);
    const labor = screen.getByLabelText(COST_SETTINGS.labor_cost_hour[0]);
    await user.clear(labor);
    await user.type(labor, '22.5');
    await user.click(screen.getByRole('button', { name: 'Save printer costs' }));
    await waitFor(() => expect(printerService.updatePrinterCostConfig).toHaveBeenCalledWith('p1', expect.objectContaining({
      labor_cost_hour: 22.5, machine_cost_mode: 0,
    })));
    expect(printerService.updatePrinterCostConfig).not.toHaveBeenCalledWith('p2', expect.anything());
    const submitted = printerService.updatePrinterCostConfig.mock.calls[0][1];
    expect(Object.keys(submitted).sort()).toEqual(Object.keys(COST_SETTINGS).sort());
    expect(Object.values(submitted).every(Number.isFinite)).toBe(true);
  });

  it('keeps backend conflict details out of the page and reports a user-facing failure', async () => {
    printerService.archivePrinter.mockRejectedValue(new Error('Select another active default printer first'));
    const user = userEvent.setup();
    render(<Printers />);
    await screen.findByText('A1 – Workshop');
    await user.click(screen.getAllByRole('button', { name: 'Archive' })[0]);
    await waitFor(() => expect(toast.error).toHaveBeenCalledWith('Select another active default printer first'));
  });
});
