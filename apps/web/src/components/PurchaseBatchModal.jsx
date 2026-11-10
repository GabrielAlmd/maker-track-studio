import { useState } from 'react';
import { useApi } from '../hooks/useApi';
import { getFilamentTypes } from '../services/filamentTypeService';
import FilamentTypesManager from './FilamentTypesManager';
import FilamentTypeOptions from './FilamentTypeOptions';
import FilamentTypeCatalog from './FilamentTypeCatalog';
import { Controller, useFieldArray, useForm } from 'react-hook-form';
import { yupResolver } from '@hookform/resolvers/yup';
import * as yup from 'yup';
import toast from 'react-hot-toast';
import { Loader2, Plus } from 'lucide-react';
import Modal from './ui/Modal';
import { InlineError } from './ui/PageState';
import { createFilamentPurchaseBatch } from '../services/filamentPurchaseService';
import { VALUE_FALLBACKS, formatCurrency, toFiniteNumber } from '../utils/formatters';
import { useTranslation } from 'react-i18next';
import FilamentSelect from './FilamentSelect';
import BrandSelect from './BrandSelect';
import { getFilamentBrands } from '../services/filamentService';

const newField = (label, t) => yup.string().when('is_new', {
  is: true,
  then: (schema) => schema.trim().required(t('validation.fieldRequired', { field: label })).max(200),
  otherwise: (schema) => schema.strip(),
});
const batchSchema = (t) => yup.object({
  purchase_date: yup.string().required(t('validation.purchaseDateRequired')),
  items: yup.array().of(yup.object({
    filament_id: yup.string().when('is_new', {
      is: true,
      then: (field) => field.strip(),
      otherwise: (field) => field.required(t('validation.filamentRequired')),
    }),
    is_new: yup.boolean().required(),
    brand: newField(t('filaments.brand'), t),
    type: newField(t('filaments.type'), t),
    color: newField(t('filaments.color'), t),
    price: yup.number().typeError(t('validation.priceNumber')).positive(t('validation.pricePositive')).max(Number.MAX_SAFE_INTEGER).required(),
    amount: yup.number().typeError(t('validation.amountNumber')).positive(t('validation.amountPositive')).max(Number.MAX_SAFE_INTEGER).required(),
  })).min(1).max(100),
});
const emptyLine = () => ({ filament_id: '', is_new: false, price: '', amount: '', brand: '', type: '', color: '' });
const inputClass = 'w-full border ui-border-default rounded-md px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ui-focus-ring';

