import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import api from '../services/api';
import ProductionModal from './ProductionModal';

vi.mock('../services/api', () => ({ default: { get: vi.fn(), request: vi.fn() } }));

const printers = [{ id: 'printer-1', name: 'Workshop', is_default: true }];

const material = { id: 'r1', material_type: 'PLA', color: 'Black', part_weight_g: 5 };
const models = [
  { id: 'm1', name: 'Bracket', material_requirements: [material] },
  { id: 'm2', name: 'Cover', material_requirements: [{ ...material, part_weight_g: 2 }] },
  { id: 'm3', name: 'Spacer', material_requirements: [{ ...material, color: 'Orange' }] },
];

function renderProduction() {
  render(<ProductionModal models={models}
    filaments={[{ id: 'f1', brand: 'Workshop', type: 'PLA', color: 'Black' }]}
    printers={[{ id: 'p1', name: 'Workshop printer', is_default: true }]}
    onClose={vi.fn()} onSaved={vi.fn()} />);
}

async function chooseModel(user, run, name, index = 0) {
  await user.type(run.getAllByRole('combobox', { name: 'Model', exact: true })[index], name);
  await user.click(screen.getByRole('option', { name }));
}

beforeEach(() => {
  vi.clearAllMocks();
  api.get.mockResolvedValue({ data: [] });
  api.request.mockResolvedValue({});
});

it('aggregates mixed models per plate and submits estimates through the production API', async () => {
  const user = userEvent.setup();
  renderProduction();
  const first = within(screen.getByRole('group', { name: 'Run 1' }));
  await chooseModel(user, first, 'Bracket');
  fireEvent.change(first.getByLabelText('planned'), { target: { value: '3' } });
  await user.click(first.getByRole('button', { name: 'Add model' }));
  await chooseModel(user, first, 'Cover', 1);
  expect(first.getByLabelText('Total grams')).toHaveValue(17);
  expect(first.getByText('Bracket × 3 · PLA / Black')).toBeVisible();
  expect(first.getByText('Cover × 1 · PLA / Black')).toBeVisible();

  await user.click(screen.getByRole('button', { name: 'Add run' }));
  const second = within(screen.getByRole('group', { name: 'Run 2' }));
  await chooseModel(user, second, 'Spacer');
  await user.click(screen.getByRole('button', { name: 'Save production' }));
  expect(await second.findByText('Select a filament for each material row')).toBeVisible();
  expect(api.request).not.toHaveBeenCalled();
  await user.type(second.getByRole('combobox', { name: 'Filament' }), 'black');
  await user.click(screen.getByRole('option', { name: /black/i }));
  await user.click(screen.getByRole('button', { name: 'Save production' }));
  await waitFor(() => expect(api.request).toHaveBeenCalled());
  const { runs } = api.request.mock.calls[0][0].data;
  expect(runs.map(run => run.materials)).toEqual([17, 5].map(weight => [{
    filament_id: 'f1', weight_used: weight, estimated_weight_g: weight, waste_weight_g: null, measurement_state: 'estimated',
  }]));
});

it('keeps imported plate grams and duration when selecting a model and changing the quantity', async () => {
  const user = userEvent.setup();
  renderProduction();
  const text = '; filament used [g] = 0, 0, 28\n; estimated printing time (normal mode) = 41m';
  fireEvent.change(screen.getByLabelText('Import slicer estimates'), {
    target: { files: [{ name: 'plate.gcode', size: text.length, text: async () => text }] },
  });
  const run = within(await screen.findByRole('group', { name: 'Run 2' }));
  await chooseModel(user, run, 'Bracket');
  fireEvent.change(run.getByLabelText('planned'), { target: { value: '2' } });
  expect(run.getByLabelText('Total grams')).toHaveValue(28);
  expect(run.getByDisplayValue('41')).toBeVisible();
  expect(run.getByText(/Material rows were kept/)).toBeVisible();
  await user.click(run.getByRole('button', { name: 'Replace materials with model estimates' }));
  expect(run.getByLabelText('Total grams')).toHaveValue(10);
  expect(run.getByDisplayValue('41')).toBeVisible();
});

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
  expect(await screen.findByText('Select a filament for each material row')).toBeVisible();
  expect(api.request).not.toHaveBeenCalled();
  fireEvent.change(screen.getAllByLabelText('planned')[1], { target: { value: '3' } });
  fireEvent.change(screen.getByLabelText('Filament'), { target: { value: 'Blue' } });
  fireEvent.click(screen.getByRole('option', { name: 'Test PLA — Blue' }));
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
