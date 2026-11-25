import api from './api';
export const getMaterials = async () => (await api.get('/v2/inventory/materials')).data;
export const getMovements = async (id, after = 0) => (await api.get(`/v2/inventory/movements/filament/${id}`, { params: { after } })).data;
export const setThreshold = async (id, threshold_mg) => api.put(`/v2/inventory/thresholds/${id}`, { threshold_mg });
export const updateLifecycle = (id, changes) => api.patch(`/v2/inventory/materials/${id}`, changes);
export const postCount = async (data) => (await api.post('/v2/inventory/adjustments', data)).data;
export function stockWarning(item, requiredMG = null) {
  if (item.tracking_enabled === false || item.warnings_enabled === false || item.archived) return '';
  if (!item.balance.verified) return 'Unknown stock — reconcile a physical count';
  if (item.balance.available < 0) return 'Negative stock — investigate';
  if (requiredMG !== null && requiredMG > item.balance.available) return 'Insufficient material';
  return item.balance.available <= item.threshold_mg ? 'Low stock' : '';
}
export const getFinishedGoods = async () => (await api.get('/v2/inventory/finished-goods')).data;
