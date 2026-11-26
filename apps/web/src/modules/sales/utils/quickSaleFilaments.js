import { estimateRunMaterials, matchingFilaments } from '../../../utils/productionEstimates';

export function quickSaleFilamentChoices({ product, filaments, stock }, quantity, selections) {
  const priced = filaments.filter(filament => !filament.archived && !stock.get(filament.id)?.archived
    && Number.isFinite(filament.avg_price) && filament.avg_price > 0);
  const requirements = product.material_requirements ?? [];
  if (!requirements.length) {
    if (product.is_multicolor || !(product.material_weight > 0)) return [];
    return [{
      key: 'legacy', options: priced, value: priced.find(filament => filament.id === selections.legacy)?.id,
      legacyWeight: product.material_weight + (product.waste_material ?? 0),
    }];
  }

  const estimates = estimateRunMaterials([{ model_id: product.id, planned: quantity }], [product], priced, stock);
  const suggestions = new Map(estimates.flatMap(row => row._sources.map(source => [source.requirementID, row.filament_id])));
  return requirements.filter(requirement => suggestions.has(requirement.id)).map(requirement => {
    const options = matchingFilaments(requirement, priced, stock, true);
    const suggested = suggestions.get(requirement.id);
    const value = options.find(filament => filament.id === selections[requirement.id])?.id ?? suggested;
    const substitutesColor = Boolean(value && !matchingFilaments(requirement, options, stock).some(filament => filament.id === value));
    return { key: requirement.id, requirement, options, suggested, value, substitutesColor };
  });
}

export function quickSaleFilamentInputs(choices) {
  const legacy = choices.find(choice => choice.legacyWeight);
  if (legacy) return { legacy_filament_id: legacy.value };
  return {
    filament_mapping: Object.fromEntries(choices.map(choice => [choice.key, choice.value])),
    allow_color_substitution: choices.some(choice => choice.substitutesColor) || undefined,
  };
}
