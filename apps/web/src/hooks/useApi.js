import { useState, useEffect } from 'react';

// Custom hook for data fetching with loading and error states
export const useApi = (apiFunction) => {
  const [data, setData] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);

  useEffect(() => {
    const fetchData = async () => {
      try {
        setLoading(true);
        setError(null);
        const result = await apiFunction();
        setData(result);
      } catch (err) {
        setError(err?.message || 'Could not load data.');
      } finally {
        setLoading(false);
      }
    };

    fetchData();
  }, [apiFunction]);

  const refetch = async () => {
    try {
      setLoading(true);
      setError(null);
      const result = await apiFunction();
      setData(result);
    } catch (err) {
      setError(err?.message || 'Could not load data.');
    } finally {
      setLoading(false);
    }
  };

  return { data, loading, error, refetch, hasData: data !== null };
};
