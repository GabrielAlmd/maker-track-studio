const normalize = value => (value ?? '').trim().toLowerCase();
const grams = (value, precision) => Number(value.toFixed(precision));

export function matchingFilaments(requirement, filaments, stock, allowOtherColors = false) {
  return filaments.filter(filament => !filament.archived && !stock.get(filament.id)?.archived
    && normalize(filament.type) === normalize(requirement.material_type)
    && (allowOtherColors || !normalize(requirement.color) || normalize(filament.color) === normalize(requirement.color)));
}

function suggestFilament(requirement, filaments, stock, weight) {
  const matches = matchingFilaments(requirement, filaments, stock);
  const preferred = matches.find(filament => filament.id === requirement.preferred_filament_id);
  const available = matches.filter(filament => {
    const balance = stock.get(filament.id);
    return filament.tracking_enabled !== false && balance?.tracking_enabled !== false && balance?.balance?.verified
      && balance.balance.available >= Math.round(weight * 1000);
  });
  return preferred?.id ?? (available.length === 1 ? available[0].id : matches.length === 1 ? matches[0].id : '');
}

export function estimateRunMaterials(items, models, filaments, stock, precision = 3) {
  const requirements = new Map();
  for (const item of items) {
    const model = models.find(candidate => candidate.id === item.model_id);
    const quantity = Number(item.planned);
    if (!model || !Number.isSafeInteger(quantity) || quantity < 1) continue;
    const rows = model.material_requirements?.length ? model.material_requirements : [{
      part_weight_g: model.material_weight ?? 0, waste_weight_g: model.waste_material ?? 0,
    }];
    rows.forEach((requirement, index) => {
      const key = JSON.stringify([model.id, requirement.id ?? index]);
      const existing = requirements.get(key);
      if (existing) existing.quantity += quantity;
      else requirements.set(key, { key, model, requirement, quantity });
    });
  }

  const groups = new Map();
  for (const { key, model, requirement, quantity } of requirements.values()) {
    const waste = requirement.waste_weight_g == null ? null : requirement.waste_weight_g * quantity;
    const weight = (Number(requirement.part_weight_g) + Number(requirement.waste_weight_g ?? 0)) * quantity;
    if (!Number.isFinite(weight) || weight <= 0) continue;
    const structured = Boolean(requirement.material_type);
    const filament = structured ? suggestFilament(requirement, filaments, stock, weight) : '';
    const groupKey = filament || key;
    const group = groups.get(groupKey) ?? { filament_id: filament, weight: 0, waste: 0, sources: [] };
    group.weight += weight;
    group.waste = waste === null || group.waste === null ? null : group.waste + waste;
    group.sources.push({ key, requirementID: requirement.id, model: model.name, type: requirement.material_type, color: requirement.color, quantity });
    groups.set(groupKey, group);
  }

  return [...groups.values()].map(group => ({
    filament_id: group.filament_id,
    weight_used: grams(group.weight, precision),
    estimated_weight_g: grams(group.weight, precision),
    waste_weight_g: group.waste === null ? '' : grams(group.waste, precision),
    measurement_state: 'estimated',
    _sources: group.sources,
  }));
}

// Automatic suggestions may be mapped to the same filament during review.
export function combineMappedEstimates(rows, precision = 3) {
  const positions = new Map();
  const combined = [];
  for (const row of rows) {
    if (!row.filament_id || !row._sources || row.measurement_state !== 'estimated') {
      combined.push(row);
      continue;
    }
    const index = positions.get(row.filament_id);
    if (index === undefined) {
      positions.set(row.filament_id, combined.length);
      combined.push(row);
      continue;
    }
    const previous = combined[index];
    const weight = grams(Number(previous.weight_used) + Number(row.weight_used), precision);
    const waste = [previous, row].some(candidate => candidate.waste_weight_g === '' || candidate.waste_weight_g == null)
      ? '' : grams(Number(previous.waste_weight_g) + Number(row.waste_weight_g), precision);
    combined[index] = { ...previous, weight_used: weight, estimated_weight_g: weight, waste_weight_g: waste === '' ? null : waste,
      _sources: [...previous._sources, ...row._sources] };
  }
  return combined;
}

export function materialStockStatus(filament, stock, total) {
  if (filament?.archived || stock?.archived) return 'archived';
  if (filament?.tracking_enabled === false || stock?.tracking_enabled === false) return 'untracked';
  if (!stock?.balance?.verified) return 'unknown';
  return stock.balance.available < Math.round(Number(total) * 1000) ? 'insufficient' : 'available';
}
