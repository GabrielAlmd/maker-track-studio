import { afterEach } from 'vitest';
import { exportDesktopData } from './appConfigService';
import api from './api';

vi.mock('./api', () => ({ default: { get: vi.fn() } }));

afterEach(() => {
  delete window.go;
  vi.clearAllMocks();
});

it.each(['/backups/backup.zip', ''])('uses the native save dialog and returns its result (%s)', async (path) => {
  const save = vi.fn().mockResolvedValue(path);
  window.go = { main: { App: { ExportDesktopData: save } } };
  expect(await exportDesktopData()).toEqual({ path, cancelled: !path });
  expect(save).toHaveBeenCalledOnce();
  expect(api.get).not.toHaveBeenCalled();
});

it('preserves browser downloads during frontend development', async () => {
  const blob = new Blob(['backup']);
  api.get.mockResolvedValue({ data: blob, headers: { 'content-disposition': 'attachment; filename="backup.zip"' } });
  expect(await exportDesktopData()).toEqual({ blob, filename: 'backup.zip' });
  expect(api.get).toHaveBeenCalledWith('/desktop-data/export', { responseType: 'blob' });
});
