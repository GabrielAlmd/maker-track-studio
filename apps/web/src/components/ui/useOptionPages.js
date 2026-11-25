import { useEffect, useState } from 'react';

const emptyPage = { items: [] };
export default function useOptionPages(loadOptions, open, query, debounceMs) {
  const [page, setPage] = useState(emptyPage);
  useEffect(() => {
    if (!open || !loadOptions) return;
    const controller = new AbortController();
    setPage({ ...emptyPage, loading: true });
    async function fetchPage(cursor) {
      setPage(previous => ({ ...previous, loading: true, error: false, next: null }));
      try {
        const result = await loadOptions({ query, cursor, signal: controller.signal });
        if (controller.signal.aborted) return;
        setPage(previous => ({
          items: cursor == null ? result.items : [...previous.items, ...result.items],
          next: result.nextCursor == null ? null : () => fetchPage(result.nextCursor),
        }));
      } catch {
        if (controller.signal.aborted) return;
        setPage(previous => ({ ...previous, loading: false, error: true, next: () => fetchPage(cursor) }));
      }
    }
    const timer = setTimeout(() => fetchPage(null), debounceMs);
    return () => { clearTimeout(timer); controller.abort(); };
  }, [loadOptions, open, query, debounceMs]);
  return page;
}
