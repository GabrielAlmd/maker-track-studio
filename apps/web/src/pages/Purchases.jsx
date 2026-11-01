import PageHeader from '../components/ui/PageHeader';
import { EmptyState, InlineError, TableSkeleton } from '../components/ui/PageState';
import { useCallback, useState } from 'react';
import { Controller, useForm } from 'react-hook-form';
import { yupResolver } from '@hookform/resolvers/yup';
import * as yup from 'yup';
import toast from 'react-hot-toast';
import { Loader2, Plus, ShoppingCart } from 'lucide-react';
import { format } from 'date-fns';
import { useTranslation } from 'react-i18next';
import Modal from '../components/ui/Modal';
import PurchaseBatchModal from '../components/PurchaseBatchModal';
import { useApi } from '../hooks/useApi';
import { usePaginatedList } from '../hooks/usePaginatedList';
import ListToolbar from '../components/ui/ListToolbar';
import BulkActions, { RowSelectionCheckbox, SelectAllCheckbox } from '../components/ui/BulkActions';
import { useBulkSelection } from '../hooks/useBulkSelection';
import {
  getFilamentPurchases,
  createFilamentPurchase,
  updateFilamentPurchase,
  deleteFilamentPurchase,
} from '../services/filamentPurchaseService';
import { getFilaments } from '../services/filamentService';
import { VALUE_FALLBACKS, formatCurrency, formatDate, formatNumber } from '../utils/formatters';
import FilamentSelect from '../components/FilamentSelect';

const purchaseSchema = (t) => yup.object({
  filament_id: yup.string().required(t('validation.filamentRequired')),
  price: yup
    .number()
    .typeError(t('validation.priceNumber'))
    .positive(t('validation.pricePositive'))
    .required(t('validation.priceRequired')),
  amount: yup
    .number()
    .typeError(t('validation.amountNumber'))
    .positive(t('validation.amountPositive'))
    .required(t('validation.amountRequired')),
  purchase_date: yup.string().required(t('validation.purchaseDateRequired')),
});