export default function PurchaseBatchModal({ filaments, filamentsLoading, filamentsError, onRetryFilaments, onClose, onSaved }) {
  const { t } = useTranslation();
  const [managingTypes, setManagingTypes] = useState(false);
  const { data: types, loading: typesLoading, error: typesError, refetch: refetchTypes } = useApi(getFilamentTypes);
  const { data: brands } = useApi(getFilamentBrands);
  const { register, control, watch, setValue, handleSubmit, formState: { errors, isSubmitting } } = useForm({
    resolver: yupResolver(batchSchema(t)),
    defaultValues: { purchase_date: '', items: [emptyLine()] },
  });
  const { fields, append, remove } = useFieldArray({ control, name: 'items' });
  const items = watch('items');
  const prices = items.map((item) => toFiniteNumber(item.price));
  const total = prices.every((price) => price !== null) ? prices.reduce((sum, price) => sum + price, 0) : null;
  const close = () => { if (!isSubmitting) onClose(); };
  const submit = async (data) => {
    try {
      await createFilamentPurchaseBatch({
        purchase_date: new Date(data.purchase_date).toISOString(),
        items: data.items.map(({ filament_id, is_new, brand, type, color, price, amount }) => ({
          ...(is_new ? { new_filament: { brand, type, color } } : { filament_id }),
          price, amount,
        })),
      });
      toast.success(t('purchases.saved'));
      onSaved();
    } catch {
      // The API interceptor displays errors. Keep every line available for retry.
    }
  };

  if (managingTypes) return <FilamentTypesManager types={types || []} onClose={() => setManagingTypes(false)} onSaved={() => { setManagingTypes(false); refetchTypes(); }} />;

  return (
    <Modal title={t('purchases.new')} onClose={close}>
      <form onSubmit={handleSubmit(submit)} className="space-y-4">
        {filamentsLoading && <p role="status">{t('filaments.loading')}</p>}
        {filamentsError && <InlineError title={t('filaments.loadError')} message={filamentsError} onRetry={onRetryFilaments} />}
        <fieldset disabled={isSubmitting} className="space-y-4">
          <p className="text-sm ui-text-secondary">{t('purchaseBatch.help')}</p>
          <label className="block text-sm font-medium ui-text-secondary">
            {t('purchases.purchaseDate')}
            <input type="date" {...register('purchase_date')} className={inputClass} />
          </label>
          {errors.purchase_date && <p className="ui-text-danger text-xs">{errors.purchase_date.message}</p>}
          {fields.map((field, index) => (
            <fieldset key={field.id} className="border ui-border-subtle rounded-md p-3 space-y-3">
              <legend className="text-sm font-medium px-1">{t('purchaseBatch.filamentNumber', { number: index + 1 })}</legend>
              <div>
                <div className="mb-1 flex items-center justify-between gap-3">
                  <label htmlFor={`purchase-filament-${field.id}`} className="block text-sm font-medium ui-text-secondary">{t('purchases.filament')}</label>
                  <button
                    type="button"
                    onClick={() => {
                      setValue(`items.${index}.filament_id`, '', { shouldDirty: true });
                      setValue(`items.${index}.is_new`, true, { shouldDirty: true, shouldValidate: true });
                    }}
                    className="flex items-center gap-1 text-xs ui-text-primary-action hover:underline"
                  >
                    <Plus className="h-3 w-3" /> {t('purchaseBatch.createFilament')}
                  </button>
                </div>
                <Controller
                  name={`items.${index}.filament_id`}
                  control={control}
                  render={({ field: filamentField }) => (
                    <FilamentSelect
                      id={`purchase-filament-${field.id}`}
                      value={filamentField.value}
                      onChange={(filamentId) => {
                        filamentField.onChange(filamentId);
                        setValue(`items.${index}.is_new`, false, { shouldDirty: true, shouldValidate: true });
                      }}
                      filaments={filaments}
                      invalid={!!errors.items?.[index]?.filament_id}
                    />
                  )}
                />
              </div>
              {items[index]?.is_new && (
                <>
                  <div>
                    <label htmlFor={`purchase-brand-${field.id}`} className="block text-sm font-medium ui-text-secondary">{t('filaments.brand')}</label>
                    <Controller name={`items.${index}.brand`} control={control} render={({ field: brandField }) => (
                      <BrandSelect id={`purchase-brand-${field.id}`} {...brandField} brands={brands || []} invalid={!!errors.items?.[index]?.brand} />
                    )} />
                  </div>
                  <label className="block text-sm font-medium ui-text-secondary">
                    {t('filaments.color')}
                    <input {...register(`items.${index}.color`)} maxLength={200} className={inputClass} />
                  </label>
                  <label className="block text-sm font-medium ui-text-secondary">
                    {t('filaments.type')}
                    <select {...register(`items.${index}.type`)} disabled={typesLoading || !!typesError} className={inputClass}>
                      <FilamentTypeOptions types={types || []} current={items[index]?.type} />
                    </select>
                  </label>
                  <FilamentTypeCatalog loading={typesLoading} error={typesError} onRetry={refetchTypes} onManage={() => setManagingTypes(true)} />
                  <p className="text-xs ui-text-muted">{t('purchaseBatch.averageHelp')}</p>
                </>
              )}
              <label className="block text-sm font-medium ui-text-secondary">
                {t('purchases.price')}
                <input type="number" min="0.01" max={Number.MAX_SAFE_INTEGER} step="0.01" placeholder="e.g. 24.99" {...register(`items.${index}.price`)} className={inputClass} />
              </label>
              <label className="block text-sm font-medium ui-text-secondary">
                {t('purchases.amount')}
                <input type="number" min="0.001" max={Number.MAX_SAFE_INTEGER} step="0.001" placeholder="e.g. 1" {...register(`items.${index}.amount`)} className={inputClass} />
              </label>
              {Object.entries(errors.items?.[index] || {}).map(([name, error]) => <p key={name} role="alert" className="ui-text-danger text-xs">{error.message}</p>)}
              {fields.length > 1 && <button type="button" onClick={() => remove(index)} className="text-sm ui-text-danger">{t('purchaseBatch.removeFilament', { number: index + 1 })}</button>}
            </fieldset>
          ))}
          <button type="button" disabled={fields.length >= 100} onClick={() => append(emptyLine())} className="text-sm ui-text-primary-action disabled:opacity-50">{t('purchaseBatch.addAnother')}</button>
          <p className="font-medium text-sm">{t('purchaseBatch.total')}: {formatCurrency(total, { fallback: VALUE_FALLBACKS.notCalculated })}</p>
        </fieldset>
        <div className="flex justify-end gap-3 pt-2">
          <button type="button" disabled={isSubmitting} onClick={close} className="px-4 py-2 text-sm border rounded-md">{t('common.cancel')}</button>
          <button type="submit" disabled={isSubmitting || (items.some((item) => item.is_new) && (typesLoading || !!typesError))} className="px-4 py-2 text-sm ui-text-inverse ui-bg-primary rounded-md disabled:opacity-50 flex items-center gap-2">
            {isSubmitting && <Loader2 className="h-4 w-4 animate-spin" />}{t('purchases.record')}
          </button>
        </div>
      </form>
    </Modal>
  );
}
