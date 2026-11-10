import { act, renderHook, waitFor } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { usePaginatedList } from './usePaginatedList';
import { apiPage, deferred } from '../test/apiFixtures';

describe('usePaginatedList contracts and request ownership (A05/B02/B03)', () => {
  it('keeps real page metadata and sends the selected page', async () => {
    const fetch = vi.fn().mockResolvedValue(apiPage([{ id: 'first' }], { total: 26 }))
      .mockResolvedValueOnce(apiPage([{ id: 'first' }], { total: 26 }));
    const { result } = renderHook(() => usePaginatedList(fetch));
    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.data.total).toBe(26);
    fetch.mockResolvedValueOnce(apiPage([{ id: 'last' }], { total: 26, page: 2 }));
    act(() => result.current.setPage(2));
    await waitFor(() => expect(result.current.data.page).toBe(2));
    expect(fetch).toHaveBeenLastCalledWith({ page: 2, pageSize: 25, sort: 'name', direction: 'asc', search: '' });
    expect(result.current.data.items).toEqual([{ id: 'last' }]);
  });

  it('supports the legacy array contract explicitly', async () => {
    const fetch = vi.fn().mockResolvedValue([{ id: 'legacy' }]);
    const { result } = renderHook(() => usePaginatedList(fetch));
    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.data).toEqual({ ...apiPage([{ id: 'legacy' }]), legacy: true });
  });

  it('resets the page when filters change and clears the active filters', async () => {
    const fetch = vi.fn().mockResolvedValue(apiPage());
    const { result } = renderHook(() => usePaginatedList(fetch));
    await waitFor(() => expect(result.current.loading).toBe(false));
    act(() => result.current.setPage(3));
    await waitFor(() => expect(fetch).toHaveBeenLastCalledWith(expect.objectContaining({ page: 3 })));
    act(() => result.current.changeFilters({ brand: 'Fixture' }));
    await waitFor(() => expect(fetch).toHaveBeenLastCalledWith(expect.objectContaining({ page: 1, brand: 'Fixture' })));
    expect(result.current.hasFilters).toBe(true);
    act(() => result.current.clear());
    await waitFor(() => expect(result.current.hasFilters).toBe(false));
  });

  it.each(['resolve', 'reject'])('ignores an older page that completes with %s', async (completion) => {
    const old = deferred();
    const next = deferred();
    const fetch = vi.fn().mockReturnValueOnce(old.promise).mockReturnValueOnce(next.promise);
    const { result } = renderHook(() => usePaginatedList(fetch));
    act(() => result.current.setPage(2));
    await act(async () => next.resolve(apiPage([{ id: 'page-2' }], { page: 2 })));
    await act(async () => old[completion](completion === 'resolve' ? apiPage([{ id: 'page-1' }]) : new Error('obsolete page error')));
    expect(result.current.data.items).toEqual([{ id: 'page-2' }]);
    expect(result.current.error).toBeNull();
  });
  it.each(['filter', 'search', 'refetch'])('keeps current loading and data when %s changes during a request', async (change) => {
    const old = deferred();
    const next = deferred();
    const fetch = vi.fn().mockReturnValueOnce(old.promise).mockReturnValueOnce(next.promise);
    const { result } = renderHook(() => usePaginatedList(fetch));
    if (change === 'filter') act(() => result.current.changeFilters({ brand: 'New' }));
    if (change === 'search') {
      act(() => result.current.setSearchInput('new query'));
      await waitFor(() => expect(fetch).toHaveBeenCalledTimes(2));
      expect(fetch).toHaveBeenLastCalledWith(expect.objectContaining({ search: 'new query' }));
    }
    if (change === 'refetch') act(() => { result.current.refetch(); });
    await act(async () => old.reject(new Error('obsolete')));
    expect(result.current.loading).toBe(true);
    expect(result.current.data).toBeNull();
    expect(result.current.error).toBeNull();
    await act(async () => next.resolve(apiPage([{ id: 'latest' }])));
    expect(result.current.loading).toBe(false);
    expect(result.current.data.items).toEqual([{ id: 'latest' }]);
  });

  it('does not reset page navigation when the search has not changed', async () => {
    vi.useFakeTimers();
    try {
      const fetch = vi.fn().mockResolvedValue(apiPage());
      const { result, unmount } = renderHook(() => usePaginatedList(fetch));
      await act(async () => {});
      await act(async () => result.current.setPage(2));
      await act(async () => vi.advanceTimersByTimeAsync(350));
      expect(result.current.page).toBe(2);
      expect(fetch).toHaveBeenLastCalledWith(expect.objectContaining({ page: 2 }));
      unmount();
    } finally { vi.useRealTimers(); }
  });

});
