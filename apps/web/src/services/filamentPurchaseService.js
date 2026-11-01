import api from './api';

export const getFilamentPurchases = async (params) => {
  const response = await api.get('/filament-purchases', params ? { params } : undefined);
  return response.data;
};

export const getFilamentPurchase = async (id) => {
  const response = await api.get(`/filament-purchases/${id}`);
  return response.data;
};

export const createFilamentPurchase = async (data) => {
  const response = await api.post('/filament-purchases', data);
  return response.data;
};

export const updateFilamentPurchase = async (id, data) => {
  const response = await api.put(`/filament-purchases/${id}`, data);
  return response.data;
};

export const deleteFilamentPurchase = async (id) => {
  await api.delete(`/filament-purchases/${id}`);
};

export const createFilamentPurchaseBatch = async (data) => {
  const response = await api.post('/filament-purchases/batch', data);
  return response.data;
};
