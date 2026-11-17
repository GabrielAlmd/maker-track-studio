import api from './api';

export const createPrintJobFilament = async (data) => {
    const response = await api.post('/print-job-filaments', data);
    return response.data;
};

export const updatePrintJobFilament = async (id, data) => {
    const response = await api.put(`/print-job-filaments/${id}`, data);
    return response.data;
};

export const deletePrintJobFilament = async (id) => {
    await api.delete(`/print-job-filaments/${id}`);
};
