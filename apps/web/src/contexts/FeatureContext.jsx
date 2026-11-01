import { useCallback, useEffect, useMemo, useState } from 'react';
import { getFeatures, updateFeatures } from '../services/featureService';
import { FeatureContext } from './featureState';

export function FeatureProvider({ children }) {
  const [features, setFeatures] = useState(null);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState(null);

  const refetch = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const next = await getFeatures();
      setFeatures(next);
      return next;
    } catch {
      setError(true);
      return null;
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    refetch();
  }, [refetch]);

  const updateFeature = useCallback(async (key, enabled) => {
    setSaving(true);
    setError(null);
    try {
      const next = await updateFeatures({ ...(features ?? {}), [key]: enabled });
      setFeatures(next);
      return next;
    } catch (err) {
      setError(true);
      throw err;
    } finally {
      setSaving(false);
    }
  }, [features]);

  const value = useMemo(() => ({
    features,
    loading,
    saving,
    error,
    refetch,
    updateFeature,
  }), [error, features, loading, refetch, saving, updateFeature]);

  return <FeatureContext.Provider value={value}>{children}</FeatureContext.Provider>;
}
