import { useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import toast from 'react-hot-toast';
import { saveRecord } from '../services/businessService';

function operationID() {
  if (typeof crypto.randomUUID === 'function') return crypto.randomUUID();
  const bytes = crypto.getRandomValues(new Uint8Array(16));
  bytes[6] = (bytes[6] & 15) | 64;
  bytes[8] = (bytes[8] & 63) | 128;
  const hex = Array.from(bytes, (byte) => byte.toString(16).padStart(2, '0')).join('');
  return [hex.slice(0, 8), hex.slice(8, 12), hex.slice(12, 16), hex.slice(16, 20), hex.slice(20)].join('-');
}

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
      setError(t(status === 409 ? 'business.conflict' : status === 400 ? 'business.invalid' : 'business.saveError'));
      if (status >= 400 && status < 500) retry.current = null;
      return null;
    } finally {
      if (retry.current) retry.current.busy = false;
      setBusy(false);
    }
  }

  return { run, busy, error };
}
