import { getFilamentTypes, updateFilamentTypes } from '../services/filamentTypeService';
vi.mock('../services/filamentTypeService', () => ({
  getFilamentTypes: vi.fn().mockResolvedValue(['PLA', 'PETG', 'Custom']),
  updateFilamentTypes: vi.fn().mockResolvedValue([]),
}));

import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import Purchases from './Purchases';
import * as filamentPurchaseService from '../services/filamentPurchaseService';
import * as filamentService from '../services/filamentService';

vi.mock('../services/filamentPurchaseService', () => ({
  getFilamentPurchases: vi.fn(),
  createFilamentPurchase: vi.fn(),
  createFilamentPurchaseBatch: vi.fn(),
  updateFilamentPurchase: vi.fn(),
  deleteFilamentPurchase: vi.fn(),
}));

vi.mock('../services/filamentService', () => ({
  getFilaments: vi.fn(),
  getFilamentBrands: vi.fn().mockResolvedValue(['Bambu', 'Prusa']),
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

describe('Purchases page', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('renders purchases with resolved filament labels', async () => {
    filamentPurchaseService.getFilamentPurchases.mockResolvedValueOnce([
      {
        id: 'p1',
        filament_id: 'f1',
        price: 24.99,
        amount: 1000,
        purchase_date: '2026-04-01T00:00:00.000Z',
      },
    ]);
    filamentService.getFilaments.mockResolvedValueOnce([
      { id: 'f1', brand: 'Prusa', type: 'PLA', color: 'Orange' },
    ]);

    render(<Purchases />);

    expect(await screen.findByText('Prusa')).toBeInTheDocument();
    expect(screen.getByText(/PLA — Orange/i)).toBeInTheDocument();
    expect(screen.getByText('€24.99')).toBeInTheDocument();
  });

  it('shows validation errors when submitting empty purchase form', async () => {
    filamentPurchaseService.getFilamentPurchases.mockResolvedValueOnce([]);
    filamentService.getFilaments.mockResolvedValueOnce([]);
    const user = userEvent.setup();

    render(<Purchases />);

    await user.click(await screen.findByRole('button', { name: /record first purchase/i }));
    await user.click(screen.getAllByRole('button', { name: /record purchase/i })[1]);

    expect(await screen.findByText('Filament is required')).toBeInTheDocument();
    expect(screen.getByText('Price must be a number')).toBeInTheDocument();
    expect(screen.getByText('Amount must be a number')).toBeInTheDocument();
  });

  it('submits valid purchase payload', async () => {
    filamentPurchaseService.getFilamentPurchases.mockResolvedValue([]);
    filamentService.getFilaments.mockResolvedValue([
      { id: 'f1', brand: 'Bambu', type: 'PETG', color: 'Black' },
    ]);
    filamentPurchaseService.createFilamentPurchaseBatch.mockResolvedValueOnce({ id: 'purchase-1' });
    filamentPurchaseService.getFilamentPurchases.mockResolvedValueOnce([]);

    const user = userEvent.setup();
    render(<Purchases />);

    await user.click(await screen.findByRole('button', { name: /record first purchase/i }));

    const filament = screen.getByRole('combobox', { name: 'Filament' });
    await user.type(filament, 'black');
    await user.click(screen.getByRole('option', { name: /Black$/ }));
    await user.type(screen.getByPlaceholderText(/e.g. 24.99/i), '21.5');
    await user.type(screen.getByPlaceholderText(/^e.g. 1$/i), '1');
    const purchaseDateInput = document.querySelector('input[name="purchase_date"]');
    if (!purchaseDateInput) {
      throw new Error('purchase_date input not found');
    }
    await user.type(purchaseDateInput, '2026-04-01');
    await user.click(screen.getAllByRole('button', { name: /record purchase/i })[1]);

    await waitFor(() => {
      expect(filamentPurchaseService.createFilamentPurchaseBatch).toHaveBeenCalledWith({
        items: [{ filament_id: 'f1', price: 21.5, amount: 1 }],
        purchase_date: '2026-04-01T00:00:00.000Z',
      });
    });
  });

  it('records existing and new filaments together and preserves the form after failure', async () => {
    filamentPurchaseService.getFilamentPurchases.mockResolvedValue([]);
    filamentService.getFilaments.mockResolvedValue([
      { id: 'f1', brand: 'Bambu', type: 'PLA', color: 'Black' },
    ]);
    filamentPurchaseService.createFilamentPurchaseBatch.mockRejectedValueOnce(new Error('Failed')).mockResolvedValueOnce([]);
    const user = userEvent.setup();
    render(<Purchases />);
    await user.click(await screen.findByRole('button', { name: /record first purchase/i }));
    await user.type(screen.getByLabelText('Purchase Date'), '2026-04-01');
    const filament = screen.getByRole('combobox', { name: 'Filament' });
    await user.type(filament, 'black');
    await user.click(screen.getByRole('option', { name: /Black$/ }));
    await user.type(screen.getByLabelText('Price (€)'), '20');
    await user.type(screen.getByLabelText('Amount (kg)'), '1');
    await user.click(screen.getByText('Add another filament'));
    const createActions = screen.getAllByRole('button', { name: /create new filament/i });
    expect(createActions).toHaveLength(2);
    expect(screen.queryByRole('option', { name: /new filament/i })).not.toBeInTheDocument();
    await user.click(createActions[1]);
    expect(screen.getAllByLabelText('Filament')[1]).toHaveValue('');
    await user.type(screen.getByRole('combobox', { name: 'Brand' }), 'bamb');
    await user.click(screen.getByRole('option', { name: 'Bambu' }));
    await user.selectOptions(screen.getByLabelText('Type'), 'PETG');
    await user.type(screen.getByLabelText('Color'), 'Blue');
    await user.type(screen.getAllByLabelText('Price (€)')[1], '30');
    await user.type(screen.getAllByLabelText('Amount (kg)')[1], '2');
    expect(screen.getByText('Total: €50.00')).toBeInTheDocument();
    await user.click(screen.getAllByRole('button', { name: /^record purchase$/i })[1]);
    await waitFor(() => expect(filamentPurchaseService.createFilamentPurchaseBatch).toHaveBeenCalledWith({
      purchase_date: '2026-04-01T00:00:00.000Z',
      items: [
        { filament_id: 'f1', price: 20, amount: 1 },
        { new_filament: { brand: 'Bambu', type: 'PETG', color: 'Blue' }, price: 30, amount: 2 },
      ],
    }));
    expect(screen.getByRole('combobox', { name: 'Brand' })).toHaveValue('Bambu');
    await user.click(screen.getAllByRole('button', { name: /^record purchase$/i })[1]);
    await waitFor(() => expect(screen.queryByText('Record New Purchase')).not.toBeInTheDocument());
    expect(filamentService.getFilaments).toHaveBeenCalledTimes(2);
  });

  it('manages types during a purchase without losing entered values', async () => {
    filamentPurchaseService.getFilamentPurchases.mockResolvedValue([]);
    filamentService.getFilaments.mockResolvedValue([]);
    const user = userEvent.setup();
    render(<Purchases />);
    await user.click(await screen.findByRole('button', { name: /record first purchase/i }));
    await user.type(screen.getByLabelText('Purchase Date'), '2026-04-01');
    await user.click(screen.getByRole('button', { name: /create new filament/i }));
    await user.type(screen.getByLabelText('Brand'), 'My brand');
    expect(screen.getByRole('option', { name: 'Custom' })).toBeInTheDocument();
    await user.click(screen.getByText('Manage types'));
    await user.click(screen.getByText('Add type'));
    await user.type(screen.getByLabelText('Type 4'), 'PLA Silk');
    getFilamentTypes.mockResolvedValueOnce(['PLA', 'PETG', 'Custom', 'PLA Silk']);
    await user.click(screen.getByText('Save types'));
    expect(await screen.findByRole('option', { name: 'PLA Silk' })).toBeInTheDocument();
    expect(updateFilamentTypes).toHaveBeenCalledWith(['PLA', 'PETG', 'Custom', 'PLA Silk']);
    expect(screen.getByLabelText('Brand')).toHaveValue('My brand');
    expect(screen.getByLabelText('Purchase Date')).toHaveValue('2026-04-01');
    await user.selectOptions(screen.getByLabelText('Type'), 'PLA Silk');
    expect(screen.getByLabelText('Type')).toHaveValue('PLA Silk');
  });

  it('removes an unused line before validation', async () => {
    filamentPurchaseService.getFilamentPurchases.mockResolvedValue([]);
    filamentService.getFilaments.mockResolvedValue([]);
    const user = userEvent.setup();
    render(<Purchases />);
    await user.click(await screen.findByRole('button', { name: /record first purchase/i }));
    await user.click(screen.getByText('Add another filament'));
    await user.click(screen.getByRole('button', { name: 'Remove filament 2' }));
    expect(screen.getAllByRole('combobox')).toHaveLength(1);
  });

  it('edits an existing purchase', async () => {
    filamentPurchaseService.getFilamentPurchases.mockResolvedValueOnce([
      { id: 'p1', filament_id: 'f1', price: 24.99, amount: 1000, purchase_date: '2026-04-01T00:00:00.000Z' },
    ]);
    filamentService.getFilaments.mockResolvedValue([
      { id: 'f1', brand: 'Prusa', type: 'PLA', color: 'Orange' },
    ]);
    filamentPurchaseService.updateFilamentPurchase.mockResolvedValueOnce({});
    filamentPurchaseService.getFilamentPurchases.mockResolvedValueOnce([]);

    const user = userEvent.setup();
    render(<Purchases />);

    await user.click(await screen.findByRole('button', { name: /edit/i }));

    const priceInput = screen.getByPlaceholderText(/e.g. 24.99/i);
    await user.clear(priceInput);
    await user.type(priceInput, '30.00');
    await user.click(screen.getByRole('button', { name: /save changes/i }));

    await waitFor(() => {
      expect(filamentPurchaseService.updateFilamentPurchase).toHaveBeenCalledWith(
        'p1',
        expect.objectContaining({ filament_id: 'f1', price: 30 }),
      );
    });
  });

  it('deletes a purchase after confirmation', async () => {
    filamentPurchaseService.getFilamentPurchases.mockResolvedValueOnce([
      { id: 'p1', filament_id: 'f1', price: 24.99, amount: 1000, purchase_date: '2026-04-01T00:00:00.000Z' },
    ]);
    filamentService.getFilaments.mockResolvedValue([
      { id: 'f1', brand: 'Prusa', type: 'PLA', color: 'Orange' },
    ]);
    filamentPurchaseService.deleteFilamentPurchase.mockResolvedValueOnce({});
    filamentPurchaseService.getFilamentPurchases.mockResolvedValueOnce([]);
    vi.spyOn(window, 'confirm').mockReturnValue(true);

    const user = userEvent.setup();
    render(<Purchases />);

    await user.click(await screen.findByRole('button', { name: /delete/i }));

    await waitFor(() => {
      expect(filamentPurchaseService.deleteFilamentPurchase).toHaveBeenCalledWith('p1');
    });
  });
});
