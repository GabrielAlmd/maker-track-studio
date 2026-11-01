import { useEffect, useRef } from 'react';
import { Trash2 } from 'lucide-react';
import toast from 'react-hot-toast';
import { useTranslation } from 'react-i18next';

export function SelectAllCheckbox({ bulk }) {
  const { t } = useTranslation();
  const ref = useRef(null);
  useEffect(() => {
    if (ref.current) ref.current.indeterminate = bulk.someSelected && !bulk.allSelected;
  }, [bulk.allSelected, bulk.someSelected]);
  return <input ref={ref} type="checkbox" checked={bulk.allSelected} onChange={bulk.toggleAll} aria-label={t('bulk.selectAllVisible')} className="h-4 w-4 rounded ui-border-default" />;
}

export function RowSelectionCheckbox({ bulk, id, label }) {
  const { t } = useTranslation();
  return <input type="checkbox" checked={bulk.selectedIds.has(id)} onChange={() => bulk.toggle(id)} aria-label={t('bulk.selectRecord', { label })} className="h-4 w-4 rounded ui-border-default" />;
}

export default function BulkActions({ bulk }) {
  const { t } = useTranslation();
  if (!bulk.selectedCount) return null;

  const confirmDelete = async () => {
    if (!window.confirm(t('bulk.confirmDelete', { count: bulk.selectedCount }))) return;
    const result = await bulk.removeSelected();
    if (!result) return;
    if (result.deletedCount) toast.success(t('bulk.deleted', { count: result.deletedCount }));
  };

  return <section className="mb-4 rounded-xl border ui-border-default ui-surface-card p-4" aria-label={t('bulk.actions')}>
    <div className="flex flex-wrap items-center justify-between gap-3">
      <p className="text-sm font-medium ui-text-primary" aria-live="polite">{t('bulk.selected', { count: bulk.selectedCount })}</p>
      <button type="button" onClick={confirmDelete} disabled={bulk.deleting} className="button-danger inline-flex items-center gap-2 rounded-md px-4 py-2 text-sm disabled:opacity-50">
        <Trash2 className="h-4 w-4" aria-hidden="true" />
        {t(bulk.deleting ? 'bulk.deleting' : 'bulk.deleteSelected')}
      </button>
    </div>
    {bulk.result?.failures.length > 0 && <div className="mt-3 text-sm ui-text-danger" role="alert">
      <p className="font-semibold">{t('bulk.failed', { count: bulk.result.failures.length })}</p>
      <ul className="mt-1 list-disc pl-5">
        {bulk.result.failures.map((failure) => <li key={failure.id}><span className="font-medium">{failure.label}</span>: {failure.message}</li>)}
      </ul>
    </div>}
  </section>;
}
