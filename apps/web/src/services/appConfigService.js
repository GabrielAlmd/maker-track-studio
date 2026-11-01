import api from './api';
import { getDesktopApp } from '../platform/runtime';

const FALLBACK_BACKUP_FILENAME = '3d-printing-cost-tracker-backup.zip';

function parseDownloadFilename(contentDisposition) {
  if (!contentDisposition) {
    return FALLBACK_BACKUP_FILENAME;
  }

  const utf8Match = contentDisposition.match(/filename\*=UTF-8''([^;]+)/i);
  if (utf8Match?.[1]) {
    return decodeURIComponent(utf8Match[1]);
  }

  const plainMatch = contentDisposition.match(/filename="?([^";]+)"?/i);
  return plainMatch?.[1] || FALLBACK_BACKUP_FILENAME;
}

export const getAppConfig = async () => {
  const response = await api.get('/app-config');
  return response.data;
};

export const getAppConfigByKey = async (key) => {
  const response = await api.get(`/app-config/${key}`);
  return response.data;
};

export const updateAppConfig = async (key, value, type) => {
  const response = await api.put(`/app-config/${key}`, { value, type });
  return response.data;
};


export const exportDesktopData = async () => {
  const desktopExport = getDesktopApp()?.ExportDesktopData;
  if (desktopExport) {
    const path = await desktopExport();
    return { path, cancelled: !path };
  }

  const response = await api.get('/desktop-data/export', {
    responseType: 'blob',
  });

  return {
    blob: response.data,
    filename: parseDownloadFilename(response.headers['content-disposition']),
  };
};

export const importDesktopData = async (file) => {
  const formData = new FormData();
  formData.append('file', file);

  const response = await api.post('/desktop-data/import', formData, {
    headers: {
      'Content-Type': 'multipart/form-data',
    },
  });

  return response.data;
};
