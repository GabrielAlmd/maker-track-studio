import api from './api';

function migrationForm(file, plan) {
  const form = new FormData();
  form.append('file', file);
  if (plan) form.append('plan', JSON.stringify(plan));
  return form;
}

const multipart = { headers: { 'Content-Type': 'multipart/form-data' } };

export async function getMigrationSchema() {
  const response = await api.get('/data-migration/schema');
  return response.data;
}

export async function inspectMigration(file) {
  const response = await api.post('/data-migration/inspect', migrationForm(file), multipart);
  return response.data;
}

export async function previewMigration(file, plan) {
  const response = await api.post('/data-migration/preview', migrationForm(file, plan), multipart);
  return response.data;
}

export async function importMigration(file, plan) {
  const response = await api.post('/data-migration/import', migrationForm(file, plan), multipart);
  return response.data;
}

export async function downloadMigrationTemplate(section) {
  const response = await api.get(`/data-migration/templates/${section}`, { responseType: 'blob' });
  const url = window.URL.createObjectURL(response.data);
  const link = document.createElement('a');
  link.href = url;
  link.download = `makertrack-${section}-template.csv`;
  document.body.appendChild(link);
  link.click();
  link.remove();
  window.URL.revokeObjectURL(url);
}

export async function downloadMigrationWorkbookTemplate() {
  const response = await api.get('/data-migration/workbook-template', { responseType: 'blob' });
  const url = window.URL.createObjectURL(response.data);
  const link = document.createElement('a');
  link.href = url;
  link.download = 'makertrack-migration-template.xlsx';
  document.body.appendChild(link);
  link.click();
  link.remove();
  window.URL.revokeObjectURL(url);
}
