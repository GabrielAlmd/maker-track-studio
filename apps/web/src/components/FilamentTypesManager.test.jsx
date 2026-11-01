import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import FilamentTypesManager from './FilamentTypesManager';
import { updateFilamentTypes } from '../services/filamentTypeService';

vi.mock('../services/filamentTypeService', () => ({ updateFilamentTypes: vi.fn() }));
vi.mock('react-hot-toast', () => ({ default: { success: vi.fn() } }));

beforeEach(() => vi.resetAllMocks());

it('adds, edits and removes types before saving', async () => {
  const user = userEvent.setup();
  const onSaved = vi.fn();
  updateFilamentTypes.mockResolvedValue([]);
  render(<FilamentTypesManager types={['PLA', 'ABS']} onClose={vi.fn()} onSaved={onSaved} />);
  await user.clear(screen.getByLabelText('Type 1'));
  await user.type(screen.getByLabelText('Type 1'), 'PLA Silk');
  await user.click(screen.getByLabelText('Remove type 2'));
  await user.click(screen.getByText('Add type'));
  await user.type(screen.getByLabelText('Type 2'), ' PETG-CF ');
  await user.click(screen.getByText('Save types'));
  await waitFor(() => expect(updateFilamentTypes).toHaveBeenCalledWith(['PLA Silk', 'PETG-CF']));
  expect(onSaved).toHaveBeenCalled();
});

it('rejects duplicate types and retains the draft after a save failure', async () => {
  const user = userEvent.setup();
  const onSaved = vi.fn();
  render(<FilamentTypesManager types={['PLA', 'ABS']} onClose={vi.fn()} onSaved={onSaved} />);
  await user.clear(screen.getByLabelText('Type 2'));
  await user.type(screen.getByLabelText('Type 2'), 'pla');
  await user.click(screen.getByText('Save types'));
  expect(await screen.findByText('Type names must be unique')).toBeInTheDocument();
  expect(updateFilamentTypes).not.toHaveBeenCalled();
  await user.clear(screen.getByLabelText('Type 2'));
  await user.type(screen.getByLabelText('Type 2'), 'PETG');
  updateFilamentTypes.mockRejectedValueOnce(new Error('Failed'));
  await user.click(screen.getByText('Save types'));
  await waitFor(() => expect(updateFilamentTypes).toHaveBeenCalledWith(['PLA', 'PETG']));
  expect(screen.getByLabelText('Type 2')).toHaveValue('PETG');
  expect(onSaved).not.toHaveBeenCalled();
});
