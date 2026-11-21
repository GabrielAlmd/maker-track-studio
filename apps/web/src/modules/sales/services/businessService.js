import api from '../../../services/api';

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
