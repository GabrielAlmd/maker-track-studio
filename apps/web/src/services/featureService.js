import api from './api';

export const getFeatures = async () => {
  const response = await api.get('/features');
  return response.data;
};

export const updateFeatures = async (features) => {
  const response = await api.put('/features', features);
  return response.data;
};
