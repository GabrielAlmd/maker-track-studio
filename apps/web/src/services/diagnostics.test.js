import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

beforeEach(() => { vi.resetModules(); });
afterEach(() => { delete window.go; vi.restoreAllMocks(); });

describe('frontend diagnostic bridge', () => {
  it('sends only fixed kinds and valid correlation IDs, with a rate limit', async () => {
    const report = vi.fn().mockResolvedValue(undefined);
    window.go = { main: { App: { ReportFrontendFailure: report } } };
    vi.spyOn(performance, 'now').mockReturnValue(1000);
    const { reportFrontendFailure } = await import('./diagnostics');
    reportFrontendFailure('secret-password', 'secret-password');
    for (let i = 0; i < 100; i++) reportFrontendFailure('render', 'secret-password');
    expect(report).toHaveBeenCalledExactlyOnceWith('render', '');
  });

  it('does not retry or report bridge failures', async () => {
    const report = vi.fn().mockRejectedValue(new Error('secret-bridge-error'));
    window.go = { main: { App: { ReportFrontendFailure: report } } };
    const { reportFrontendFailure } = await import('./diagnostics');
    reportFrontendFailure('network');
    await Promise.resolve();
    await Promise.resolve();
    expect(report).toHaveBeenCalledTimes(1);
    delete window.go;
    expect(() => reportFrontendFailure('script')).not.toThrow();
  });

  it('ignores exception payloads and removes global listeners on cleanup', async () => {
    const report = vi.fn().mockResolvedValue(undefined);
    window.go = { main: { App: { ReportFrontendFailure: report } } };
    const { installGlobalErrorReporting } = await import('./diagnostics');
    const remove = installGlobalErrorReporting();
    window.dispatchEvent(new ErrorEvent('error', { message: 'secret-password', filename: '/home/customer/private' }));
    window.dispatchEvent(new Event('unhandledrejection'));
    expect(report.mock.calls).toEqual([['script', ''], ['promise', '']]);
    remove();
    report.mockClear();
    window.dispatchEvent(new ErrorEvent('error', { message: 'secret-password' }));
    expect(report).not.toHaveBeenCalled();
  });
});
