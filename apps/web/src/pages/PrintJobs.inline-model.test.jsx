import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import PrintJobs from './PrintJobs';
import * as printJobService from '../services/printJobService';
import * as modelService from '../services/modelService';
import * as filamentService from '../services/filamentService';
import * as printerService from '../services/printerService';

vi.mock('../services/printJobService', () => ({
  getPrintJobs: vi.fn(),
  getPrintJobCost: vi.fn(),
  createPrintJob: vi.fn(),
  updatePrintJob: vi.fn(),
  deletePrintJob: vi.fn(),
}));

vi.mock('../services/modelService', () => ({
  getModels: vi.fn(),
  createModel: vi.fn(),
  updateModel: vi.fn(),
  deleteModel: vi.fn(),
}));

vi.mock('../services/filamentService', () => ({ getFilaments: vi.fn() }));
vi.mock('../services/printJobFilamentService', () => ({
  createPrintJobFilament: vi.fn(),
  deletePrintJobFilament: vi.fn(),
}));
vi.mock('../services/printerService', () => ({ getPrinters: vi.fn() }));
vi.mock('react-hot-toast', () => ({
  default: { success: vi.fn(), error: vi.fn() },
}));

describe('PrintJobs inline model creation', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    printJobService.getPrintJobs.mockResolvedValue([]);
    modelService.getModels.mockResolvedValue([]);
    filamentService.getFilaments.mockResolvedValue([]);
    printerService.getPrinters.mockResolvedValue([
      { id: 'p1', name: 'Workshop printer', is_default: true, is_archived: false },
    ]);
  });

  async function openPrintJob() {
    const user = userEvent.setup();
    render(<PrintJobs />);
    await user.click(await screen.findByRole('button', { name: /add first print job/i }));
    return user;
  }

  async function fillModel(user, name = 'Bracket') {
    await user.type(screen.getByPlaceholderText(/e.g. Benchy/i), name);
    await user.type(screen.getByPlaceholderText(/e.g. 15.5/i), '12.5');
    await user.type(screen.getByPlaceholderText(/e.g. 120/i), '90');
  }

  it('creates, refreshes, automatically selects the model, and applies its defaults', async () => {
    const createdModel = {
      id: 'm-new', name: 'Bracket', material_weight: 12.5, print_time: 90,
      waste_material: 0, is_multicolor: false,
    };
    modelService.createModel.mockResolvedValue(createdModel);
    const user = await openPrintJob();

    fireEvent.change(document.querySelector('input[name="units_printed"]'), { target: { value: '2' } });
    await user.click(screen.getByRole('button', { name: /create new model/i }));
    await fillModel(user);
    await user.click(screen.getByRole('button', { name: /create model/i }));

    expect(await screen.findByRole('heading', { name: 'Add New Print Job' })).toBeInTheDocument();
    expect(screen.getByRole('combobox', { name: 'Model' })).toHaveValue('Bracket');
    expect(screen.getByPlaceholderText(/e.g. 90/i)).toHaveValue(180);
    expect(screen.getByPlaceholderText('g')).toHaveValue(25);
    expect(modelService.createModel).toHaveBeenCalledWith({
      name: 'Bracket', image_url: '', material_weight: 12.5, print_time: 90,
      waste_material: 0, is_multicolor: false,
    });
    await waitFor(() => expect(modelService.getModels).toHaveBeenCalledTimes(2));
  });

  it('cancels model creation without losing print-job input', async () => {
    const user = await openPrintJob();
    fireEvent.change(document.querySelector('input[name="print_date"]'), { target: { value: '2026-09-24' } });
    fireEvent.change(document.querySelector('input[name="labor_hours"]'), { target: { value: '1.5' } });

    await user.click(screen.getByRole('button', { name: /create new model/i }));
    await user.click(screen.getByRole('button', { name: 'Cancel' }));

    expect(await screen.findByRole('heading', { name: 'Add New Print Job' })).toBeInTheDocument();
    expect(document.querySelector('input[name="print_date"]')).toHaveValue('2026-09-24');
    expect(document.querySelector('input[name="labor_hours"]')).toHaveValue(1.5);
    expect(modelService.createModel).not.toHaveBeenCalled();
  });

  it('shows shared model validation failures without closing either workflow', async () => {
    const user = await openPrintJob();
    await user.click(screen.getByRole('button', { name: /create new model/i }));
    await user.click(screen.getByRole('button', { name: /create model/i }));

    expect(await screen.findByText('Name is required')).toBeInTheDocument();
    expect(screen.getByText('Material weight must be a number')).toBeInTheDocument();
    expect(screen.getByText('Print time must be a number')).toBeInTheDocument();
    expect(screen.getByRole('heading', { name: 'Add New Model' })).toBeInTheDocument();
    expect(modelService.createModel).not.toHaveBeenCalled();
  });

  it('shows an API failure, keeps model input, and preserves the print job for return', async () => {
    modelService.createModel.mockRejectedValue(new Error('Model service unavailable'));
    const user = await openPrintJob();
    fireEvent.change(document.querySelector('input[name="print_date"]'), { target: { value: '2026-09-24' } });
    await user.click(screen.getByRole('button', { name: /create new model/i }));
    await fillModel(user, 'Retry bracket');
    await user.click(screen.getByRole('button', { name: /create model/i }));

    expect(await screen.findByRole('alert')).toHaveTextContent('Model service unavailable');
    expect(screen.getByPlaceholderText(/e.g. Benchy/i)).toHaveValue('Retry bracket');
    expect(screen.getByRole('heading', { name: 'Add New Model' })).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Cancel' }));
    expect(document.querySelector('input[name="print_date"]')).toHaveValue('2026-09-24');
  });
});
