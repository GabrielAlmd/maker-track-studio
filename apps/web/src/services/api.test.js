import { afterEach, expect, it, vi } from 'vitest';
import axios from 'axios';
import toast from 'react-hot-toast';
import api, { apiBaseURL } from './api';
import { reportFrontendFailure } from './diagnostics';

vi.mock('react-hot-toast', () => ({ default: { error: vi.fn() } }));
vi.mock('./diagnostics', () => ({ reportFrontendFailure: vi.fn() }));
afterEach(() => vi.clearAllMocks());

it('uses same-origin API routing in the browser runtime', () => {
  expect(apiBaseURL).toBe('/api');
});

it('reports a network failure without sending the Axios object', async () => {
  const error = new axios.AxiosError('secret-password', 'ERR_NETWORK');
  await expect(api.get('/app-config', { adapter: () => Promise.reject(error) })).rejects.toBe(error);
  expect(reportFrontendFailure).toHaveBeenCalledExactlyOnceWith('network');
});

it('does not duplicate server failures and never renders raw server error objects', async () => {
  const error = new axios.AxiosError('secret-password');
  error.response = { status: 500, data: { error: { password: 'secret-password' } } };
  await expect(api.get('/app-config', { adapter: () => Promise.reject(error) })).rejects.toBe(error);
  expect(reportFrontendFailure).not.toHaveBeenCalled();
  expect(toast.error).toHaveBeenCalledWith('Error: Error 500');
});

it('does not report cancellation as a network failure', async () => {
  const error = new axios.CanceledError();
  await expect(api.get('/app-config', { adapter: () => Promise.reject(error) })).rejects.toBe(error);
  expect(reportFrontendFailure).not.toHaveBeenCalled();
  expect(toast.error).not.toHaveBeenCalled();
});
