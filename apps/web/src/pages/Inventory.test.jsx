import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { vi, it, expect, beforeEach } from 'vitest';
import Inventory from './Inventory';
import { getFinishedGoods, getMaterials, getMovements, postCount } from '../services/inventoryService';
vi.mock('../services/inventoryService', async (original) => ({ ...await original(), getFinishedGoods: vi.fn(), getMaterials: vi.fn(), getMovements: vi.fn(), postCount: vi.fn(), setThreshold: vi.fn() }));
const item = { id: 'f1', brand: 'Test', type: 'PLA', color: 'Blue', threshold_mg: 1000, balance: { verified: false, on_hand: 2000, available: 2000, revision: 4 } };
beforeEach(() => { vi.clearAllMocks(); getFinishedGoods.mockResolvedValue([]); getMaterials.mockResolvedValue([item]); getMovements.mockResolvedValue([]); });
it('shows unknown rather than a confirmed balance and validates physical counts', async () => {
 const user = userEvent.setup(); render(<Inventory />);
 expect(await screen.findByText(/Recorded changes: 2 g/)).toBeInTheDocument();
 await user.click(screen.getByRole('button', { name: /Count and audit/ }));
 await user.click(screen.getByRole('button', { name: 'Record count' }));
 expect(postCount).not.toHaveBeenCalled();
 await user.type(screen.getByLabelText('Counted grams'), '12.5');
 await user.type(screen.getByLabelText('Reason'), 'Shelf count');
 await user.type(screen.getByLabelText('Operator'), 'Workshop');
 await user.click(screen.getByRole('button', { name: 'Record count' }));
 await waitFor(() => expect(postCount).toHaveBeenCalledWith(expect.objectContaining({ counted_quantity:12500, expected_revision:4, reason:'Shelf count', kind:'count' })));
});
it('shows a recoverable error and empty state', async () => {
 getMaterials.mockRejectedValueOnce(new Error('offline')).mockResolvedValueOnce([]);
 const user = userEvent.setup(); render(<Inventory />);
 await user.click(await screen.findByRole('button', { name:'Retry' }));
 expect(await screen.findByText(/No materials yet/)).toBeInTheDocument();
});

it('shows finished pieces and distinguishes unknown stock from available stock', async () => {
  getFinishedGoods.mockResolvedValue([{ id: 'model', name: 'Bracket', balance: { on_hand: 10, reserved: 3, available: 7, verified: false } }]);
  render(<Inventory />);
  expect(await screen.findByText('Bracket')).toBeInTheDocument();
  expect(screen.getByText('10 recorded')).toBeInTheDocument();
  expect(screen.getByText('Count needed')).toBeInTheDocument();
});

it('reconciles finished stock in whole pieces against the displayed revision', async () => {
  getFinishedGoods.mockResolvedValue([{ id: 'model', name: 'Bracket', balance: { on_hand: 10, reserved: 0, available: 10, revision: 4 } }]);
  const user = userEvent.setup();
  render(<Inventory />);
  await user.click(await screen.findByRole('button', { name: 'Count Bracket' }));
  await user.type(screen.getByLabelText('Counted pieces'), '9');
  await user.type(screen.getByLabelText('Reason'), 'Counted shelf');
  await user.type(screen.getByLabelText('Operator'), 'Workshop');
  await user.click(screen.getByRole('button', { name: 'Record count' }));
  await waitFor(() => expect(postCount).toHaveBeenCalledWith(expect.objectContaining({
    item_type: 'model', item_id: 'model', counted_quantity: 9, expected_revision: 4,
  })));
});
