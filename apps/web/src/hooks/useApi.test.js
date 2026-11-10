import { act, renderHook, waitFor } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { useApi } from './useApi';
import { deferred } from '../test/apiFixtures';

describe('useApi request ownership (B01/B03)', () => {
  it('keeps a useful error and recovers on retry', async () => {
    const fetch = vi.fn().mockRejectedValueOnce(new Error('Could not load data')).mockResolvedValueOnce(['recovered']);
    const { result } = renderHook(() => useApi(fetch));
    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.error).toBe('Could not load data');
    await act(async () => result.current.refetch());
    expect(result.current.data).toEqual(['recovered']);
    expect(result.current.error).toBeNull();
  });

  it.each(['resolve', 'reject'])('ignores an old resource that completes with %s after the current resource', async (completion) => {
    const old = deferred();
    const next = deferred();
    const oldFetch = () => old.promise;
    const nextFetch = () => next.promise;
    const { result, rerender } = renderHook(({ fetch }) => useApi(fetch), { initialProps: { fetch: oldFetch } });
    rerender({ fetch: nextFetch });
    await act(async () => next.resolve(['current']));
    expect(result.current.data).toEqual(['current']);
    await act(async () => old[completion](completion === 'resolve' ? ['obsolete'] : new Error('obsolete failure')));
    expect(result.current.data).toEqual(['current']);
    expect(result.current.error).toBeNull();
  });

  it('does not finish loading while a newer resource is still pending', async () => {
    const old = deferred();
    const next = deferred();
    const oldFetch = () => old.promise;
    const nextFetch = () => next.promise;
    const { result, rerender } = renderHook(({ fetch }) => useApi(fetch), { initialProps: { fetch: oldFetch } });
    rerender({ fetch: nextFetch });
    await act(async () => old.resolve(['obsolete']));
    const loadingDuringNextRequest = result.current.loading;
    await act(async () => next.resolve(['current']));
    expect(loadingDuringNextRequest).toBe(true);
    expect(result.current.data).toEqual(['current']);
  });
  it.each(['resolve', 'reject'])('ignores an overlapping refetch completing with %s', async (completion) => {
    const old = deferred();
    const next = deferred();
    const fetch = vi.fn().mockResolvedValueOnce(['initial']).mockReturnValueOnce(old.promise).mockReturnValueOnce(next.promise);
    const { result } = renderHook(() => useApi(fetch));
    await waitFor(() => expect(result.current.loading).toBe(false));
    let first, second;
    act(() => { first = result.current.refetch(); second = result.current.refetch(); });
    await act(async () => { old[completion](completion === 'resolve' ? ['old'] : new Error('old')); await first; });
    expect(result.current.loading).toBe(true);
    expect(result.current.data).toEqual(['initial']);
    expect(result.current.error).toBeNull();
    await act(async () => { next.resolve(['latest']); await second; });
    expect(result.current.data).toEqual(['latest']);
    expect(result.current.loading).toBe(false);
  });

  it('invalidates requests and refetch on unmount', async () => {
    const pending = deferred();
    const fetch = vi.fn().mockReturnValue(pending.promise);
    const { result, unmount } = renderHook(() => useApi(fetch));
    const refetch = result.current.refetch;
    unmount();
    await act(async () => pending.resolve(['obsolete']));
    await refetch();
    expect(fetch).toHaveBeenCalledTimes(1);
  });

});
