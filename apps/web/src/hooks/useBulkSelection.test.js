import { act, renderHook } from '@testing-library/react';
import { useBulkSelection } from './useBulkSelection';

const items = [{ id: '1', name: 'One' }, { id: '2', name: 'Two' }];

describe('useBulkSelection', () => {
  it('selects and deselects all visible rows', () => {
    const { result } = renderHook(() => useBulkSelection({ items, deleteItem: vi.fn(), getLabel: (item) => item.name }));
    act(() => result.current.toggleAll());
    expect([...result.current.selectedIds]).toEqual(['1', '2']);
    act(() => result.current.toggleAll());
    expect(result.current.selectedCount).toBe(0);
  });

  it('clears successful rows and retains named failures', async () => {
    const deleteItem = vi.fn().mockResolvedValueOnce().mockRejectedValueOnce(new Error('Still referenced'));
    const onDeleted = vi.fn();
    const { result } = renderHook(() => useBulkSelection({ items, deleteItem, getLabel: (item) => item.name, onDeleted, failureFallback: 'Failed' }));
    act(() => result.current.toggleAll());
    await act(() => result.current.removeSelected());
    expect(deleteItem.mock.calls).toEqual([['1'], ['2']]);
    expect([...result.current.selectedIds]).toEqual(['2']);
    expect(result.current.result).toEqual({ deletedCount: 1, failures: [{ id: '2', label: 'Two', message: 'Still referenced' }] });
    expect(onDeleted).toHaveBeenCalledOnce();
  });

  it('drops selections that leave the visible result set', () => {
    const props = { deleteItem: vi.fn(), getLabel: (item) => item.name };
    const { result, rerender } = renderHook(({ visible }) => useBulkSelection({ ...props, items: visible }), { initialProps: { visible: items } });
    act(() => result.current.toggle('1'));
    rerender({ visible: [items[1]] });
    expect(result.current.selectedCount).toBe(0);
  });
});
