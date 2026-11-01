import api from './api';
import { getDesktopApp } from '../platform/runtime';

export async function exportReport(params) {
  const query = new URLSearchParams(params).toString();
  const desktopSave = getDesktopApp()?.ExportReport;
  if (desktopSave) {
    const path = await desktopSave(query);
    return { path, cancelled: !path };
  }
  const response = await api.get(`/reports/export?${query}`, { responseType: 'blob' });
  const filename = response.headers['content-disposition']?.match(/filename="?([^";]+)"?/)?.[1] || `maker-track-export.${params.format === 'pdf' ? 'pdf' : 'zip'}`;
  const url = URL.createObjectURL(response.data);
  const link = document.createElement('a');
  link.href = url;
  link.download = filename;
  document.body.appendChild(link);
  link.click();
  link.remove();
  URL.revokeObjectURL(url);
  return { path: filename, cancelled: false };
}
