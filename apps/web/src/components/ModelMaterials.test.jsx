import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import ModelModal from './ModelModal';
import { ModelDetails } from './ModelMaterials';
import { createModel, updateModel } from '../services/modelService';
import { getFilaments } from '../services/filamentService';
import api from '../services/api';

vi.mock('../services/modelService');
vi.mock('../services/filamentService');
vi.mock('../services/api', () => ({ default: { post: vi.fn() } }));

const fixture = { name: 'Housing', print_time: 20, material_weight: 15, waste_material: 2, is_multicolor: true };
const legacy = { ...fixture, id: 'model-id', material_requirements: [] };

function upload(name, content = '') {
  const file = new File([content], name);
  file.text = async () => content;
  fireEvent.change(screen.getByLabelText('Inspect slicer files (optional)'), { target: { files: [file] } });
}

describe('Model material requirements', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    getFilaments.mockResolvedValue([{ id: 'preferred', brand: 'Fixture', type: 'PLA', color: 'Blue' }]);
  });

  it('edits a legacy model without assuming its split, with optional catalogue preference', async () => {
    const user = userEvent.setup();
    const saved = vi.fn();
    updateModel.mockResolvedValue({ id: legacy.id });
    render(<ModelModal model={legacy} onClose={vi.fn()} onSaved={saved} />);
    expect(screen.getByText(/The material split is unknown/)).toBeVisible();
    expect(screen.getByRole('checkbox', { name: 'Multicolor print' })).toBeChecked();
    await user.click(screen.getByRole('button', { name: 'Add material requirement' }));
    const requirement = screen.getByRole('group', { name: 'Requirement 1' });
    await user.type(within(requirement).getByLabelText('Material type'), 'PETG');
    await user.type(within(requirement).getByLabelText('Color (optional)'), 'Orange');
    await user.type(within(requirement).getByLabelText('Part weight (g)'), '-1');
    const preference = within(requirement).getByRole('combobox', { name: 'Preferred filament (optional)' });
    await user.type(preference, 'fixture');
    await user.keyboard('{Enter}');
    expect(preference).toHaveValue('Fixture · PLA · Blue');
    await user.click(screen.getByRole('button', { name: 'Save changes' }));
    expect(await screen.findByRole('alert')).toHaveTextContent(/cannot be negative/i);
    expect(updateModel).not.toHaveBeenCalled();

    const weight = within(requirement).getByLabelText('Part weight (g)');
    await user.clear(weight);
    await user.type(weight, '15.25');
    expect(screen.getByText('15.25 g')).toBeVisible();
    expect(screen.getByText('Known waste (total incomplete)')).toBeVisible();
    await user.click(screen.getByRole('button', { name: 'Save changes' }));
    await waitFor(() => expect(updateModel).toHaveBeenCalledWith(legacy.id, expect.objectContaining({
      material_requirements: [
        { material_type: 'PETG', color: 'Orange', part_weight_g: 15.25, waste_weight_g: null, preferred_filament_id: 'preferred' },
      ],
    })));
    expect(saved).toHaveBeenCalledWith({ id: legacy.id });
  });

  it('retains tool slots and requires explicit per-unit weights before creating a model', async () => {
    const user = userEvent.setup();
    createModel.mockResolvedValue({ id: 'new-id' });
    render(<ModelModal onClose={vi.fn()} onSaved={vi.fn()} />);
    await user.type(screen.getByLabelText('Name'), 'New housing');
    await user.type(screen.getByLabelText('Print Time (min)'), '20');
    upload('fixture.gcode', '; filament used [g] = 24, 0, 9.5');
    expect(await screen.findByText(/Tool 3 · 9.5 g/)).toBeVisible();
    await user.click(screen.getByRole('button', { name: 'Add draft requirements from tools' }));
    const first = screen.getByRole('group', { name: 'Tool 1' });
    const third = screen.getByRole('group', { name: 'Tool 3' });
    expect(within(first).getByLabelText('Part weight (g)')).toHaveValue(null);
    expect(within(third).getByLabelText('Part weight (g)')).toHaveValue(null);
    await user.click(screen.getByRole('button', { name: 'Create Model' }));
    expect(await screen.findAllByText('Enter a material type')).toHaveLength(2);
    expect(createModel).not.toHaveBeenCalled();

    await user.type(within(first).getByLabelText('Material type'), 'PLA');
    await user.type(within(first).getByLabelText('Part weight (g)'), '4');
    await user.type(within(third).getByLabelText('Material type'), 'PETG');
    await user.type(within(third).getByLabelText('Part weight (g)'), '3');
    await user.type(within(third).getByLabelText('Waste (g, optional)'), '0');
    await user.click(screen.getByRole('button', { name: 'Create Model' }));
    await waitFor(() => expect(createModel).toHaveBeenCalledWith(expect.objectContaining({
      material_requirements: [
        { material_type: 'PLA', color: '', part_weight_g: 4, waste_weight_g: null, preferred_filament_id: null, source_tool: 1 },
        { material_type: 'PETG', color: '', part_weight_g: 3, waste_weight_g: 0, preferred_filament_id: null, source_tool: 3 },
      ],
    })));
  });

  it('shows unsupported, malformed and ambiguous files without generating requirement rows', async () => {
    api.post.mockResolvedValue({ data: { plates: [
      { source_label: '3MF plate 1', items: [{ source_model: 'Housing' }, { source_model: 'Cover' }], materials: [] },
      { source_label: '3MF plate 2', items: [{ source_model: 'Handle' }], materials: [] },
    ] } });
    render(<ModelModal model={legacy} onClose={vi.fn()} onSaved={vi.fn()} />);
    upload('layout.3mf');
    expect(await screen.findByText(/Multiple plates were found/)).toBeVisible();
    expect(screen.getByText('Cover')).toBeVisible();
    expect(screen.queryByRole('button', { name: /draft requirements/ })).toBeNull();
    upload('timing.gcode', '; estimated printing time (normal mode) = 25m');
    expect(await screen.findByText(/provides no per-tool weights/)).toBeVisible();
    upload('invalid.gcode', '; filament used [g] = -2');
    expect(await screen.findByRole('alert')).toHaveTextContent(/Could not read slicer metadata/);
    upload('unsupported.stl');
    expect(await screen.findByRole('alert')).toBeVisible();
    expect(screen.queryByRole('group', { name: /Tool/ })).toBeNull();
    expect(screen.getByLabelText('Material Weight (g)')).toHaveValue(15);
  });

  it('displays read-only stored classifications, known waste and tool identity', () => {
    render(<ModelDetails onClose={vi.fn()} model={{
      ...fixture,
      material_requirements: [{ id: 'row-id', material_type: 'PLA', color: '', part_weight_g: 2.5, waste_weight_g: null, source_tool: 3 }],
      material_summary: { material_status: 'single', color_status: 'unknown', part_weight_g: 2.5, known_waste_g: 0, waste_complete: false },
    }} />);
    expect(screen.getByText('PLA · Unknown')).toBeVisible();
    expect(screen.getByText('Known waste (total incomplete)')).toBeVisible();
    expect(screen.getByText('Tool 3')).toBeVisible();
  });
});
