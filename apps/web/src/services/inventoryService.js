import api from './api';
export const getMaterials = async () => (await api.get('/v2/inventory/materials')).data;
export const getMovements = async (id, after = 0) => (await api.get(`/v2/inventory/movements/filament/${id}`, { params: { after } })).data;
export const setThreshold = async (id, threshold_mg) => api.put(`/v2/inventory/thresholds/${id}`, { threshold_mg });
export const postCount = async (data) => (await api.post('/v2/inventory/adjustments', data)).data;
export const stockWarning = (item, requiredMG = null) => !item.balance.verified ? 'Unknown stock — reconcile a physical count' : item.balance.available < 0 ? 'Negative stock — investigate' : requiredMG !== null && requiredMG > item.balance.available ? 'Insufficient material' : item.balance.available <= item.threshold_mg ? 'Low stock' : '';
