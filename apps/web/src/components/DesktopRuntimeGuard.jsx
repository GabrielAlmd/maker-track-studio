import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import api from '../services/api';
import { getDesktopApp, isDesktopRuntime } from '../platform/runtime';

export default function DesktopRuntimeGuard({ children }) {
  const { t } = useTranslation();
  const desktopApp = getDesktopApp();
  const getStatus = desktopApp?.GetRuntimeStatus;
  const [state, setState] = useState({ ready: false, unreachable: false, status: null });

  useEffect(() => {
    if (isDesktopRuntime() && typeof getStatus !== 'function') {
      setState({ ready: false, unreachable: true, status: null });
      return undefined;
    }
    let active = true;
    let timer;
    const poll = async () => {
      try {
        const desktop = isDesktopRuntime();
        const result = desktop ? await getStatus() : (await api.get('/health/ready', { timeout: 5000 })).data;
        if (active) setState({ ready: desktop ? !!result?.api_ready : result?.status === 'ready', unreachable: false, status: result });
      } catch {
        if (active) setState({ ready: false, unreachable: true, status: null });
      } finally {
        // One pending request at a time; teardown cannot schedule more polling.
        if (active) timer = window.setTimeout(poll, 5000);
      }
    };
    poll();
    return () => { active = false; window.clearTimeout(timer); };
  }, [getStatus]);

  const { ready, unreachable, status } = state;
  if (unreachable || status?.failure_code) {
    const message = isDesktopRuntime() && ['API_BIND_FAILED', 'DIAGNOSTICS_IN_USE'].includes(status?.failure_code)
      ? t('runtime.serviceConflict')
      : t(isDesktopRuntime() ? 'runtime.serviceUnavailable' : 'runtime.webApiUnavailable');
    return <div role="alert" className="p-8 ui-text-inverse"><h1 className="text-xl font-semibold">{t('runtime.unavailable')}</h1><p className="mt-3">{message}</p></div>;
  }
  if (!ready) return <p role="status" className="p-8 ui-text-inverse">{t('runtime.starting')}</p>;
  if (!isDesktopRuntime()) return children;
  const code = typeof status.logs_failure_code === 'string'
    && status.logs_failure_code.length <= 80 && /^DIAG_[A-Z_]+(?:_[0-9]+)?$/.test(status.logs_failure_code)
    ? status.logs_failure_code : '';
  return (
    <>
      {!status.logs_available && <p role="status" className="ui-tone-warning p-3 ui-text-warning">{t('runtime.diagnostics')}{code && ` ${t('runtime.supportCode', { code })}`}</p>}
      {children}
    </>
  );
}