function PurchaseModal({ purchase, filaments, onClose, onSaved }) {
  const { t } = useTranslation();
  const isEdit = !!purchase;
  const [submitError, setSubmitError] = useState(null);
  const {
    register,
    control,
    handleSubmit,
    formState: { errors, isSubmitting },
  } = useForm({
    resolver: yupResolver(purchaseSchema(t)),
    defaultValues: purchase
      ? {
          filament_id: purchase.filament_id,
          price: purchase.price,
          amount: purchase.amount,
          purchase_date: format(new Date(purchase.purchase_date), 'yyyy-MM-dd'),
        }
      : {},
  });

  const onSubmit = async (data) => {
    setSubmitError(null);
    try {
      const payload = {
        ...data,
        price: Number(data.price),
        amount: Number(data.amount),
        purchase_date: new Date(data.purchase_date).toISOString(),
      };
      if (isEdit) {
        await updateFilamentPurchase(purchase.id, payload);
        toast.success(t('purchases.updated'));
      } else {
        await createFilamentPurchase(payload);
        toast.success(t('purchases.saved'));
      }
      onSaved();
    } catch (error) {
      setSubmitError(error?.message || t('purchases.saveError'));
    }
  };

  return (
    <Modal title={t(isEdit ? 'purchases.edit' : 'purchases.new')} onClose={onClose}>
      <form onSubmit={handleSubmit(onSubmit)} className="space-y-4">
          {submitError && <p className="page-form-error" role="alert">{submitError}</p>}
          <div>
            <label className="block text-sm font-medium ui-text-secondary mb-1">{t('purchases.filament')}</label>
            <Controller
              name="filament_id"
              control={control}
              render={({ field }) => (
                <FilamentSelect
                  id="purchase-filament"
                  value={field.value}
                  onChange={field.onChange}
                  filaments={filaments}
                  aria-label={t('purchases.filament')}
                  invalid={!!errors.filament_id}
                />
              )}
            />
            {errors.filament_id && (
              <p className="ui-text-danger text-xs mt-1">{errors.filament_id.message}</p>
            )}
          </div>
          <div>
            <label className="block text-sm font-medium ui-text-secondary mb-1">{t('purchases.price')}</label>
            <input
              type="number"
              step="0.01"
              {...register('price')}
              className="w-full border ui-border-default rounded-md px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ui-focus-ring"
              placeholder="e.g. 24.99"
            />
            {errors.price && <p className="ui-text-danger text-xs mt-1">{errors.price.message}</p>}
          </div>
          <div>
            <label className="block text-sm font-medium ui-text-secondary mb-1">{t('purchases.amount')}</label>
            <input
              type="number"
              step="0.001"
              {...register('amount')}
              className="w-full border ui-border-default rounded-md px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ui-focus-ring"
              placeholder="e.g. 1"
            />
            {errors.amount && <p className="ui-text-danger text-xs mt-1">{errors.amount.message}</p>}
          </div>
          <div>
            <label className="block text-sm font-medium ui-text-secondary mb-1">{t('purchases.purchaseDate')}</label>
            <input
              type="date"
              {...register('purchase_date')}
              className="w-full border ui-border-default rounded-md px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ui-focus-ring"
            />
            {errors.purchase_date && (
              <p className="ui-text-danger text-xs mt-1">{errors.purchase_date.message}</p>
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
              disabled={isSubmitting}
              className="px-4 py-2 text-sm ui-text-inverse ui-bg-primary rounded-md hover:ui-bg-primary-hover disabled:opacity-50 flex items-center gap-2"
              aria-busy={isSubmitting}
            >
              {isSubmitting && <Loader2 className="h-4 w-4 animate-spin" />}
              {isSubmitting ? t('common.saving') : isEdit ? t('common.saveChanges') : t('purchases.record')}
            </button>
          </div>
        </form>
    </Modal>
  );
}

function Purchases() {
  const { t } = useTranslation();
  const [modalOpen, setModalOpen] = useState(false);
  const [editingPurchase, setEditingPurchase] = useState(null);
  const [deletingId, setDeletingId] = useState(null);

  const fetchPurchases = useCallback((params) => getFilamentPurchases(params), []);
  const fetchFilaments = useCallback(() => getFilaments(), []);

  const list = usePaginatedList(fetchPurchases, 'date');
  const { data: purchasePage, loading, error, refetch } = list;
  const purchases = purchasePage?.items;
  const { data: filaments, refetch: refetchFilaments } = useApi(fetchFilaments);

  const filamentMap = {};
  if (filaments) {
    filaments.forEach((f) => {
      filamentMap[f.id] = f;
    });
  }
  const bulk = useBulkSelection({
    items: purchases ?? [],
    deleteItem: deleteFilamentPurchase,
    getLabel: (purchase) => {
      const filament = purchase.filament || filamentMap[purchase.filament_id];
      return filament ? `${filament.brand} ${filament.type} — ${filament.color}` : purchase.id;
    },
    onDeleted: async () => { await refetch(); await refetchFilaments(); },
    failureFallback: t('bulk.failureFallback'),
  });

  const handleEdit = (purchase) => {
    setEditingPurchase(purchase);
    setModalOpen(true);
  };

  const handleAdd = () => {
    setEditingPurchase(null);
    setModalOpen(true);
  };

  const handleModalClose = () => {
    setModalOpen(false);
    setEditingPurchase(null);
  };

  const handleSaved = () => {
    handleModalClose();
    refetch();
    refetchFilaments();
  };

  const handleDelete = async (id) => {
    if (!window.confirm(t('purchases.confirmDelete'))) return;
    setDeletingId(id);
    try {
      await deleteFilamentPurchase(id);
      toast.success(t('purchases.deleted'));
      refetch();
    } catch {
      // toasted by interceptor
    } finally {
      setDeletingId(null);
    }
  };

  return (
    <div>
      {modalOpen && !editingPurchase && (
        <PurchaseBatchModal filaments={filaments} onClose={handleModalClose} onSaved={handleSaved} />
      )}
      {modalOpen && editingPurchase && (
        <PurchaseModal
          purchase={editingPurchase}
          filaments={filaments}
          onClose={handleModalClose}
          onSaved={handleSaved}
        />
      )}

      <PageHeader title={t('purchases.title')} description={t('purchases.description')}>
        <button
          onClick={handleAdd}
          className="page-action"
        >
          <Plus className="h-4 w-4" />
          {t('purchases.record')}
        </button>
      </PageHeader>

      <div className="page-panel">
        <ListToolbar list={list} sorts={[{ value: 'date', label: t('purchases.purchaseDate') }, { value: 'price', label: t('purchases.priceHeading') }, { value: 'brand', label: t('purchases.brand') }, { value: 'type', label: t('filaments.type') }]}>
          <div className="space-y-3 text-sm">
            <label className="block">{t('purchases.from')}<input type="date" value={list.filters.date_from || ''} onChange={(e) => list.changeFilters({ ...list.filters, date_from: e.target.value })} className="mt-1 w-full rounded border p-2" /></label>
            <label className="block">{t('purchases.to')}<input type="date" value={list.filters.date_to || ''} onChange={(e) => list.changeFilters({ ...list.filters, date_to: e.target.value })} className="mt-1 w-full rounded border p-2" /></label>
            <div className="block"><span>{t('purchases.brand')}</span><input aria-label={t('purchases.brand')} value={list.filters.brand || ''} onChange={(e) => list.changeFilters({ ...list.filters, brand: e.target.value })} className="mt-1 w-full rounded border p-2" /></div>
            <div className="block"><span>{t('purchases.filamentType')}</span><input aria-label={t('purchases.filamentType')} value={list.filters.type || ''} onChange={(e) => list.changeFilters({ ...list.filters, type: e.target.value })} className="mt-1 w-full rounded border p-2" /></div>
            <label className="block">{t('purchases.minPrice')}<input type="number" min="0" value={list.filters.min_price || ''} onChange={(e) => list.changeFilters({ ...list.filters, min_price: e.target.value })} className="mt-1 w-full rounded border p-2" /></label>
          </div>
        </ListToolbar>
        <div className="px-4 pt-4"><BulkActions bulk={bulk} /></div>
        {error && <div className="p-4"><InlineError title={t('purchases.loadError')} message={purchases ? t('common.existingDataSafe') : error} onRetry={refetch} /></div>}
        {loading && !purchases ? <TableSkeleton columns={6} label={t('purchases.loading')} /> : !purchases || purchases.length === 0 ? (
          !error && <EmptyState icon={ShoppingCart} title={list.hasFilters ? t('common.noResults') : t('common.noRecords')} description={list.hasFilters ? t('common.changeSearch') : t('purchases.emptyHelp')} action={list.hasFilters ? <button onClick={list.clear} className="page-action">{t('common.clearFilters')}</button> : <button onClick={handleAdd} className="page-action">{t('purchases.recordFirst')}</button>} />
        ) : (
          <div className="overflow-x-auto">
            <table className="purchases-table min-w-full divide-y ui-divide-subtle">
              <thead className="sticky top-0 z-10 ui-surface-nested">
                <tr>
                  <th scope="col" className="w-12 px-6 py-3"><SelectAllCheckbox bulk={bulk} /></th>
                  {[t('purchases.filament'), t('purchases.priceHeading'), t('purchases.amountHeading'), t('purchases.dateHeading'), t('common.actions')].map((h) => (
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
                {purchases.map((p) => {
                  const fil = p.filament || filamentMap[p.filament_id];
                  return (
                    <tr key={p.id} className="hover:ui-surface-hover">
                      <td className="w-12 px-6 py-4"><RowSelectionCheckbox bulk={bulk} id={p.id} label={fil ? `${fil.brand} ${fil.type} — ${fil.color}` : p.id} /></td>
                      <td className="px-6 py-4 whitespace-nowrap text-sm ui-text-primary">
                        {fil ? (
                          <span>
                            {fil.brand}{' '}
                            <span className="ui-text-muted">
                              {fil.type} — {fil.color}
                            </span>
                          </span>
                        ) : (
                          <span className="ui-text-muted italic">{t('common.unknown')}</span>
                        )}
                      </td>
                      <td className="px-6 py-4 whitespace-nowrap text-sm ui-text-secondary">
                        {formatCurrency(p.price)}
                      </td>
                      <td className="px-6 py-4 whitespace-nowrap text-sm ui-text-secondary">
                        {formatNumber(p.amount, { maximumFractionDigits: 3, fallback: VALUE_FALLBACKS.dataUnavailable })} kg
                      </td>
                      <td className="px-6 py-4 whitespace-nowrap text-sm ui-text-secondary">
                        {formatDate(p.purchase_date)}
                      </td>
                      <td className="px-6 py-4 whitespace-nowrap text-sm font-medium">
                        <button
                          onClick={() => handleEdit(p)}
                          className="ui-text-primary-action hover:ui-text-primary-action mr-3"
                        >
                          {t('common.edit')}
                        </button>
                        <button
                          onClick={() => handleDelete(p.id)}
                          disabled={deletingId === p.id}
                          className="ui-text-danger hover:ui-text-danger disabled:opacity-50"
                        >
                          {t(deletingId === p.id ? 'common.deleting' : 'common.delete')}
                        </button>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </div>
  );
}

export default Purchases;
