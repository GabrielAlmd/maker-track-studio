import { apiPage } from '../test/apiFixtures';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import Models from './Models';
import * as modelService from '../services/modelService';
import toast from 'react-hot-toast';

vi.mock('../services/modelService', () => ({
  getModels: vi.fn(),
  createModel: vi.fn(),
  updateModel: vi.fn(),
  deleteModel: vi.fn(),
}));

vi.mock('react-hot-toast', () => ({
  default: {
    success: vi.fn(),
    error: vi.fn(),
  },
}));

describe('Models page', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('requests the next server page and replaces the visible models (A05/B04)', async () => {
    modelService.getModels
      .mockResolvedValueOnce(apiPage(Array.from({ length: 25 }, (_, index) => ({
        id: `m${index}`, name: `Model ${index}`, material_weight: 10, print_time: 60, waste_material: 0,
      })), { total: 26 }))
      .mockResolvedValueOnce(apiPage([{ id: 'm-last', name: 'Last model', material_weight: 20, print_time: 90, waste_material: 0 }], { total: 26, page: 2 }));
    const user = userEvent.setup();
    render(<Models />);
    await screen.findByText('Model 0');
    await user.click(screen.getByRole('button', { name: 'Next page' }));
    expect(await screen.findByText('Last model')).toBeInTheDocument();
    expect(screen.queryByText('Model 0')).not.toBeInTheDocument();
    expect(modelService.getModels).toHaveBeenLastCalledWith(expect.objectContaining({ page: 2, pageSize: 25 }));
  });

  it('loads models and renders table rows', async () => {
    modelService.getModels.mockResolvedValueOnce(apiPage([
      {
        id: 'm1',
        name: 'Benchy',
        material_weight: 15.5,
        print_time: 90,
        waste_material: 1.2,
        is_multicolor: false,
        print_jobs: [],
      },
    ]));

    render(<Models />);

    expect(await screen.findByText('Benchy')).toBeInTheDocument();
    expect(screen.getByText('15.5 g')).toBeInTheDocument();
    expect(screen.getByText('1h 30m')).toBeInTheDocument();
  });

  it('shows validation errors when submitting empty model form', async () => {
    modelService.getModels.mockResolvedValueOnce(apiPage([]));
    const user = userEvent.setup();

    render(<Models />);

    await user.click(await screen.findByRole('button', { name: /add your first model/i }));
    await user.click(screen.getByRole('button', { name: /create model/i }));

    expect(await screen.findByText('Name is required')).toBeInTheDocument();
    expect(screen.getByText('Material weight must be a number')).toBeInTheDocument();
    expect(screen.getByText('Print time must be a number')).toBeInTheDocument();
  });

  it('submits valid create model form', async () => {
    modelService.getModels.mockResolvedValue(apiPage([]));
    modelService.createModel.mockResolvedValueOnce({ id: 'new-model' });
    modelService.getModels.mockResolvedValueOnce(apiPage([]));

    const user = userEvent.setup();
    render(<Models />);

    await user.click(await screen.findByRole('button', { name: /add your first model/i }));

    await user.type(screen.getByPlaceholderText(/e.g. Benchy/i), 'Bracket');
    await user.type(screen.getByPlaceholderText(/https:\/\/\.\.\./i), 'https://example.com/model.png');
    await user.type(screen.getByPlaceholderText(/e.g. 15.5/i), '12.34');
    await user.type(screen.getByPlaceholderText(/e.g. 120/i), '150');
    await user.clear(screen.getByPlaceholderText(/e.g. 2.0/i));
    await user.type(screen.getByPlaceholderText(/e.g. 2.0/i), '1.5');
    await user.click(screen.getByLabelText(/Multicolor print/i));
    await user.click(screen.getByRole('button', { name: /create model/i }));

    await waitFor(() => {
      expect(modelService.createModel).toHaveBeenCalledWith({
        name: 'Bracket',
        image_url: 'https://example.com/model.png',
        material_weight: 12.34,
        print_time: 150,
        waste_material: 1.5,
        is_multicolor: true,
      });
    });
  });

  it('edits an existing model', async () => {
    modelService.getModels.mockResolvedValueOnce(apiPage([
      { id: 'm1', name: 'Benchy', material_weight: 15.5, print_time: 90, waste_material: 1.2, is_multicolor: false, print_jobs: [] },
    ]));
    modelService.updateModel.mockResolvedValueOnce({});
    modelService.getModels.mockResolvedValueOnce(apiPage([]));

    const user = userEvent.setup();
    render(<Models />);

    await user.click(await screen.findByRole('button', { name: /edit/i }));

    const nameInput = screen.getByPlaceholderText(/e.g. Benchy/i);
    await user.clear(nameInput);
    await user.type(nameInput, 'BenchyV2');
    await user.click(screen.getByRole('button', { name: /save changes/i }));

    await waitFor(() => {
      expect(modelService.updateModel).toHaveBeenCalledWith(
        'm1',
        expect.objectContaining({ name: 'BenchyV2' }),
      );
    });
  });

  it('deletes a model after confirmation', async () => {
    modelService.getModels.mockResolvedValueOnce(apiPage([
      { id: 'm1', name: 'Benchy', material_weight: 15.5, print_time: 90, waste_material: 1.2, is_multicolor: false, print_jobs: [] },
    ]));
    modelService.deleteModel.mockResolvedValueOnce({});
    modelService.getModels.mockResolvedValueOnce(apiPage([]));
    vi.spyOn(window, 'confirm').mockReturnValue(true);

    const user = userEvent.setup();
    render(<Models />);

    await user.click(await screen.findByRole('button', { name: /delete/i }));

    await waitFor(() => {
      expect(modelService.deleteModel).toHaveBeenCalledWith('m1');
    });
  });

  it('selects visible rows and bulk deletes them after count-aware confirmation', async () => {
    modelService.getModels
      .mockResolvedValueOnce(apiPage([
        { id: 'm1', name: 'Benchy', material_weight: 15, print_time: 90, waste_material: 1, is_multicolor: false },
        { id: 'm2', name: 'Bracket', material_weight: 20, print_time: 60, waste_material: 2, is_multicolor: false },
      ]))
      .mockResolvedValueOnce(apiPage([]));
    modelService.deleteModel.mockResolvedValue({});
    const confirm = vi.spyOn(window, 'confirm').mockReturnValue(true);
    const user = userEvent.setup();
    render(<Models />);

    await user.click(await screen.findByRole('checkbox', { name: 'Select all visible records' }));
    expect(screen.getByText('2 records selected')).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Delete selected' }));

    expect(confirm).toHaveBeenCalledWith('Delete 2 selected records? This cannot be undone.');
    await waitFor(() => expect(modelService.deleteModel.mock.calls).toEqual([['m1'], ['m2']]));
    await waitFor(() => expect(screen.queryByText('2 records selected')).not.toBeInTheDocument());
    expect(toast.success).toHaveBeenCalledWith('2 records deleted');
  });

  it('cancels bulk deletion without losing the selection', async () => {
    modelService.getModels.mockResolvedValueOnce(apiPage([
      { id: 'm1', name: 'Benchy', material_weight: 15, print_time: 90, waste_material: 1, is_multicolor: false },
    ]));
    vi.spyOn(window, 'confirm').mockReturnValue(false);
    const user = userEvent.setup();
    render(<Models />);

    await user.click(await screen.findByRole('checkbox', { name: 'Select Benchy' }));
    await user.click(screen.getByRole('button', { name: 'Delete selected' }));

    expect(modelService.deleteModel).not.toHaveBeenCalled();
    expect(screen.getByText('1 record selected')).toBeInTheDocument();
  });

  it('keeps protected rows selected and identifies bulk deletion failures', async () => {
    modelService.getModels.mockResolvedValueOnce(apiPage([
      { id: 'm1', name: 'Referenced model', material_weight: 15, print_time: 90, waste_material: 1, is_multicolor: false },
    ]));
    modelService.deleteModel.mockRejectedValueOnce({ response: { data: { error: 'Model is used by print jobs' } } });
    vi.spyOn(window, 'confirm').mockReturnValue(true);
    const user = userEvent.setup();
    render(<Models />);

    await user.click(await screen.findByRole('checkbox', { name: 'Select Referenced model' }));
    await user.click(screen.getByRole('button', { name: 'Delete selected' }));

    expect(await screen.findByRole('alert')).toHaveTextContent('Referenced model: Model is used by print jobs');
    expect(screen.getByText('1 record selected')).toBeInTheDocument();
    expect(modelService.getModels).toHaveBeenCalledOnce();
  });
});
