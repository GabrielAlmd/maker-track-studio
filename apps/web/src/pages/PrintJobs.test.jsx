import { apiPage } from '../test/apiFixtures';
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import PrintJobs from './PrintJobs';
import { FeatureContext } from '../contexts/featureState';
import * as printJobService from '../services/printJobService';
import * as salesService from '../modules/sales/services/salesService';
import * as modelService from '../services/modelService';
import * as printJobFilamentService from '../services/printJobFilamentService';
import * as filamentService from '../services/filamentService';
import * as printerService from '../services/printerService';

vi.mock('../services/printJobService', () => ({
  getPrintJobs: vi.fn(),
  getPrintJobCost: vi.fn(),
  createPrintJob: vi.fn(),
  updatePrintJob: vi.fn(),
  deletePrintJob: vi.fn(),
}));

vi.mock('../modules/sales/services/salesService', () => ({
  getPrintJobAllocations: vi.fn(),
}));

vi.mock('../services/modelService', () => ({
  getModels: vi.fn(),
  createModel: vi.fn(),
  updateModel: vi.fn(),
  deleteModel: vi.fn(),
}));

vi.mock('../services/filamentService', () => ({
  getFilaments: vi.fn(),
  createFilament: vi.fn(),
  updateFilament: vi.fn(),
  deleteFilament: vi.fn(),
}));

vi.mock('../services/printJobFilamentService', () => ({
  createPrintJobFilament: vi.fn(),
  deletePrintJobFilament: vi.fn(),
}));

vi.mock('../services/printerService', () => ({ getPrinters: vi.fn() }));

vi.mock('react-hot-toast', () => ({
  default: {
    success: vi.fn(),
    error: vi.fn(),
  },
}));

