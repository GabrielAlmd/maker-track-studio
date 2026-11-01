import { afterEach, describe, expect, it, vi } from 'vitest';
import { cleanup, render, screen } from '@testing-library/react';
import DesktopRuntimeGuard from './DesktopRuntimeGuard';
import api from '../services/api';

vi.mock('../services/api', () => ({ default: { get: vi.fn() } }));

afterEach(() => { cleanup(); delete window.go; vi.clearAllMocks(); });

describe('desktop runtime availability', () => {
  it('waits for browser API readiness without the desktop bridge', async () => {
    api.get.mockResolvedValue({ data: { status: 'ready' } });
    render(<DesktopRuntimeGuard><p>Business screen</p></DesktopRuntimeGuard>);
    expect(await screen.findByText('Business screen')).toBeInTheDocument();
    expect(api.get).toHaveBeenCalledWith('/health/ready', { timeout: 5000 });
  });

  it('shows a browser API error when readiness cannot be reached', async () => {
    api.get.mockRejectedValue(new Error('network details'));
    render(<DesktopRuntimeGuard><p>Business screen</p></DesktopRuntimeGuard>);
    expect(await screen.findByRole('alert')).toHaveTextContent('configured API is unavailable');
    expect(document.body).not.toHaveTextContent('network details');
  });

  it('waits for desktop startup and warns when logging uses only memory', async () => {
    window.go = { main: { App: { GetRuntimeStatus: vi.fn().mockResolvedValue({ api_ready: true, logs_available: false }) } } };
    render(<DesktopRuntimeGuard><p>Business screen</p></DesktopRuntimeGuard>);
    expect(screen.queryByText('Business screen')).not.toBeInTheDocument();
    expect(await screen.findByText('Business screen')).toBeInTheDocument();
    expect(screen.getByRole('status')).toHaveTextContent('Diagnostic files are unavailable');
  });

  it('does not mount API consumers after a failed startup or print raw failure data', async () => {
    window.go = { main: { App: { GetRuntimeStatus: vi.fn().mockResolvedValue({ api_ready: false, failure_code: 'secret-password' }) } } };
    render(<DesktopRuntimeGuard><p>Business screen</p></DesktopRuntimeGuard>);
    expect(await screen.findByRole('alert')).toHaveTextContent('Application unavailable');
    expect(screen.queryByText('Business screen')).not.toBeInTheDocument();
    expect(document.body).not.toHaveTextContent('secret-password');
  });

  it('shows a sanitized support code when file logging fails', async () => {
    window.go = { main: { App: { GetRuntimeStatus: vi.fn().mockResolvedValue({ api_ready: true, logs_available: false, logs_failure_code: 'DIAG_WINDOWS_SECURITY_OPEN_5' }) } } };
    render(<DesktopRuntimeGuard><p>Business screen</p></DesktopRuntimeGuard>);
    expect(await screen.findByText(/Support code: DIAG_WINDOWS_SECURITY_OPEN_5/)).toBeInTheDocument();
    expect(screen.getByText('Business screen')).toBeInTheDocument();
  });

  it('never displays arbitrary filesystem error text as a support code', async () => {
    window.go = { main: { App: { GetRuntimeStatus: vi.fn().mockResolvedValue({ api_ready: true, logs_available: false, logs_failure_code: 'open C:/Users/secret-customer: access denied' }) } } };
    render(<DesktopRuntimeGuard><p>Business screen</p></DesktopRuntimeGuard>);
    await screen.findByText('Business screen');
    expect(document.body).not.toHaveTextContent('secret-customer');
    expect(document.body).not.toHaveTextContent('Support code:');
  });
});
