import PageHeader from '../components/ui/PageHeader';
import { useCallback, useState } from 'react';
import { Controller, useForm } from 'react-hook-form';
import { yupResolver } from '@hookform/resolvers/yup';
import * as yup from 'yup';
import toast from 'react-hot-toast';
import { Loader2, Plus, Package, Star } from 'lucide-react';
import Modal from '../components/ui/Modal';
import { useApi } from '../hooks/useApi';
import { usePaginatedList } from '../hooks/usePaginatedList';
import ListToolbar from '../components/ui/ListToolbar';
import BulkActions, { RowSelectionCheckbox, SelectAllCheckbox } from '../components/ui/BulkActions';
import { useBulkSelection } from '../hooks/useBulkSelection';
import {
  getFilaments,
  getFilamentBrands,
  createFilament,
  updateFilament,
  deleteFilament,
} from '../services/filamentService';

import { getFilamentTypes } from '../services/filamentTypeService';
import FilamentTypesManager from '../components/FilamentTypesManager';
import FilamentTypeOptions from '../components/FilamentTypeOptions';
import FilamentTypeCatalog from '../components/FilamentTypeCatalog';
import { EmptyState, InlineError, TableSkeleton } from '../components/ui/PageState';
import { VALUE_FALLBACKS, formatCurrency, formatNumber, toFiniteNumber } from '../utils/formatters';
import { useTranslation } from 'react-i18next';
import BrandSelect from '../components/BrandSelect';

const filamentSchema = (t) => yup.object({
  brand: yup.string().required(t('validation.brandRequired')),
  type: yup.string().required(t('validation.typeRequired')),
  color: yup.string().required(t('validation.colorRequired')),
  avg_price: yup
    .number()
    .typeError(t('validation.priceNumber'))
    .positive(t('validation.pricePositive'))
    .required(t('validation.priceRequired')),
  personal_score: yup
    .number()
    .typeError(t('validation.scoreNumber'))
    .min(1)
    .max(5)
    .integer()
    .required(t('validation.scoreRequired')),
});

