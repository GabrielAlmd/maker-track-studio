import api from '../../../services/api';
import { getDesktopApp } from '../../../platform/runtime';

const root = '/v2/business';

export async function listRecords(kind, page = 1, suffix = '') {
  const { data } = await api.get(`${root}/${kind}${suffix}`, { params: { page, page_size: 25 } });
  return data;
}

export async function getRecord(path, revision) {
  const { data } = await api.get(`${root}/${path}`, { params: { revision } });
  return data;
}

export async function saveRecord(path, payload, method = 'post') {
  const { data } = await api[method](`${root}/${path}`, payload);
  return data;
}

function saveBrowserDownload(blob, filename) {
  const url = URL.createObjectURL(blob);
  const link = document.createElement('a');
  link.href = url;
  link.download = filename;
  document.body.appendChild(link);
  link.click();
  link.remove();
  URL.revokeObjectURL(url);
}

export async function downloadProposalPDF(id, revision, language) {
  const save = getDesktopApp()?.ExportProposalPDF;
  if (save) return save(id, revision, language);
  const { data } = await api.get(`${root}/proposals/${id}/document/pdf`, {
    params: { revision, language }, responseType: 'blob',
  });
  saveBrowserDownload(data, `proposal-${id}-r${revision}.pdf`);
}

export async function uploadOrderAttachment(id, file, operationID) {
  const { data } = await api.post(`${root}/orders/${id}/attachments`, file, {
    params: { name: file.name, operation_id: operationID },
    headers: { 'Content-Type': 'application/octet-stream' },
  });
  return data;
}

export async function downloadOrderAttachment(id, attachment) {
  const save = getDesktopApp()?.ExportOrderAttachment;
  if (save) return save(id, attachment.id, attachment.name);
  const { data } = await api.get(`${root}/orders/${id}/attachments/${attachment.id}`, { responseType: 'blob' });
  saveBrowserDownload(data, attachment.name);
}
