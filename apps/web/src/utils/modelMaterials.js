export const maxModelGrams = 1e9;
export const maxRequirements = 100;
export const blankRequirement = { material_type: '', color: '', part_weight_g: '', waste_weight_g: '', preferred_filament_id: '' };

export function summarizeModelMaterials(rows) {
  const types = new Set(rows.map(row => row.material_type?.trim().toLowerCase()).filter(Boolean));
  const colors = new Set(rows.map(row => row.color?.trim().toLowerCase()).filter(Boolean));
  const complete = field => rows.every(row => row[field] != null && row[field] !== '' && Number.isFinite(Number(row[field])));
  const sum = field => rows.reduce((total, row) => total + (Number(row[field]) || 0), 0);
  return {
    part_weight_g: complete('part_weight_g') ? sum('part_weight_g') : null,
    known_waste_g: sum('waste_weight_g'),
    waste_complete: complete('waste_weight_g'),
    color_status: colors.size > 1 ? 'multiple' : rows.every(row => row.color?.trim()) ? 'single' : 'unknown',
    material_status: types.size > 1 ? 'multiple' : rows.every(row => row.material_type?.trim()) ? 'single' : 'unknown',
  };
}