function FilamentModal({ filament, brands, catalog, onClose, onSaved }) {
  const { t } = useTranslation();
  const [managingTypes, setManagingTypes] = useState(false);
  const [submitError, setSubmitError] = useState(null);
  const { data: types, loading: typesLoading, error: typesError, refetch: refetchTypes } = catalog;
  const isEdit = !!filament;
  const {
    register,
    control,
    watch,
    handleSubmit,
    formState: { errors, isSubmitting },
  } = useForm({
    resolver: yupResolver(filamentSchema(t)),
    defaultValues: filament
      ? {
          brand: filament.brand,
          type: filament.type,
          color: filament.color,
          avg_price: filament.avg_price,
          personal_score: filament.personal_score,
        }
      : { personal_score: 3 },
  });

  const onSubmit = async (data) => {
    setSubmitError(null);
    try {
      if (isEdit) {
        await updateFilament(filament.id, data);
        toast.success(t('filaments.updated'));
      } else {
        await createFilament(data);
        toast.success(t('filaments.created'));
      }
      onSaved();
    } catch (error) {
      setSubmitError(error?.message || t('filaments.saveError'));
    }
  };

  if (managingTypes) return <FilamentTypesManager types={types || []} onClose={() => setManagingTypes(false)} onSaved={() => { setManagingTypes(false); refetchTypes(); }} />;

  return (
    <Modal title={t(isEdit ? 'filaments.editTitle' : 'filaments.addTitle')} onClose={onClose}>
      <form onSubmit={handleSubmit(onSubmit)} className="space-y-4">
          {submitError && <p className="page-form-error" role="alert">{submitError}</p>}
          <div>
            <label htmlFor="filament-brand" className="block text-sm font-medium ui-text-secondary mb-1">{t('filaments.brand')}</label>
            <Controller name="brand" control={control} render={({ field }) => (
              <BrandSelect id="filament-brand" {...field} brands={brands} invalid={!!errors.brand} />
            )} />
            {errors.brand && <p className="ui-text-danger text-xs mt-1">{errors.brand.message}</p>}
          </div>
          <div>
            <label htmlFor="filament-type" className="block text-sm font-medium ui-text-secondary mb-1">{t('filaments.type')}</label>
            <select
              id="filament-type"
              {...register('type')}
              disabled={typesLoading || !!typesError}
              className="w-full border ui-border-default rounded-md px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ui-focus-ring"
            >
              <FilamentTypeOptions types={types || []} current={watch('type')} />
            </select>
            <FilamentTypeCatalog loading={typesLoading} error={typesError} onRetry={refetchTypes} onManage={() => setManagingTypes(true)} />
            {errors.type && <p className="ui-text-danger text-xs mt-1">{errors.type.message}</p>}
          </div>
          <div>
            <label htmlFor="filament-color" className="block text-sm font-medium ui-text-secondary mb-1">{t('filaments.color')}</label>
            <input
              id="filament-color"
              {...register('color')}
              className="w-full border ui-border-default rounded-md px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ui-focus-ring"
              placeholder="e.g. Black"
            />
            {errors.color && <p className="ui-text-danger text-xs mt-1">{errors.color.message}</p>}
          </div>
          <div>
            <label htmlFor="filament-average-price" className="block text-sm font-medium ui-text-secondary mb-1">
              {t('filaments.averagePriceLong')}
            </label>
            <input
              id="filament-average-price"
              type="number"
              min="0.01"
              max={Number.MAX_SAFE_INTEGER}
              step="0.01"
              {...register('avg_price')}
              className="w-full border ui-border-default rounded-md px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ui-focus-ring"
              placeholder="e.g. 22.99"
            />
            {errors.avg_price && (
              <p className="ui-text-danger text-xs mt-1">{errors.avg_price.message}</p>
            )}
          </div>
          <div>
            <label htmlFor="filament-personal-score" className="block text-sm font-medium ui-text-secondary mb-1">
              {t('filaments.personalScore')}
            </label>
            <input
              id="filament-personal-score"
              type="number"
              min="1"
              max="5"
              step="1"
              {...register('personal_score')}
              className="w-full border ui-border-default rounded-md px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ui-focus-ring"
            />
            {errors.personal_score && (
              <p className="ui-text-danger text-xs mt-1">{errors.personal_score.message}</p>
            )}
          </div>
          <div className="flex justify-end gap-3 pt-2">
            <button
              type="button"
              onClick={onClose}
              className="px-4 py-2 text-sm ui-text-secondary border ui-border-default rounded-md hover:ui-surface-hover"
            >
              {t('common.cancel')}
            </button>
            <button
              type="submit"
              disabled={isSubmitting || typesLoading || !!typesError}
              className="px-4 py-2 text-sm ui-text-inverse ui-bg-primary rounded-md hover:ui-bg-primary-hover disabled:opacity-50 flex items-center gap-2"
              aria-busy={isSubmitting}
            >
              {isSubmitting && <Loader2 className="h-4 w-4 animate-spin" />}
              {isSubmitting ? t('common.saving') : isEdit ? t('common.saveChanges') : t('filaments.create')}
            </button>
          </div>
        </form>
    </Modal>
  );
}

function ScoreStars({ score }) {
  const { t } = useTranslation();
  const scoreValue = toFiniteNumber(score);
  const scoreLabel = formatNumber(score, { fallback: VALUE_FALLBACKS.dataUnavailable });
  return (
    <div className="flex gap-0.5" role="img" aria-label={t('filaments.stars', { score: scoreLabel })}>
      {[1, 2, 3, 4, 5].map((s) => (
        <Star
          key={s}
          className={`h-4 w-4 ${scoreValue !== null && s <= scoreValue ? 'ui-text-warning ui-fill-warning' : 'ui-text-muted'}`}
        />
      ))}
    </div>
  );
}

function groupFilamentsByBrand(filaments, direction = 'asc', unknownBrand = 'Unknown brand') {
  const groups = new Map();

  for (const filament of filaments) {
    const brand = filament.brand?.trim() || unknownBrand;
    const key = brand.toLowerCase();
    if (!groups.has(key)) groups.set(key, { brand, items: [] });
    groups.get(key).items.push(filament);
  }

  return [...groups.values()].sort((a, b) => {
    const comparison = a.brand.localeCompare(b.brand, undefined, { sensitivity: 'base' });
    return direction === 'desc' ? -comparison : comparison;
  });
}

