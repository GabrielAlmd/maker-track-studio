import { useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import toast from 'react-hot-toast';
import { saveRecord } from '../services/businessService';

import operationID from '../utils/operationID';

export default function useBusinessCommand() {
  const { t } = useTranslation();
  const retry = useRef(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState(null);

  async function run(path, values, method) {
    const key = JSON.stringify([path, values, method]);
    if (retry.current?.busy) return null;
    if (retry.current?.key !== key) retry.current = { key, id: operationID() };
    retry.current.busy = true;
    setBusy(true);
    setError(null);
    try {
      const result = await saveRecord(path, { ...values, operation_id: retry.current.id }, method);
      retry.current = null;
      toast.success(t('business.saved'));
      return result;
    } catch (failure) {
      const status = failure?.response?.status;
      const code = failure?.response?.data?.error?.code;
      const knownError = {
        stock_unavailable: 'business.quickStockChanged',
        cost_unavailable: 'business.quickMissingCost',
        estimate_changed: 'business.quickEstimateChanged',
      }[code];
      setError(t(knownError ?? (status === 409 ? 'business.conflict' : status === 400 ? 'business.invalid' : 'business.saveError')));
      if (status >= 400 && status < 500 && code !== 'concurrent_update') retry.current = null;
      return null;
    } finally {
      if (retry.current) retry.current.busy = false;
      setBusy(false);
    }
  }

  return { run, busy, error };
}
