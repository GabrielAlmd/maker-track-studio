import api from './api';

export const getFilaments = async (params) => {
  const response = await api.get('/filaments', params ? { params } : undefined);
  return response.data;
};

export const getFilamentBrands = async () => {
  const response = await api.get('/filament-brands');
  return response.data;
};

export const getFilament = async (id) => {
  const response = await api.get(`/filaments/${id}`);
  return response.data;
};

export const createFilament = async (data) => {
  const response = await api.post('/filaments', data);
  return response.data;
};

export const updateFilament = async (id, data) => {
  const response = await api.put(`/filaments/${id}`, data);
  return response.data;
};

export const deleteFilament = async (id) => {
  await api.delete(`/filaments/${id}`);
};
