import { useCallback, useEffect, useState } from 'react';

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

  useEffect(() => { const id = setTimeout(() => { setSearch(searchInput); setPage(1); }, 300); return () => clearTimeout(id); }, [searchInput]);
  const load = useCallback(async () => {
    try { setLoading(true); setError(null); const result = await fetchPage({ page, pageSize, sort, direction, search, ...filters }); setData(Array.isArray(result) ? { ...initial, legacy: true, items: result, total: result.length, page, page_size: pageSize } : result); }
    catch (err) { setError(err?.message || 'Could not load data.'); }
    finally { setLoading(false); }
  }, [fetchPage, page, pageSize, sort, direction, search, filters]);
  useEffect(() => { load(); }, [load]);
  const changeFilters = (next) => { setFilters(next); setPage(1); };
  const clear = () => { setSearchInput(''); setSearch(''); setFilters({}); setPage(1); };
  const hasActiveFilters = Object.values(filters).some((value) => value !== '' && value != null);
  return { data, loading, error, refetch: load, searchInput, setSearchInput, page, setPage, pageSize, setPageSize, sort, setSort, direction, setDirection, filters, changeFilters, clear, hasFilters: Boolean(searchInput || hasActiveFilters) };
}
