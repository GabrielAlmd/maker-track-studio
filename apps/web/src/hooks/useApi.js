import { useState, useEffect, useCallback, useRef } from 'react';

export const useApi = (apiFunction) => {
  const [data, setData] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const generation = useRef(0);
  const mounted = useRef(false);

  const refetch = useCallback(async () => {
    if (!mounted.current) return;
    const request = ++generation.current;
    const ownsRequest = () => mounted.current && request === generation.current;
    setLoading(true);
    setError(null);
    try {
      const result = await apiFunction();
      if (ownsRequest()) setData(result);
    } catch (err) {
      if (ownsRequest()) setError(err?.message || 'Could not load data.');
    } finally {
      if (ownsRequest()) setLoading(false);
    }
  }, [apiFunction]);

  useEffect(() => {
    mounted.current = true;
    refetch();
    return () => { mounted.current = false; generation.current += 1; };
  }, [refetch]);

  return { data, loading, error, refetch, hasData: data !== null };
};
