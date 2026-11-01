import api from './api';

export const getPrinters = async (state = 'active') => {
  const response = await api.get('/printers', { params: { state } });
  return response.data;
};

export const createPrinter = async (data) => {
  const response = await api.post('/printers', data);
  return response.data;
};

export const updatePrinter = async (id, data) => {
  const response = await api.put(`/printers/${id}`, data);
  return response.data;
};

export const setDefaultPrinter = async (id) => {
  const response = await api.put(`/printers/${id}/default`);
  return response.data;
};

export const archivePrinter = async (id) => {
  const response = await api.put(`/printers/${id}/archive`);
  return response.data;
};

export const restorePrinter = async (id) => {
  const response = await api.put(`/printers/${id}/restore`);
  return response.data;
};

export const updatePrinterCostConfig = async (id, costConfig) => {
  const response = await api.put(`/printers/${id}/cost-config`, { cost_config: costConfig });
  return response.data;
};
