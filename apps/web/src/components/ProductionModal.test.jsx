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

it('appends imported estimates, preserves manual runs and requires review before atomic save', async () => {
  api.request.mockResolvedValue({});
  render(<ProductionModal printers={printers} filaments={[{ id: 'f1', brand: 'Test', type: 'PLA', color: 'Blue' }]} onClose={() => {}} onSaved={() => {}} />);
  fireEvent.change(screen.getByLabelText('planned'), { target: { value: '7' } });
  const text = '; filament used [g] = 12.5\n; estimated printing time (normal mode) = 2m';
  fireEvent.change(screen.getByLabelText('Import slicer estimates'), { target: { files: [{ name: 'plate.gcode', size: text.length, text: async () => text }] } });
  await screen.findByText(/Imported estimate: plate.gcode/);
  expect(screen.getAllByLabelText('planned')[0]).toHaveValue(7);
  expect(screen.getAllByLabelText('planned')[1]).toHaveValue(null);
  expect(screen.getAllByLabelText('Estimated time (minutes)')[1]).toHaveValue(2);
  fireEvent.submit(screen.getByRole('button', { name: 'Save production' }).closest('form'));
  await screen.findByRole('alert');
  expect(api.request).not.toHaveBeenCalled();
  fireEvent.change(screen.getAllByLabelText('planned')[1], { target: { value: '3' } });
  fireEvent.change(screen.getByLabelText('Filament'), { target: { value: 'f1' } });
  fireEvent.change(screen.getByLabelText(/Total grams/), { target: { value: '15' } });
  fireEvent.click(screen.getByRole('button', { name: 'Save production' }));
  await waitFor(() => expect(api.request).toHaveBeenCalledOnce());
  const imported = api.request.mock.calls[0][0].data.runs[1];
  expect(imported.estimated_time_minutes).toBe(2);
  expect(imported.print_time_minutes).toBeNull();
  expect(imported.materials[0]).toMatchObject({ weight_used: 15, estimated_weight_g: 15, measurement_state: 'estimated', waste_weight_g: null });
  expect(imported.source_label).toBeUndefined();
  expect(imported.materials[0].source_tool).toBeUndefined();
});

it('leaves the form unchanged when any imported plate is malformed', async () => {
  render(<ProductionModal printers={printers} onClose={() => {}} onSaved={() => {}} />);
  fireEvent.change(screen.getByLabelText('Import slicer estimates'), { target: { files: [{ name: 'broken.gcode', size: 3, text: async () => 'bad' }] } });
  await screen.findByRole('alert');
  expect(screen.getAllByLabelText('planned')).toHaveLength(1);
  expect(api.request).not.toHaveBeenCalled();
});