describe('PrintJobs page', () => {
  async function chooseModel(user, name = 'Gear') {
    const selector = screen.getByRole('combobox', { name: 'Model' });
    await user.click(selector);
    if (name) await user.type(selector, name);
    await user.click(screen.getByRole('option', { name: name || 'No model' }));
  }

  beforeEach(() => {
    vi.clearAllMocks();
    printerService.getPrinters.mockResolvedValue([{ id: 'p1', name: 'A1 – Workshop', is_default: true, is_archived: false }]);
    salesService.getPrintJobAllocations.mockImplementation(async (id) => ({
      print_job: { id, units_printed: 1, units_allocated: 0, units_available: 1 },
      used_by: [],
    }));
  });

  it('calculates model estimates by quantity and saves the selected filament', async () => {
    printJobService.getPrintJobs.mockResolvedValue(apiPage([]));
    modelService.getModels.mockResolvedValue([{ id: 'm1', name: 'Gear', print_time: 60, material_weight: 12.34, waste_material: 0 }]);
    filamentService.getFilaments.mockResolvedValue([{ id: 'f1', brand: 'Generic', type: 'PLA', color: 'Black' }]);
    printJobService.createPrintJob.mockResolvedValue({ id: 'new-job' });
    const user = userEvent.setup();
    render(<PrintJobs />);
    await user.click(await screen.findByRole('button', { name: /add first print job/i }));
    await chooseModel(user);
    const time = screen.getByPlaceholderText(/e.g. 90/i);
    expect(time).toHaveValue(60);
    expect(screen.getByPlaceholderText('g')).toHaveValue(12.34);
    fireEvent.change(document.querySelector('input[name="units_printed"]'), { target: { value: '3' } });
    expect(time).toHaveValue(180);
    expect(screen.getByPlaceholderText('g')).toHaveValue(37.02);
    const filament = screen.getByRole('combobox', { name: 'Filament 1' });
    await user.type(filament, 'black');
    await user.click(screen.getByRole('option', { name: /Black$/ }));
    fireEvent.change(document.querySelector('input[name="print_date"]'), { target: { value: '2026-04-03' } });
    await user.click(screen.getByRole('button', { name: /create print job/i }));
    await waitFor(() => expect(printJobFilamentService.createPrintJobFilament).toHaveBeenCalledWith({
      print_job_id: 'new-job', filament_id: 'f1', weight_used: 37.02,
    }));
    expect(printJobService.createPrintJob).toHaveBeenCalledWith(expect.objectContaining({
      model_id: 'm1', units_printed: 3, print_time_minutes: 180,
    }));
  });

  it('includes decimal model waste exactly once in the automatic filament weight', async () => {
    printJobService.getPrintJobs.mockResolvedValue(apiPage([]));
    modelService.getModels.mockResolvedValue([{
      id: 'm1', name: 'Gear', print_time: 60, material_weight: 12.34, waste_material: 1.11,
    }]);
    filamentService.getFilaments.mockResolvedValue([{ id: 'f1', brand: 'Generic', type: 'PLA', color: 'Black' }]);
    printJobService.createPrintJob.mockResolvedValue({ id: 'new-job' });
    const user = userEvent.setup();
    render(<PrintJobs />);

    await user.click(await screen.findByRole('button', { name: /add first print job/i }));
    expect(screen.getByRole('combobox', { name: 'Printer' })).toHaveAttribute('aria-required', 'true');
    expect(document.querySelector('input[name="print_date"]')).toHaveAttribute('aria-required', 'true');
    expect(document.querySelector('input[name="units_printed"]')).toHaveAttribute('aria-required', 'true');
    expect(document.querySelector('input[name="labor_hours"]')).toHaveAttribute('aria-required', 'true');
    expect(screen.getByText('Print Date').parentElement).toHaveTextContent('(required)');
    await chooseModel(user);
    expect(screen.getByPlaceholderText('g')).toHaveValue(13.45);

    fireEvent.change(document.querySelector('input[name="units_printed"]'), { target: { value: '3' } });
    expect(screen.getByPlaceholderText('g')).toHaveValue(40.35);
    const filament = screen.getByRole('combobox', { name: 'Filament 1' });
    await user.type(filament, 'black');
    await user.click(screen.getByRole('option', { name: /Black$/ }));
    fireEvent.change(document.querySelector('input[name="print_date"]'), { target: { value: '2026-04-03' } });
    await user.click(screen.getByRole('button', { name: /create print job/i }));

    await waitFor(() => expect(printJobFilamentService.createPrintJobFilament).toHaveBeenCalledWith({
      print_job_id: 'new-job', filament_id: 'f1', weight_used: 40.35,
    }));
  });

  it('submits manually entered two-decimal filament weight unchanged', async () => {
    printJobService.getPrintJobs.mockResolvedValue(apiPage([]));
    modelService.getModels.mockResolvedValue([]);
    filamentService.getFilaments.mockResolvedValue([{ id: 'f1', brand: 'Generic', type: 'PLA', color: 'Black' }]);
    printJobService.createPrintJob.mockResolvedValue({ id: 'new-job' });
    const user = userEvent.setup();
    render(<PrintJobs />);

    await user.click(await screen.findByRole('button', { name: /add first print job/i }));
    await user.click(screen.getByRole('button', { name: /add filament/i }));
    const weight = screen.getByPlaceholderText('g');
    expect(weight).toHaveAttribute('step', '0.01');
    const filament = screen.getByRole('combobox', { name: 'Filament 1' });
    await user.type(filament, 'black');
    await user.click(screen.getByRole('option', { name: /Black$/ }));
    await user.type(weight, '12.34');
    fireEvent.change(document.querySelector('input[name="print_date"]'), { target: { value: '2026-04-03' } });
    await user.click(screen.getByRole('button', { name: /create print job/i }));

    await waitFor(() => expect(printJobFilamentService.createPrintJobFilament).toHaveBeenCalledWith({
      print_job_id: 'new-job', filament_id: 'f1', weight_used: 12.34,
    }));
  });

  it('keeps manual overrides when changing quantity and clears automatic values when deselecting', async () => {
    printJobService.getPrintJobs.mockResolvedValue(apiPage([]));
    modelService.getModels.mockResolvedValue([{ id: 'm1', name: 'Gear', print_time: 60, material_weight: 12.5, waste_material: 2.5 }]);
    filamentService.getFilaments.mockResolvedValue([]);
    const user = userEvent.setup();
    render(<PrintJobs />);
    await user.click(await screen.findByRole('button', { name: /add first print job/i }));
    await chooseModel(user);
    const time = screen.getByPlaceholderText(/e.g. 90/i);
    fireEvent.change(time, { target: { value: '80' } });
    fireEvent.change(screen.getByPlaceholderText('g'), { target: { value: '20' } });
    fireEvent.change(document.querySelector('input[name="units_printed"]'), { target: { value: '2' } });
    expect(time).toHaveValue(80);
    expect(screen.getByPlaceholderText('g')).toHaveValue(20);
    await chooseModel(user, '');
    await chooseModel(user);
    expect(time).toHaveValue(120);
    expect(screen.getByPlaceholderText('g')).toHaveValue(30);
    await chooseModel(user, '');
    expect(time).toHaveValue(null);
    expect(screen.queryByPlaceholderText('g')).not.toBeInTheDocument();
  });

  it('keeps saved estimates on edit and replaces persisted filament rows', async () => {
    printJobService.getPrintJobs.mockResolvedValue(apiPage([{
      id: 'j1', printer_id: 'p1', model_id: 'm1', print_date: '2026-04-01T00:00:00.000Z',
      units_printed: 2, labor_hours: 0, print_time_minutes: 95,
      filaments: [{ id: 'pjf1', filament_id: 'f1', weight_used: 30 }],
    }]));
    modelService.getModels.mockResolvedValue([{ id: 'm1', name: 'Gear', print_time: 60, material_weight: 12.5, waste_material: 2.5 }]);
    filamentService.getFilaments.mockResolvedValue([{ id: 'f1', brand: 'Generic', type: 'PLA', color: 'Black' }]);
    const user = userEvent.setup();
    render(<PrintJobs />);
    await user.click(await screen.findByRole('button', { name: /edit/i }));
    fireEvent.change(document.querySelector('input[name="units_printed"]'), { target: { value: '3' } });
    expect(screen.getByPlaceholderText(/e.g. 90/i)).toHaveValue(95);
    expect(screen.getByPlaceholderText('g')).toHaveValue(30);
    await user.click(screen.getByRole('button', { name: /save changes/i }));
    await waitFor(() => expect(printJobFilamentService.deletePrintJobFilament).toHaveBeenCalledWith('pjf1'));
    await waitFor(() => expect(printJobFilamentService.createPrintJobFilament).toHaveBeenCalledWith({
      print_job_id: 'j1', filament_id: 'f1', weight_used: 30,
    }));
  });

  it('renders estimated cost from API response', async () => {
    printJobService.getPrintJobs.mockResolvedValueOnce(apiPage([
      {
        id: 'j1',
        model_id: 'm1',
		model: { state: 'assigned', id: 'm1', name: 'Gear' },
        print_date: '2026-04-01T00:00:00.000Z',
        units_printed: 2,
        labor_hours: 1.5,
        print_time_minutes: 120,
        estimated_cost: 28.01,
        filaments: [{ id: 'pjf1', filament_id: 'f1', weight_used: 150 }],
      },
    ]));
    modelService.getModels.mockResolvedValueOnce([{ id: 'm1', name: 'Gear', print_time: 120 }]);
    filamentService.getFilaments.mockResolvedValueOnce([
      { id: 'f1', brand: 'Generic', type: 'PLA', color: 'Black', avg_price: 30 },
    ]);

    render(<PrintJobs />);

    expect(await screen.findByText('Gear')).toBeInTheDocument();
    expect(screen.getByText('€28.01')).toBeInTheDocument();
  });

  it('shows allocation capacity and supports retry when metadata fails to load', async () => {
    printJobService.getPrintJobs.mockResolvedValue(apiPage([{
      id: 'j1', model_id: 'm1', model: { state: 'assigned', id: 'm1', name: 'Gear' },
      print_date: '2026-04-01T00:00:00.000Z', units_printed: 6, labor_hours: 0,
      print_time_minutes: 120, filaments: [],
    }]));
    modelService.getModels.mockResolvedValue([{ id: 'm1', name: 'Gear' }]);
    filamentService.getFilaments.mockResolvedValue([]);
    salesService.getPrintJobAllocations.mockRejectedValueOnce(new Error('Network failure')).mockResolvedValue({
      print_job: { id: 'j1', model_id: 'm1', units_printed: 6, units_allocated: 4, units_available: 2 }, used_by: [],
    });

    const user = userEvent.setup();
    render(<FeatureContext.Provider value={{ features: { sales_orders: true } }}><PrintJobs /></FeatureContext.Provider>);

    const article = await screen.findByRole('article', { name: 'Gear print job' });
    expect(await within(article).findByRole('alert')).toHaveTextContent('Could not load allocation capacity.');
    await user.click(within(article).getByRole('button', { name: 'Try again' }));

    const capacity = within(article).getByLabelText('Production capacity');
    await waitFor(() => {
      expect(within(capacity).getByText('Units printed').parentElement).toHaveTextContent('6');
      expect(within(capacity).getByText('Units allocated').parentElement).toHaveTextContent('4');
      expect(within(capacity).getByText('Units available').parentElement).toHaveTextContent('2');
    });
    expect(salesService.getPrintJobAllocations).toHaveBeenCalledTimes(2);
  });

  it('shows validation errors when submitting empty print job form', async () => {
    printJobService.getPrintJobs.mockResolvedValueOnce(apiPage([]));
    modelService.getModels.mockResolvedValueOnce([]);
    filamentService.getFilaments.mockResolvedValueOnce([]);
    const user = userEvent.setup();

    render(<PrintJobs />);

    await user.click(await screen.findByRole('button', { name: /add first print job/i }));
    const spinButtons = screen.getAllByRole('spinbutton');
    await user.clear(spinButtons[0]);
    await user.click(screen.getByRole('button', { name: /create print job/i }));

    expect(await screen.findByText('Print date is required')).toBeInTheDocument();
    expect(screen.getByText('Units must be a number')).toBeInTheDocument();
  });

  it('submits valid print job payload with nullable model', async () => {
    printJobService.getPrintJobs.mockResolvedValue(apiPage([]));
    modelService.getModels.mockResolvedValue([]);
    filamentService.getFilaments.mockResolvedValue([]);
    printJobService.createPrintJob.mockResolvedValueOnce({ id: 'job-1' });
    printJobService.getPrintJobs.mockResolvedValueOnce(apiPage([]));

    const user = userEvent.setup();
    render(<PrintJobs />);

    await user.click(await screen.findByRole('button', { name: /add first print job/i }));

    const printDateInput = document.querySelector('input[name="print_date"]');
    if (!printDateInput) {
      throw new Error('print_date input not found');
    }
    await user.type(printDateInput, '2026-04-03');
    const spinButtons = screen.getAllByRole('spinbutton');
    await user.clear(spinButtons[0]);
    await user.type(spinButtons[0], '3');
    await user.clear(spinButtons[1]);
    await user.type(spinButtons[1], '1.5');
    await user.type(screen.getByPlaceholderText(/e.g. 90/i), '110');
    await user.click(screen.getByRole('button', { name: /create print job/i }));

    await waitFor(() => {
      expect(printJobService.createPrintJob).toHaveBeenCalledWith({
        printer_id: 'p1',
        model_id: null,
        print_date: '2026-04-03T00:00:00.000Z',
        units_printed: 3,
        labor_hours: 1.5,
        print_time_minutes: 110,
      });
    });
  });

  it('preselects the default printer and submits an explicit alternative', async () => {
    printerService.getPrinters.mockResolvedValue([
      { id: 'p1', name: 'Default', is_default: true, is_archived: false },
      { id: 'p2', name: 'Selected', is_default: false, is_archived: false },
      { id: 'p3', name: 'Archived', is_default: false, is_archived: true },
    ]);
    printJobService.getPrintJobs.mockResolvedValue(apiPage([]));
    modelService.getModels.mockResolvedValue([]);
    filamentService.getFilaments.mockResolvedValue([]);
    printJobService.createPrintJob.mockResolvedValue({ id: 'job-2' });

    const user = userEvent.setup();
    render(<PrintJobs />);

    await user.click(await screen.findByText('Filters'));
    await user.selectOptions(screen.getByRole('listbox', { name: 'Printer' }), 'p2');
    await waitFor(() => expect(printJobService.getPrintJobs).toHaveBeenCalledWith(expect.objectContaining({ printer_id: 'p2' })));
    await user.click(screen.getByText('Filters (1)'));
    expect(await screen.findByText('No results match your search')).toBeVisible();
    expect(screen.queryByRole('button', { name: /add first print job/i })).not.toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Add Print Job', exact: true }));

    const printer = screen.getByRole('combobox', { name: 'Printer' });
    expect(printer).toHaveValue('p1');
    await user.selectOptions(printer, 'p2');
    expect(Array.from(printer.options).map((option) => option.value)).not.toContain('p3');
    fireEvent.change(document.querySelector('input[name="print_date"]'), { target: { value: '2026-04-03' } });
    await user.click(screen.getByRole('button', { name: /create print job/i }));

    await waitFor(() => expect(printJobService.createPrintJob).toHaveBeenCalledWith(expect.objectContaining({
      printer_id: 'p2',
    })));
    expect(printerService.getPrinters).toHaveBeenCalledWith('all');
  });

  it('keeps an archived printer representable on its existing job', async () => {
    printerService.getPrinters.mockResolvedValue([
      { id: 'p1', name: 'Current default', is_default: true, is_archived: false },
      { id: 'p-old', name: 'Old machine', is_default: false, is_archived: true },
    ]);
    printJobService.getPrintJobs.mockResolvedValueOnce(apiPage([{
      id: 'j-archived',
      printer_id: 'p-old',
      printer_cost: { printer_name: 'Retired MK3' },
      model_id: null,
      print_date: '2026-04-01T00:00:00.000Z',
      units_printed: 1,
      labor_hours: 0,
      print_time_minutes: 60,
      filaments: [],
    }]));
    modelService.getModels.mockResolvedValue([]);
    filamentService.getFilaments.mockResolvedValue([]);
    printJobService.updatePrintJob.mockResolvedValue({});
    printJobService.getPrintJobs.mockResolvedValueOnce(apiPage([]));

    const user = userEvent.setup();
    render(<PrintJobs />);
    await user.click(await screen.findByRole('button', { name: /edit/i }));

    const printer = screen.getByRole('combobox', { name: 'Printer' });
    expect(printer).toHaveValue('p-old');
    expect(screen.getByRole('option', { name: 'Retired MK3 (archived)' })).toBeInTheDocument();
    expect(screen.queryByRole('option', { name: /old machine/i })).not.toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: /save changes/i }));

    await waitFor(() => expect(printJobService.updatePrintJob).toHaveBeenCalledWith(
      'j-archived',
      expect.objectContaining({ printer_id: 'p-old' }),
    ));
  });

  it('edits an existing print job', async () => {
    printJobService.getPrintJobs.mockResolvedValueOnce(apiPage([
      { id: 'j1', printer_id: 'p1', model_id: null, print_date: '2026-04-01T00:00:00.000Z', units_printed: 2, labor_hours: 1, print_time_minutes: 60, filaments: [] },
    ]));
    modelService.getModels.mockResolvedValue([]);
    filamentService.getFilaments.mockResolvedValue([]);
    printJobService.updatePrintJob.mockResolvedValueOnce({});
    printJobService.getPrintJobs.mockResolvedValueOnce(apiPage([]));

    const user = userEvent.setup();
    render(<PrintJobs />);

    await user.click(await screen.findByRole('button', { name: /edit/i }));

    const spinButtons = screen.getAllByRole('spinbutton');
    await user.clear(spinButtons[0]);
    await user.type(spinButtons[0], '5');
    await user.click(screen.getByRole('button', { name: /save changes/i }));

    await waitFor(() => {
      expect(printJobService.updatePrintJob).toHaveBeenCalledWith(
        'j1',
        expect.objectContaining({ units_printed: 5 }),
      );
    });
  });

  it('deletes a print job after confirmation', async () => {
    printJobService.getPrintJobs.mockResolvedValueOnce(apiPage([
      { id: 'j1', model_id: null, print_date: '2026-04-01T00:00:00.000Z', units_printed: 1, labor_hours: 0, print_time_minutes: null, filaments: [] },
    ]));
    modelService.getModels.mockResolvedValue([]);
    filamentService.getFilaments.mockResolvedValue([]);
    printJobService.deletePrintJob.mockResolvedValueOnce({});
    printJobService.getPrintJobs.mockResolvedValueOnce(apiPage([]));
    vi.spyOn(window, 'confirm').mockReturnValue(true);

    const user = userEvent.setup();
    render(<PrintJobs />);

    await user.click(await screen.findByRole('button', { name: /delete/i }));

    await waitFor(() => {
      expect(printJobService.deletePrintJob).toHaveBeenCalledWith('j1');
    });
  });

  it('shows zero cost when API returns zero estimated_cost', async () => {
    printJobService.getPrintJobs.mockResolvedValueOnce(apiPage([
      {
        id: 'j1',
        model_id: null,
        print_date: '2026-04-01T00:00:00.000Z',
        units_printed: 1,
        labor_hours: 0,
        print_time_minutes: 0,
        estimated_cost: 0,
        filaments: [],
      },
    ]));
    modelService.getModels.mockResolvedValueOnce([]);
    filamentService.getFilaments.mockResolvedValueOnce([]);

    render(<PrintJobs />);

    expect(await screen.findByText('€0.00')).toBeInTheDocument();
  });

  it('shows materials without opening details and searches by filament', async () => {
    const rows = [
      { id: 'j1', model_id: 'm1', model: { state: 'assigned', id: 'm1', name: 'Gear' }, print_date: '2026-04-01', units_printed: 2, labor_hours: 1, print_time_minutes: 95, estimated_cost: 12, filaments: [{ filament_id: 'f1', weight_used: 30, filament: { state: 'available', id: 'f1', name: 'Generic PLA Black' } }] },
      { id: 'j2', model_id: null, model: { state: 'not_assigned', name: 'Model not assigned' }, print_date: '2026-04-02', units_printed: 1, labor_hours: 0, filaments: [] },
    ];
    printJobService.getPrintJobs.mockImplementation(async ({ search }) => apiPage(
      search ? rows.filter((job) => job.filaments.some((row) => row.filament.name.toLowerCase().includes(search))) : rows,
    ));
    modelService.getModels.mockResolvedValue([{ id: 'm1', name: 'Gear' }]);
    filamentService.getFilaments.mockResolvedValue([{ id: 'f1', brand: 'Generic', type: 'PLA', color: 'Black' }]);
    const user = userEvent.setup();
    render(<PrintJobs />);
    expect(await screen.findByText(/Generic PLA Black/)).toBeVisible();
    expect(screen.getByText('1h 35m')).toBeVisible();
    expect(printJobService.getPrintJobCost).not.toHaveBeenCalled();
    await user.type(screen.getByRole('searchbox'), 'black');
    await waitFor(() => expect(screen.getAllByRole('article')).toHaveLength(1));
    await user.type(screen.getByRole('searchbox'), ' nonexistent');
    expect(await screen.findByText('No results match your search')).toBeVisible();
    await user.click(screen.getByRole('button', { name: 'Clear search' }));
    await waitFor(() => expect(screen.getAllByRole('article')).toHaveLength(2));
    await user.selectOptions(screen.getByRole('combobox', { name: 'Sort print jobs' }), 'oldest');
    expect(screen.getAllByRole('article')[0]).toHaveAccessibleName('Gear print job');
  });

  it('finds a model by name in the list filter', async () => {
    printJobService.getPrintJobs.mockResolvedValue(apiPage([]));
    modelService.getModels.mockResolvedValue([
      { id: 'm1', name: 'Alpha bracket' },
      { id: 'm2', name: 'Workshop enclosure' },
    ]);
    filamentService.getFilaments.mockResolvedValue([]);
    const user = userEvent.setup();
    render(<PrintJobs />);
    await screen.findByRole('button', { name: /add first print job/i });
    await user.click(screen.getByText('Filters'));
    const selector = screen.getByRole('combobox', { name: 'Model' });
    await user.type(selector, 'encl');
    await user.click(screen.getByRole('option', { name: 'Workshop enclosure' }));
    await waitFor(() => expect(printJobService.getPrintJobs).toHaveBeenCalledWith(expect.objectContaining({ model_id: 'm2' })));
    await user.click(selector);
    await user.click(screen.getByRole('option', { name: 'All models' }));
    await waitFor(() => expect(printJobService.getPrintJobs).toHaveBeenLastCalledWith(expect.objectContaining({ model_id: '' })));
  });

  it('loads cost details inline, supports retry and retains details when collapsed', async () => {
    printJobService.getPrintJobs.mockResolvedValue(apiPage([{ id: 'j1', print_date: '2026-04-01', units_printed: 1, labor_hours: 0, filaments: [] }]));
    modelService.getModels.mockResolvedValue([]);
    filamentService.getFilaments.mockResolvedValue([]);
    printJobService.getPrintJobCost.mockRejectedValueOnce(new Error('Network failure')).mockResolvedValue({ state: 'calculated',
      filament_cost: 2, electricity_cost: 1, wear_cost: 1, labor_cost: 0, total_cost: 4,
    });
    const user = userEvent.setup();
    render(<PrintJobs />);
    const toggle = await screen.findByRole('button', { name: /cost breakdown/i });
    expect(toggle).toHaveAttribute('aria-expanded', 'false');
    await user.click(toggle);
    expect(await screen.findByRole('alert')).toHaveTextContent('Could not load cost details');
    await user.click(screen.getByRole('button', { name: 'Try again' }));
    expect(await screen.findByText('€4.00')).toBeVisible();
    expect(screen.queryByText(/suggested selling prices/i)).not.toBeInTheDocument();
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    await user.click(toggle);
    expect(screen.getByText('€4.00')).not.toBeVisible();
    await user.click(toggle);
    expect(screen.getByText('€4.00')).toBeVisible();
    expect(printJobService.getPrintJobCost).toHaveBeenCalledTimes(2);
  });
});
