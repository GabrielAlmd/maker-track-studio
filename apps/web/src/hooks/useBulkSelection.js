import { useCallback, useEffect, useMemo, useState } from 'react';

function errorMessage(error, fallback) {
  const candidate = error?.response?.data?.error || error?.response?.data?.message || error?.message;
  return typeof candidate === 'string' && candidate.trim() ? candidate : fallback;
}

export function useBulkSelection({ items = [], deleteItem, getLabel, onDeleted, failureFallback }) {
  const [selectedIds, setSelectedIds] = useState(() => new Set());
  const [deleting, setDeleting] = useState(false);
  const [result, setResult] = useState(null);
  const visibleIds = useMemo(() => items.map((item) => item.id), [items]);
  const visibleKey = visibleIds.join('\u0000');

  useEffect(() => {
    const visible = new Set(visibleKey ? visibleKey.split('\u0000') : []);
    setSelectedIds((current) => new Set([...current].filter((id) => visible.has(id))));
    setResult((current) => {
      if (!current) return null;
      const failures = current.failures.filter((failure) => visible.has(failure.id));
      return failures.length ? { ...current, failures } : null;
    });
  }, [visibleKey]); // Selection must never silently include rows outside the visible result set.

  const toggle = useCallback((id) => {
    setResult(null);
    setSelectedIds((current) => {
      const next = new Set(current);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }, []);

  const allSelected = visibleIds.length > 0 && visibleIds.every((id) => selectedIds.has(id));
  const someSelected = visibleIds.some((id) => selectedIds.has(id));

  const toggleAll = useCallback(() => {
    setResult(null);
    setSelectedIds((current) => {
      if (visibleIds.length > 0 && visibleIds.every((id) => current.has(id))) return new Set();
      return new Set(visibleIds);
    });
  }, [visibleIds]);

  const removeSelected = useCallback(async () => {
    const selectedItems = items.filter((item) => selectedIds.has(item.id));
    if (!selectedItems.length || deleting) return null;

    setDeleting(true);
    setResult(null);
    const deletedIds = [];
    const failures = [];

    for (const item of selectedItems) {
      try {
        await deleteItem(item.id);
        deletedIds.push(item.id);
      } catch (error) {
        failures.push({ id: item.id, label: getLabel(item), message: errorMessage(error, failureFallback) });
      }
    }

    setSelectedIds(new Set(failures.map((failure) => failure.id)));
    const nextResult = { deletedCount: deletedIds.length, failures };
    setResult(nextResult);
    try {
      if (deletedIds.length) await onDeleted?.();
    } finally {
      setDeleting(false);
    }
    return nextResult;
  }, [deleteItem, deleting, failureFallback, getLabel, items, onDeleted, selectedIds]);

  return {
    selectedIds,
    selectedCount: selectedIds.size,
    allSelected,
    someSelected,
    deleting,
    result,
    toggle,
    toggleAll,
    removeSelected,
  };
}
