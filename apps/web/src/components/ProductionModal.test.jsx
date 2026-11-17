import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import ProductionModal from './ProductionModal';
import api from '../services/api';

vi.mock('../services/api', () => ({ default: { request: vi.fn(), get: vi.fn() } }));
const printers = [{ id: 'printer-1', name: 'Workshop', is_default: true }];

beforeEach(() => vi.clearAllMocks());

it('saves a planned run and reuses its operation ID after a lost response', async () => {
  const saved = vi.fn();
  api.request.mockRejectedValueOnce(new Error('network')).mockResolvedValueOnce({});
  render(<ProductionModal printers={printers} onClose={() => {}} onSaved={saved} />);
  fireEvent.click(screen.getByRole('button', { name: 'Save production' }));
  await screen.findByRole('alert');
  expect(api.request).toHaveBeenCalledTimes(1);
  expect(api.request.mock.calls[0][0].data.runs[0].status).toBe('planned');
  fireEvent.click(screen.getByRole('button', { name: 'Save production' }));
  await waitFor(() => expect(saved).toHaveBeenCalledOnce());
  expect(api.request.mock.calls[1][0].data.operation_id).toBe(api.request.mock.calls[0][0].data.operation_id);
});

it('blocks fractional unit counts before sending a request', async () => {
  render(<ProductionModal printers={printers} onClose={() => {}} onSaved={() => {}} />);
  fireEvent.change(screen.getByLabelText('planned'), { target: { value: '1.5' } });
  fireEvent.submit(screen.getByRole('button', { name: 'Save production' }).closest('form'));
  await screen.findByRole('alert');
  expect(api.request).not.toHaveBeenCalled();
});
