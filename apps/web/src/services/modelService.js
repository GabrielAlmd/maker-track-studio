import api from './api';

// Get all models
export const getModels = async (params) => {
  const response = await api.get('/models', params ? { params } : undefined);
  return response.data;
};

// Get a single model by ID
export const getModel = async (id) => {
  const response = await api.get(`/models/${id}`);
  return response.data;
};

// Create a new model
export const createModel = async (modelData) => {
  const response = await api.post('/models', modelData);
  return response.data;
};

// Update an existing model
export const updateModel = async (id, modelData) => {
  const response = await api.put(`/models/${id}`, modelData);
  return response.data;
};

// Delete a model
export const deleteModel = async (id) => {
  const response = await api.delete(`/models/${id}`);
  return response.data;
};
