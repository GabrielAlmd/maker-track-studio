import api from './api';
import { getMigrationSchema, importMigration, inspectMigration, previewMigration } from './migrationService';

vi.mock('./api', () => ({ default: { get: vi.fn(), post: vi.fn() } }));

beforeEach(() => vi.clearAllMocks());

it('uses multipart requests for inspection, preview, and import', async () => {
  api.get.mockResolvedValue({ data: { sections: [] } });
  api.post.mockResolvedValue({ data: { ok: true } });
  const file = new File(['csv'], 'records.csv', { type: 'text/csv' });
  const plan = { sections: [{ section: 'printers' }] };

  await getMigrationSchema();
  await inspectMigration(file);
  await previewMigration(file, plan);
  await importMigration(file, plan);

  expect(api.get).toHaveBeenCalledWith('/data-migration/schema');
  expect(api.post).toHaveBeenCalledTimes(3);
  expect(api.post.mock.calls.map(([path]) => path)).toEqual(['/data-migration/inspect', '/data-migration/preview', '/data-migration/import']);
  expect(api.post.mock.calls[0][1].get('file')).toBe(file);
  expect(JSON.parse(api.post.mock.calls[1][1].get('plan'))).toEqual(plan);
  expect(JSON.parse(api.post.mock.calls[2][1].get('plan'))).toEqual(plan);
});
