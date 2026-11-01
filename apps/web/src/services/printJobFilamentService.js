import api from './api';

export const createPrintJobFilament = async (data) => {
    const response = await api.post('/print-job-filaments', data);
    return response.data;
};

export const deletePrintJobFilament = async (id) => {
    await api.delete(`/print-job-filaments/${id}`);
};
