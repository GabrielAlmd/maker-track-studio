import { useCallback, useEffect, useRef, useState } from 'react';

const initial = { items: [], total: 0, page: 1, page_size: 25 };

// Keeps list controls outside modal state and only asks the API for one page.
export function usePaginatedList(fetchPage, initialSort = 'name', initialDirection = 'asc') {
  const [searchInput, setSearchInput] = useState('');
  const [search, setSearch] = useState('');
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(25);
  const [sort, setSort] = useState(initialSort);
  const [direction, setDirection] = useState(initialDirection);
  const [filters, setFilters] = useState({});
  const [data, setData] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);

  useEffect(() => {
    if (searchInput === search) return;
    const id = setTimeout(() => { setSearch(searchInput); setPage(1); }, 300);
    return () => clearTimeout(id);
  }, [searchInput, search]);
  const generation = useRef(0);
  const mounted = useRef(false);
  const load = useCallback(async () => {
    if (!mounted.current) return;
    const request = ++generation.current;
    const ownsRequest = () => mounted.current && request === generation.current;
    setLoading(true);
    setError(null);
    try {
      const result = await fetchPage({ page, pageSize, sort, direction, search, ...filters });
      if (ownsRequest()) setData(Array.isArray(result) ? { ...initial, legacy: true, items: result, total: result.length, page, page_size: pageSize } : result);
    } catch (err) {
      if (ownsRequest()) setError(err?.message || 'Could not load data.');
    } finally {
      if (ownsRequest()) setLoading(false);
    }
  }, [fetchPage, page, pageSize, sort, direction, search, filters]);
  useEffect(() => {
    mounted.current = true;
    load();
    return () => { mounted.current = false; generation.current += 1; };
  }, [load]);
  const changeFilters = (next) => { setFilters(next); setPage(1); };
  const clear = () => { setSearchInput(''); setSearch(''); setFilters({}); setPage(1); };
  const hasActiveFilters = Object.values(filters).some((value) => value !== '' && value != null);
  return { data, loading, error, refetch: load, searchInput, setSearchInput, page, setPage, pageSize, setPageSize, sort, setSort, direction, setDirection, filters, changeFilters, clear, hasFilters: Boolean(searchInput || hasActiveFilters) };
}
