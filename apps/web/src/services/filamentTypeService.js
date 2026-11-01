import api from './api';

export const getFilamentTypes = async () => (await api.get('/filament-types')).data;
export const updateFilamentTypes = async (types) => (await api.put('/filament-types', { types })).data;
