import PageHeader from '../components/ui/PageHeader';
import { useCallback, useState } from 'react';
import toast from 'react-hot-toast';
import { Plus, Boxes } from 'lucide-react';
import ModelModal from '../components/ModelModal';
import { EmptyState, InlineError, TableSkeleton } from '../components/ui/PageState';
import { usePaginatedList } from '../hooks/usePaginatedList';
import ListToolbar from '../components/ui/ListToolbar';
import BulkActions, { RowSelectionCheckbox, SelectAllCheckbox } from '../components/ui/BulkActions';
import { getModels, deleteModel } from '../services/modelService';
import { useBulkSelection } from '../hooks/useBulkSelection';
import { VALUE_FALLBACKS, formatDuration, formatNumber, formatWeight } from '../utils/formatters';
import { useTranslation } from 'react-i18next';


function Models() {
  const { t } = useTranslation();
  const [modalOpen, setModalOpen] = useState(false);
  const [editingModel, setEditingModel] = useState(null);
  const [deletingId, setDeletingId] = useState(null);

  const fetchModels = useCallback((params) => getModels(params), []);
  const list = usePaginatedList(fetchModels);
  const { data: modelPage, loading, error, refetch } = list;
  const models = modelPage?.items;
  const bulk = useBulkSelection({
    items: models ?? [],
    deleteItem: deleteModel,
    getLabel: (model) => model.name,
    onDeleted: refetch,
    failureFallback: t('bulk.failureFallback'),
  });

  const handleEdit = (model) => {
    setEditingModel(model);
    setModalOpen(true);
  };

  const handleAdd = () => {
    setEditingModel(null);
    setModalOpen(true);
  };

  const handleModalClose = () => {
    setModalOpen(false);
    setEditingModel(null);
  };

  const handleSaved = () => {
    handleModalClose();
    refetch();
  };

  const handleDelete = async (id) => {
    if (!window.confirm(t('models.confirmDelete'))) return;
    setDeletingId(id);
    try {
      await deleteModel(id);
      toast.success(t('models.deleted'));
      refetch();
    } catch {
      // toasted by interceptor
    } finally {
      setDeletingId(null);
    }
  };

  return (
    <div>
      {modalOpen && (
        <ModelModal model={editingModel} onClose={handleModalClose} onSaved={handleSaved} />
      )}

      <PageHeader title={t('models.title')} description={t('models.description')}>
        <button
          onClick={handleAdd}
          className="page-action"
        >
          <Plus className="h-4 w-4" />
          {t('models.add')}
        </button>
      </PageHeader>

      <div className="page-panel">
        <ListToolbar list={list} sorts={[{ value: 'name', label: t('models.name') }, { value: 'print_time', label: t('models.printTime') }, { value: 'created_at', label: t('models.dateAdded') }]}>
          <div className="space-y-3 text-sm">
            <label className="block">{t('models.multicolor')}<select value={list.filters.multicolor || ''} onChange={(e) => list.changeFilters({ ...list.filters, multicolor: e.target.value })} className="mt-1 w-full rounded border p-2"><option value="">{t('common.all')}</option><option value="true">{t('common.yes')}</option><option value="false">{t('common.no')}</option></select></label>
            <label className="block">{t('models.printJobs')}<select value={list.filters.print_jobs || ''} onChange={(e) => list.changeFilters({ ...list.filters, print_jobs: e.target.value })} className="mt-1 w-full rounded border p-2"><option value="">{t('common.all')}</option><option value="has">{t('models.hasJobs')}</option><option value="none">{t('models.noJobs')}</option></select></label>
            <label className="block">{t('models.printTimeRange')}<div className="mt-1 flex gap-2"><input aria-label={t('models.minimumPrintTime')} type="number" min="0" placeholder="Min" value={list.filters.min_print_time || ''} onChange={(e) => list.changeFilters({ ...list.filters, min_print_time: e.target.value })} className="w-full rounded border p-2" /><input aria-label={t('models.maximumPrintTime')} type="number" min="0" placeholder="Max" value={list.filters.max_print_time || ''} onChange={(e) => list.changeFilters({ ...list.filters, max_print_time: e.target.value })} className="w-full rounded border p-2" /></div></label>
            <label className="block">{t('models.materialRange')}<div className="mt-1 flex gap-2"><input aria-label={t('models.minimumMaterial')} type="number" min="0" step="0.01" placeholder="Min" value={list.filters.min_material || ''} onChange={(e) => list.changeFilters({ ...list.filters, min_material: e.target.value })} className="w-full rounded border p-2" /><input aria-label={t('models.maximumMaterial')} type="number" min="0" step="0.01" placeholder="Max" value={list.filters.max_material || ''} onChange={(e) => list.changeFilters({ ...list.filters, max_material: e.target.value })} className="w-full rounded border p-2" /></div></label>
            <label className="block">{t('models.wasteRange')}<div className="mt-1 flex gap-2"><input aria-label={t('models.minimumWaste')} type="number" min="0" step="0.01" placeholder="Min" value={list.filters.min_waste || ''} onChange={(e) => list.changeFilters({ ...list.filters, min_waste: e.target.value })} className="w-full rounded border p-2" /><input aria-label={t('models.maximumWaste')} type="number" min="0" step="0.01" placeholder="Max" value={list.filters.max_waste || ''} onChange={(e) => list.changeFilters({ ...list.filters, max_waste: e.target.value })} className="w-full rounded border p-2" /></div></label>
          </div>
        </ListToolbar>
        <div className="px-4 pt-4"><BulkActions bulk={bulk} /></div>
        {error && <div className="p-4"><InlineError title={t('models.loadError')} message={models ? t('common.existingDataSafe') : error} onRetry={refetch} /></div>}
        {loading && !models ? <TableSkeleton columns={8} label={t('models.loading')} /> : !models || models.length === 0 ? (
          !error && <EmptyState icon={Boxes} title={list.hasFilters ? t('common.noResults') : t('common.noRecords')} description={list.hasFilters ? t('common.changeSearch') : t('models.emptyHelp')} action={list.hasFilters ? <button onClick={list.clear} className="page-action">{t('common.clearFilters')}</button> : <button onClick={handleAdd} className="page-action">{t('models.addFirst')}</button>} />
        ) : (
          <div className="overflow-x-auto">
            <table className="models-table min-w-full divide-y ui-divide-subtle">
              <thead className="sticky top-0 z-10 ui-surface-nested">
                <tr>
                  <th scope="col" className="w-12 px-6 py-3"><SelectAllCheckbox bulk={bulk} /></th>
                  {[
                    t('models.name'),
                    t('models.material'),
                    t('models.printTimeHeading'),
                    t('models.waste'),
                    t('models.multicolor'),
                    t('models.printJobsHeading'),
                    t('common.actions'),
                  ].map((h) => (
                    <th
                      key={h}
                      className="px-6 py-3 text-left text-xs font-medium ui-text-muted uppercase tracking-wider"
                    >
                      {h}
                    </th>
                  ))}
                </tr>
              </thead>
              <tbody className="ui-surface-card divide-y ui-divide-subtle">
                {models.map((model) => (
                  <tr key={model.id} className="hover:ui-surface-hover">
                    <td className="w-12 px-6 py-4"><RowSelectionCheckbox bulk={bulk} id={model.id} label={model.name} /></td>
                    <td className="px-6 py-4 whitespace-nowrap">
                      <div className="text-sm font-medium ui-text-primary">{model.name}</div>
                    </td>
                    <td className="px-6 py-4 whitespace-nowrap text-sm ui-text-secondary">
                      {formatWeight(model.material_weight, { fallback: VALUE_FALLBACKS.dataUnavailable })}
                    </td>
                    <td className="px-6 py-4 whitespace-nowrap text-sm ui-text-secondary">
                      {formatDuration(model.print_time, { fallback: VALUE_FALLBACKS.notRecorded })}
                    </td>
                    <td className="px-6 py-4 whitespace-nowrap text-sm ui-text-secondary">
                      {formatWeight(model.waste_material, { fallback: VALUE_FALLBACKS.dataUnavailable })}
                    </td>
                    <td className="px-6 py-4 whitespace-nowrap">
                      <span
                        className={`px-2 inline-flex text-xs leading-5 font-semibold rounded-full ${
                          model.is_multicolor
                            ? 'bg-purple-100 text-purple-800'
                            : 'ui-surface-input ui-text-secondary'
                        }`}
                      >
                        {model.is_multicolor ? t('common.yes') : t('common.no')}
                      </span>
                    </td>
                    <td className="px-6 py-4 whitespace-nowrap text-sm ui-text-secondary">
                      {formatNumber(model.print_job_count, { fallback: VALUE_FALLBACKS.unknown })}
                    </td>
                    <td className="px-6 py-4 whitespace-nowrap text-sm font-medium">
                      <button
                        onClick={() => handleEdit(model)}
                        className="ui-text-primary-action hover:ui-text-primary-action mr-3"
                      >
                        {t('common.edit')}
                      </button>
                      <button
                        onClick={() => handleDelete(model.id)}
                        disabled={deletingId === model.id}
                        className="ui-text-danger hover:ui-text-danger disabled:opacity-50"
                      >
                        {t(deletingId === model.id ? 'common.deleting' : 'common.delete')}
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </div>
  );
}

export default Models;
