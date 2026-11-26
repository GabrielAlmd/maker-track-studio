import { act, render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { vi, it, expect, beforeEach, afterEach } from 'vitest';
import i18n from '../i18n';
import Inventory from './Inventory';
import { getFinishedGoods, getMaterialPage, getMovements, postCount, setThreshold } from '../services/inventoryService';
vi.mock('../services/inventoryService', async (original) => ({ ...await original(), getFinishedGoods: vi.fn(), getMaterialPage: vi.fn(), getMovements: vi.fn(), postCount: vi.fn(), setThreshold: vi.fn() }));
const item = { id: 'f1', brand: 'Test', type: 'PLA', color: 'Blue', threshold_mg: 1000, balance: { verified: false, on_hand: 2000, available: 2000, revision: 4 } };
const page = (items, current = 1, total = items.length) => ({ items, page: current, page_size: 25, total });
const blue = 'Details for Test PLA — Blue';
const red = { ...item, id: 'f2', color: 'Red' };
const redDetails = 'Details for Test PLA — Red';
const movement = (sequence, reason) => ({ sequence, kind: 'count', quantity: 12500, reason, actor: 'Synthetic operator', created_at: '2026-09-18T09:00:00Z' });

beforeEach(() => { vi.clearAllMocks(); getFinishedGoods.mockResolvedValue([]); getMaterialPage.mockResolvedValue(page([item])); getMovements.mockResolvedValue([]); postCount.mockResolvedValue({}); setThreshold.mockResolvedValue({}); });
it('shows unknown rather than a confirmed balance and validates physical counts', async () => {
 const user = userEvent.setup(); render(<Inventory />);
 await user.click(await screen.findByRole('button', { name: blue }));
 expect(screen.getByText(/Recorded changes: 2 g/)).toBeInTheDocument();
 await user.click(screen.getByRole('button', { name: 'Record count' }));
 expect(postCount).not.toHaveBeenCalled();
 await user.type(screen.getByLabelText('Counted grams'), '12.5');
 await user.type(screen.getByLabelText('Reason'), 'Shelf count');
 await user.type(screen.getByLabelText('Operator'), 'Workshop');
 await user.click(screen.getByRole('button', { name: 'Record count' }));
 await waitFor(() => expect(postCount).toHaveBeenCalledWith(expect.objectContaining({ counted_quantity:12500, expected_revision:4, reason:'Shelf count', kind:'count' })));
});
it('shows a recoverable error and empty state', async () => {
 getMaterialPage.mockRejectedValueOnce(new Error('offline')).mockResolvedValueOnce(page([]));
 const user = userEvent.setup(); render(<Inventory />);
 await user.click(await screen.findByRole('button', { name:'Try again' }));
 expect(await screen.findByText(/No materials yet/)).toBeInTheDocument();
});

it('shows finished pieces and distinguishes unknown stock from available stock', async () => {
  getFinishedGoods.mockResolvedValue([{ id: 'model', name: 'Bracket', balance: { on_hand: 10, reserved: 3, available: 7, verified: false } }]);
  const user = userEvent.setup();
  render(<Inventory />);
  await user.click(screen.getByRole('button', { name: 'Finished goods' }));
  expect(await screen.findByText('Bracket')).toBeInTheDocument();
  expect(screen.getByText('10 recorded')).toBeInTheDocument();
  expect(screen.getByText('Count needed')).toBeInTheDocument();
});

it('reconciles finished stock in whole pieces against the displayed revision', async () => {
  getFinishedGoods.mockResolvedValue([{ id: 'model', name: 'Bracket', balance: { on_hand: 10, reserved: 0, available: 10, revision: 4 } }]);
  const user = userEvent.setup();
  render(<Inventory />);
  await user.click(screen.getByRole('button', { name: 'Finished goods' }));
  await user.click(await screen.findByRole('button', { name: 'Count Bracket' }));
  await user.type(screen.getByLabelText('Counted pieces'), '9');
  await user.type(screen.getByLabelText('Reason'), 'Counted shelf');
  await user.type(screen.getByLabelText('Operator'), 'Workshop');
  await user.click(screen.getByRole('button', { name: 'Record count' }));
  await waitFor(() => expect(postCount).toHaveBeenCalledWith(expect.objectContaining({
    item_type: 'model', item_id: 'model', counted_quantity: 9, expected_revision: 4,
  })));
});

afterEach(async () => { await i18n.changeLanguage('en'); });

it('keeps the current movement page and retries its cursor', async () => {
  getMovements.mockResolvedValueOnce(Array.from({ length: 100 }, (_, i) => movement(i + 1, `Event ${i + 1}`)))
    .mockRejectedValueOnce(new Error('offline')).mockResolvedValueOnce([movement(101, 'Page two count')]);
  const user = userEvent.setup();
  render(<Inventory />);
  await user.click(await screen.findByRole('button', { name: blue }));
  await user.click(await screen.findByRole('button', { name: 'Load more movements' }));
  expect(await screen.findByText('Could not load movement history')).toBeVisible();
  expect(screen.getByText('Event 100')).toBeVisible();
  await user.click(screen.getByRole('button', { name: 'Try again' }));
  expect(await screen.findByText('Page two count')).toBeVisible();
  expect(getMovements.mock.calls.map(([, after]) => after)).toEqual([0, 100, 100]);
});

it('ignores movements arriving after a different material is opened', async () => {
  getMaterialPage.mockResolvedValue(page([item, red]));
  let completeBlue;
  getMovements.mockImplementationOnce(() => new Promise(resolve => { completeBlue = resolve; }))
    .mockResolvedValueOnce([movement(2, 'Red count')]);
  const user = userEvent.setup();
  render(<Inventory />);
  await user.click(await screen.findByRole('button', { name: blue }));
  await user.click(screen.getByRole('button', { name: 'Close' }));
  await user.click(screen.getByRole('button', { name: redDetails }));
  const dialog = screen.getByRole('dialog');
  expect(await within(dialog).findByText('Red count')).toBeVisible();
  await act(() => completeBlue([movement(1, 'Blue count')]));
  expect(getMovements.mock.calls[0][2].aborted).toBe(true);
  expect(within(dialog).queryByText('Blue count')).not.toBeInTheDocument();
});

it('uses bounded list requests and resets the page when a stock filter changes', async () => {
  getMaterialPage.mockResolvedValueOnce(page([item], 1, 30)).mockResolvedValueOnce(page([red], 2, 30))
    .mockResolvedValueOnce(page([item]));
  const user = userEvent.setup();
  render(<Inventory />);
  await screen.findByRole('button', { name: blue });
  expect(getMaterialPage).toHaveBeenLastCalledWith(expect.objectContaining({ page: 1, pageSize: 25 }));
  await user.click(screen.getByRole('button', { name: 'Next page' }));
  await screen.findByRole('button', { name: redDetails });
  await user.click(screen.getByText('Filters', { exact: true }));
  await user.selectOptions(await screen.findByRole('combobox', { name: 'Stock status' }), 'unverified');
  await screen.findByRole('button', { name: blue });
  expect(getMaterialPage).toHaveBeenLastCalledWith(expect.objectContaining({ page: 1, status: 'unverified' }));
});

it('localizes movement quantities and form validation in Portuguese', async () => {
  await i18n.changeLanguage('pt-PT');
  getMovements.mockResolvedValue([movement(1, 'Contagem de teste')]);
  const user = userEvent.setup();
  render(<Inventory />);
  await user.click(await screen.findByRole('button', { name: 'Detalhes de Test PLA — Blue' }));
  expect(await screen.findByRole('heading', { name: 'Histórico de movimentos' })).toBeVisible();
  expect(screen.getByText(/\+12,5\s*g/)).toBeVisible();
  await user.click(screen.getByRole('button', { name: 'Registar contagem' }));
  expect(await screen.findByText(/Introduza um peso não negativo/)).toBeVisible();
});

it('keeps the original count operation and source IDs after an uncertain response', async () => {
  postCount.mockRejectedValueOnce(new Error('offline'));
  const user = userEvent.setup();
  render(<Inventory />);
  await user.click(await screen.findByRole('button', { name: blue }));
  await user.type(screen.getByLabelText('Counted grams'), '12.5');
  await user.type(screen.getByLabelText('Reason'), 'Shelf count');
  await user.type(screen.getByLabelText('Operator'), 'Workshop');
  const record = screen.getByRole('button', { name: 'Record count' });
  await user.click(record);
  await waitFor(() => expect(record).toBeEnabled());
  await user.click(record);
  await waitFor(() => expect(postCount).toHaveBeenCalledTimes(2));
  expect(postCount.mock.calls[0][0]).toEqual(postCount.mock.calls[1][0]);
});

it('validates a threshold in grams and submits integer milligrams', async () => {
  const user = userEvent.setup();
  render(<Inventory />);
  await user.click(await screen.findByRole('button', { name: blue }));
  const threshold = screen.getByLabelText('Warning threshold (g)');
  const save = screen.getByRole('button', { name: 'Save threshold' });
  await user.clear(threshold);
  await user.type(threshold, '1.0001');
  await user.click(save);
  expect(await screen.findByText(/Enter a non-negative weight/)).toBeVisible();
  expect(setThreshold).not.toHaveBeenCalled();
  await user.clear(threshold);
  await user.type(threshold, '12.501');
  await user.click(save);
  await waitFor(() => expect(setThreshold).toHaveBeenCalledWith('f1', 12501));
});
