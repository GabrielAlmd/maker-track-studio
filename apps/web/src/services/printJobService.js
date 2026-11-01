import api from './api';

export const getPrintJobs = async (params) => {
  const response = await api.get('/print-jobs', params ? { params } : undefined);
  return response.data;
};

export const getPrintJobStats = async () => {
  const response = await api.get('/print-jobs/stats');
  return response.data;
};

export const getDashboardSummary = async () => {
  const response = await api.get('/print-jobs/dashboard-summary');
  return response.data;
};

export const getPrintJob = async (id) => {
  const response = await api.get(`/print-jobs/${id}`);
  return response.data;
};

export const getPrintJobCost = async (id) => {
  const response = await api.get(`/print-jobs/${id}/cost`);
  return response.data;
};

export const createPrintJob = async (data) => {
  const response = await api.post('/print-jobs', data);
  return response.data;
};

export const updatePrintJob = async (id, data) => {
  const response = await api.put(`/print-jobs/${id}`, data);
  return response.data;
};

export const deletePrintJob = async (id) => {
  await api.delete(`/print-jobs/${id}`);
};
