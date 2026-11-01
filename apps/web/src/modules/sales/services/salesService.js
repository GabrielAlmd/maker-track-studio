import api from '../../../services/api';

export const getSalesOrders = async (params) => {
  const response = await api.get('/sales-orders', { params });
  return response.data;
};

export const getSalesSummary = async () => {
  const response = await api.get('/sales/summary');
  return response.data;
};

export const getSalesOrder = async (id) => {
  const response = await api.get(`/sales-orders/${id}`);
  return response.data;
};

export const createSalesOrder = async (data) => {
  const response = await api.post('/sales-orders', data);
  return response.data;
};

export const updateSalesOrder = async (id, data) => {
  const response = await api.put(`/sales-orders/${id}`, data);
  return response.data;
};

export const updateSalesOrderStatus = async (id, data) => {
  const response = await api.put(`/sales-orders/${id}/status`, data);
  return response.data;
};

export const deleteSalesOrder = async (id) => {
  await api.delete(`/sales-orders/${id}`);
};

export const getSalesChannels = async (state = 'active') => {
  const response = await api.get('/sales-channels', { params: { state } });
  return response.data;
};

export const createSalesChannel = async (data) => {
  const response = await api.post('/sales-channels', data);
  return response.data;
};

export const getSalesOrderAllocations = async (id) => {
  const response = await api.get(`/sales-orders/${id}/allocations`);
  return response.data;
};

export const replaceSalesOrderAllocations = async (id, allocations) => {
  const response = await api.put(`/sales-orders/${id}/allocations`, { allocations });
  return response.data;
};

export const getCompatiblePrintJobs = async (modelId, params = {}) => {
  const response = await api.get('/print-jobs', {
    params: { page: 1, pageSize: 100, sort: 'date', direction: 'desc', ...params, model_id: modelId },
  });
  return response.data;
};

export const getPrintJobAllocations = async (id) => {
  const response = await api.get(`/print-jobs/${id}/allocations`);
  return response.data;
};
