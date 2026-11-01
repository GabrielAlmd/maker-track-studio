const kinds = new Set(['render', 'script', 'promise', 'network']);
const lastReported = new Map();
const uuidV4 = /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;
import { getDesktopApp } from '../platform/runtime';

// No Error objects, messages, URLs, stack traces or form values cross this bridge.
export function reportFrontendFailure(kind, requestId = '') {
  if (!kinds.has(kind)) return;
  const report = getDesktopApp()?.ReportFrontendFailure;
  if (typeof report !== 'function') return;
  const now = performance.now();
  if (lastReported.has(kind) && now - lastReported.get(kind) < 5000) return;
  lastReported.set(kind, now);
  const safeId = typeof requestId === 'string' && requestId.length === 36 && uuidV4.test(requestId) ? requestId : '';
  try {
    Promise.resolve(report(kind, safeId)).catch(() => {
      // Reporting failures never report themselves or retry recursively.
    });
  } catch {
    // The bridge can be unavailable during startup or shutdown.
  }
}

export function installGlobalErrorReporting() {
  const onError = () => reportFrontendFailure('script');
  const onRejection = () => reportFrontendFailure('promise');
  window.addEventListener('error', onError);
  window.addEventListener('unhandledrejection', onRejection);
  return () => {
    window.removeEventListener('error', onError);
    window.removeEventListener('unhandledrejection', onRejection);
  };
}
