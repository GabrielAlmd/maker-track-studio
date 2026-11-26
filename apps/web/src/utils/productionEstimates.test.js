import { combineMappedEstimates, estimateRunMaterials, materialStockStatus } from './productionEstimates';

const makeModel = (id, ...material_requirements) => ({ id, name: id, material_requirements });
const requirement = { id: 'r', material_type: ' PLA ', color: 'Black', part_weight_g: 8.25, waste_weight_g: 0.25 };
const filaments = [
  { id: 'black', type: 'pla', color: 'black' },
  { id: 'paused', type: 'PLA', color: 'Black', tracking_enabled: false },
  { id: 'archived', type: 'PLA', color: 'Black', archived: true },
  { id: 'wrong', type: 'PETG', color: 'Black' },
];
const stock = new Map([
  ['black', { balance: { verified: true, available: 90000 } }],
  ['paused', { balance: { verified: true, available: 90000 } }],
]);

it('aggregates repeated and mixed model estimates by compatible filament and retains their sources', () => {
  const rows = estimateRunMaterials(
    [{ model_id: 'A', planned: 2 }, { model_id: 'A', planned: 1 }, { model_id: 'B', planned: 2 }],
    [makeModel('A', { ...requirement, preferred_filament_id: 'archived' }),
      makeModel('B', { ...requirement, preferred_filament_id: 'wrong', waste_weight_g: null })],
    filaments, stock,
  );
  expect(rows).toEqual([expect.objectContaining({
    filament_id: 'black', weight_used: 42, estimated_weight_g: 42, waste_weight_g: '', measurement_state: 'estimated',
    _sources: [expect.objectContaining({ model: 'A', quantity: 3 }), expect.objectContaining({ model: 'B', quantity: 2 })],
  })]);
  expect(estimateRunMaterials([{ model_id: 'A', planned: 20 }], [makeModel('A', requirement)], filaments, stock)[0].filament_id).toBe('');
  expect(materialStockStatus(filaments[0], stock.get('black'), 100)).toBe('insufficient');
  expect(materialStockStatus(filaments[1], stock.get('paused'), 1)).toBe('untracked');
  expect(materialStockStatus(filaments[0], undefined, 1)).toBe('unknown');
});

it('combines reviewed estimates with the same mapping and leaves actual duplicate rows for validation', () => {
  const rows = estimateRunMaterials([{ model_id: 'A', planned: 1 }],
    [makeModel('A', requirement, { ...requirement, id: 'second', color: 'Red' })], [], stock);
  const mapped = rows.map(row => ({ ...row, filament_id: 'black' }));
  const actual = { ...mapped[0], measurement_state: 'verified' };
  expect(combineMappedEstimates([...mapped, actual])).toEqual([
    expect.objectContaining({ weight_used: 17, waste_weight_g: 0.5, _sources: rows.flatMap(row => row._sources) }),
    actual,
  ]);
});