function Filaments() {
  const { t } = useTranslation();
  const [managingTypes, setManagingTypes] = useState(false);
  const catalog = useApi(getFilamentTypes);
  const brandCatalog = useApi(getFilamentBrands);
  const { data: types, loading: typesLoading, error: typesError, refetch: refetchTypes } = catalog;
  const [modalOpen, setModalOpen] = useState(false);
  const [editingFilament, setEditingFilament] = useState(null);
  const [deletingId, setDeletingId] = useState(null);
  const [collapsedBrands, setCollapsedBrands] = useState({});

  const fetchFilaments = useCallback((params) => getFilaments(params), []);
  const list = usePaginatedList(fetchFilaments, 'brand');
  const { data: filamentPage, loading, error, refetch } = list;
  const filaments = filamentPage?.items;
  const filamentGroups = groupFilamentsByBrand(filaments ?? [], list.sort === 'brand' ? list.direction : 'asc', t('filaments.unknownBrand'));
  const selectableFilaments = filamentGroups.flatMap(({ brand, items }) => collapsedBrands[brand] ? [] : items);
  const bulk = useBulkSelection({
    items: selectableFilaments,
    deleteItem: deleteFilament,
    getLabel: (filament) => `${filament.brand} ${filament.type} — ${filament.color}`,
    onDeleted: refetch,
    failureFallback: t('bulk.failureFallback'),
  });

  const handleEdit = (filament) => {
    setEditingFilament(filament);
    setModalOpen(true);
  };

  const handleAdd = () => {
    setEditingFilament(null);
    setModalOpen(true);
  };

  const handleModalClose = () => {
    setModalOpen(false);
    setEditingFilament(null);
  };

  const handleSaved = () => {
    handleModalClose();
    refetch();
    brandCatalog.refetch();
  };

  const handleDelete = async (id) => {
    if (!window.confirm(t('filaments.confirmDelete'))) return;
    setDeletingId(id);
    try {
      await deleteFilament(id);
      toast.success(t('filaments.deleted'));
      refetch();
    } catch {
      // toasted by interceptor
    } finally {
      setDeletingId(null);
    }
  };

  return (
    <div>
      {managingTypes && <FilamentTypesManager types={types || []} onClose={() => setManagingTypes(false)} onSaved={() => { setManagingTypes(false); refetchTypes(); }} />}
      {modalOpen && (
        <FilamentModal
          filament={editingFilament}
          brands={brandCatalog.data || []}
          catalog={catalog}
          onClose={handleModalClose}
          onSaved={handleSaved}
        />
      )}

      <PageHeader title={t('filaments.title')} description={t('filaments.description')}>
        <button
          onClick={handleAdd}
          className="page-action"
        >
          <Plus className="h-4 w-4" />
          {t('filaments.add')}
        </button>
      </PageHeader>

      <div className="mb-6"><FilamentTypeCatalog loading={typesLoading} error={typesError} onRetry={refetchTypes} onManage={() => setManagingTypes(true)} /></div>
      <div>
        <div className="list-toolbar-panel mb-4"><ListToolbar list={list} sorts={[{ value: 'brand', label: t('filaments.brand') }, { value: 'type', label: t('filaments.type') }, { value: 'rating', label: t('filaments.rating') }, { value: 'price', label: t('purchases.priceHeading') }]}>
          <div className="space-y-3 text-sm">
            <div className="block"><span>{t('filaments.brand')}</span><input aria-label={t('filaments.brand')} value={list.filters.brand || ''} onChange={(e) => list.changeFilters({ ...list.filters, brand: e.target.value })} className="mt-1 w-full rounded border p-2" /></div>
            <div className="block"><span>{t('filaments.type')}</span><input aria-label={t('filaments.type')} value={list.filters.type || ''} onChange={(e) => list.changeFilters({ ...list.filters, type: e.target.value })} className="mt-1 w-full rounded border p-2" /></div>
            <label className="block">{t('filaments.rating')}<select role="listbox" value={list.filters.rating || ''} onChange={(e) => list.changeFilters({ ...list.filters, rating: e.target.value })} className="mt-1 w-full rounded border p-2"><option value="">{t('common.all')}</option>{[1,2,3,4,5].map((n) => <option key={n}>{n}</option>)}</select></label>
            <label className="flex gap-2"><input type="checkbox" checked={list.filters.low_stock === 'true'} onChange={(e) => list.changeFilters({ ...list.filters, low_stock: e.target.checked ? 'true' : '' })} />{t('filaments.lowStock')}</label>
          </div>
        </ListToolbar></div>
        <BulkActions bulk={bulk} />
        {error && <InlineError title={t('filaments.loadError')} message={filaments ? t('common.existingDataSafe') : error} onRetry={refetch} />}
        {loading && !filaments ? <div className="page-panel"><TableSkeleton columns={7} label={t('filaments.loading')} /></div> : !filaments || filaments.length === 0 ? (
          !error && <div className="page-panel"><EmptyState icon={Package} title={list.hasFilters ? t('common.noResults') : t('common.noRecords')} description={list.hasFilters ? t('common.changeSearch') : t('filaments.emptyHelp')} action={list.hasFilters ? <button onClick={list.clear} className="page-action">{t('common.clearFilters')}</button> : <button onClick={handleAdd} className="page-action">{t('filaments.addFirst')}</button>} /></div>
        ) : (
          <div className="space-y-8">
            {filamentGroups.map(({ brand, items }, index) => (
              <section key={brand} aria-labelledby={`filament-brand-${index}`}>
                <div className="mb-4 flex items-center gap-3 border-b ui-border-subtle pb-3">
                  <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-xl ui-tone-primary ui-text-primary-action">
                    <Package className="h-5 w-5" aria-hidden="true" />
                  </div>
                  <button type="button" onClick={() => setCollapsedBrands((current) => ({ ...current, [brand]: !current[brand] }))} aria-expanded={!collapsedBrands[brand]} className="text-lg font-semibold ui-text-primary">
                    {brand}
                  </button>
                  <span className="rounded-full ui-surface-input px-3 py-1 text-xs font-medium ui-text-secondary">
                    {t('filaments.count', { count: items.length })}
                  </span>
                </div>
                {!collapsedBrands[brand] && <div className="page-panel overflow-x-auto">
                  <table className="filaments-table w-full text-left text-sm" aria-labelledby={`filament-brand-${index}`}>
                    <thead className="sticky top-0 z-10 border-b ui-border-subtle ui-surface-nested text-xs ui-text-muted">
                      <tr>
                        <th scope="col" className="w-12 px-5 py-3"><SelectAllCheckbox bulk={bulk} /></th>
                        {[t('filaments.type'), t('filaments.color'), t('filaments.averagePrice'), t('filaments.yourRating'), t('filaments.timesUsed'), t('common.actions')].map((label) => (
                          <th key={label} scope="col" className="whitespace-nowrap px-5 py-3 font-medium">{label}</th>
                        ))}
                      </tr>
                    </thead>
                    <tbody className="divide-y ui-divide-subtle">
                      {items.map((f) => (
                        <tr key={f.id} className="hover:ui-surface-hover">
                          <td className="w-12 px-5 py-3"><RowSelectionCheckbox bulk={bulk} id={f.id} label={`${brand} ${f.type} — ${f.color}`} /></td>
                          <td className="w-28 whitespace-nowrap px-5 py-3">
                            <span className="inline-flex rounded-full ui-tone-primary px-2.5 py-1 text-xs font-semibold ui-text-primary-action">
                              {f.type}
                            </span>
                          </td>
                          <th scope="row" className="min-w-36 px-5 py-3 font-semibold ui-text-primary">{f.color}</th>
                          <td className="w-40 whitespace-nowrap px-5 py-3 font-semibold tabular-nums ui-text-primary">
                            {Array.isArray(f.purchases) && f.purchases.length === 0
                              ? VALUE_FALLBACKS.noPurchaseData
                              : formatCurrency(f.avg_price)}
                          </td>
                          <td className="w-36 px-5 py-3"><ScoreStars score={f.personal_score} /></td>
                          <td className="w-28 px-5 py-3 tabular-nums ui-text-secondary">{formatNumber(f.times_used, { fallback: VALUE_FALLBACKS.unknown })}</td>
                          <td className="w-44 whitespace-nowrap px-5 py-3">
                            <div className="flex items-center gap-3">
                              <button
                                onClick={() => handleEdit(f)}
                                aria-label={`${t('common.edit')} ${brand} ${f.type} ${f.color}`}
                                className="rounded-md border ui-border-default ui-surface-card px-3 py-2 text-sm font-medium ui-text-secondary hover:ui-surface-hover focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-blue-600"
                              >
                                {t('common.edit')}
                              </button>
                              <button
                                onClick={() => handleDelete(f.id)}
                                aria-label={`${t('common.delete')} ${brand} ${f.type} ${f.color}`}
                                disabled={deletingId === f.id}
                                className="rounded-md px-3 py-2 text-sm font-medium ui-text-danger hover:ui-tone-danger focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-red-600 disabled:opacity-50"
                              >
                                {t(deletingId === f.id ? 'common.deleting' : 'common.delete')}
                              </button>
                            </div>
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>}
              </section>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}

export default Filaments;
