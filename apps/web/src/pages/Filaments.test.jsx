vi.mock('../services/filamentTypeService', () => ({
  getFilamentTypes: vi.fn().mockResolvedValue(['PLA', 'PETG', 'Custom']),
  updateFilamentTypes: vi.fn().mockResolvedValue([]),
}));

import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import Filaments from './Filaments';
import * as filamentService from '../services/filamentService';

vi.mock('../services/filamentService', () => ({
  getFilaments: vi.fn(),
  getFilamentBrands: vi.fn().mockResolvedValue(['Bambu Lab', 'Prusament']),
  createFilament: vi.fn(),
  updateFilament: vi.fn(),
  deleteFilament: vi.fn(),
}));

vi.mock('react-hot-toast', () => ({
  default: {
    success: vi.fn(),
    error: vi.fn(),
  },
}));

describe('Filaments page', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('loads filaments and renders table rows', async () => {
    filamentService.getFilaments.mockResolvedValueOnce([
      {
        id: 'f1',
        brand: 'Prusa',
        type: 'PLA',
        color: 'Orange',
        avg_price: 29.99,
        personal_score: 5,
        times_used: 3,
      },
    ]);

    render(<Filaments />);

    expect(await screen.findByText('Prusa')).toBeInTheDocument();
    expect(screen.getByText('Orange')).toBeInTheDocument();
  });

  it('shows no purchase data instead of a made-up average price', async () => {
    filamentService.getFilaments.mockResolvedValueOnce([
      {
        id: 'f1', brand: 'Prusa', type: 'PLA', color: 'Orange', avg_price: 0,
        personal_score: 5, times_used: 3, purchases: [],
      },
    ]);

    render(<Filaments />);

    expect(await screen.findByText('No purchase data')).toBeInTheDocument();
    expect(screen.queryByText('€0.00')).not.toBeInTheDocument();
  });

  it('shows validation errors when submitting empty filament form', async () => {
    filamentService.getFilaments.mockResolvedValueOnce([]);
    const user = userEvent.setup();

    render(<Filaments />);

    await user.click(await screen.findByRole('button', { name: /add your first filament/i }));
    await user.click(screen.getByRole('button', { name: /create filament/i }));

    expect(await screen.findByText('Brand is required')).toBeInTheDocument();
    expect(screen.getByText('Type is required')).toBeInTheDocument();
    expect(screen.getByText('Color is required')).toBeInTheDocument();
  });

  it('submits valid create filament form', async () => {
    filamentService.getFilaments.mockResolvedValue([]);
    filamentService.createFilament.mockResolvedValueOnce({ id: 'new-id' });
    filamentService.getFilaments.mockResolvedValueOnce([]);

    const user = userEvent.setup();
    render(<Filaments />);

    await user.click(await screen.findByRole('button', { name: /add your first filament/i }));

    await user.type(screen.getByLabelText('Brand'), 'New Brand');
    await user.selectOptions(screen.getByLabelText('Type'), 'PLA');
    await user.type(screen.getByPlaceholderText(/e.g. Black/i), 'Green');
    await user.type(screen.getByPlaceholderText(/e.g. 22.99/i), '24.5');

    const spinButtons = screen.getAllByRole('spinbutton');
    const personalScoreInput = spinButtons[1];
    await user.clear(personalScoreInput);
    await user.type(personalScoreInput, '4');

    await user.click(screen.getByRole('button', { name: /create filament/i }));

    await waitFor(() => {
      expect(filamentService.createFilament).toHaveBeenCalledWith({
        brand: 'New Brand',
        type: 'PLA',
        color: 'Green',
        avg_price: 24.5,
        personal_score: 4,
      });
    });
  });

  it('edits an existing filament', async () => {
    filamentService.getFilaments.mockResolvedValueOnce([
      { id: 'f1', brand: 'Prusa', type: 'PLA', color: 'Orange', avg_price: 29.99, personal_score: 5, times_used: 3 },
    ]);
    filamentService.updateFilament.mockResolvedValueOnce({});
    filamentService.getFilaments.mockResolvedValueOnce([]);

    const user = userEvent.setup();
    render(<Filaments />);

    await user.click(await screen.findByRole('button', { name: /edit/i }));

    const brandInput = screen.getByLabelText('Brand');
    expect(brandInput).toHaveValue('Prusa');
    await user.clear(brandInput);
    await user.type(brandInput, 'bambu');
    await user.click(screen.getByRole('option', { name: 'Bambu Lab' }));
    await user.click(screen.getByRole('button', { name: /save changes/i }));

    await waitFor(() => {
      expect(filamentService.updateFilament).toHaveBeenCalledWith(
        'f1',
        expect.objectContaining({ brand: 'Bambu Lab', type: 'PLA', color: 'Orange' }),
      );
    });
  });

  it('deletes a filament after confirmation', async () => {
    filamentService.getFilaments.mockResolvedValueOnce([
      { id: 'f1', brand: 'Prusa', type: 'PLA', color: 'Orange', avg_price: 29.99, personal_score: 5, times_used: 3 },
    ]);
    filamentService.deleteFilament.mockResolvedValueOnce({});
    filamentService.getFilaments.mockResolvedValueOnce([]);
    vi.spyOn(window, 'confirm').mockReturnValue(true);

    const user = userEvent.setup();
    render(<Filaments />);

    await user.click(await screen.findByRole('button', { name: /delete/i }));

    await waitFor(() => {
      expect(filamentService.deleteFilament).toHaveBeenCalledWith('f1');
    });
  });
});
