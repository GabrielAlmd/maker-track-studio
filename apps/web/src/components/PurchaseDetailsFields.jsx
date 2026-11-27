import { useId } from 'react';
import { useTranslation } from 'react-i18next';

const inputClass = 'w-full border ui-border-default rounded-md px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ui-focus-ring';
const fields = [
  ['supplier', 'supplier'],
  ['supplier_reference', 'reference'],
  ['purchase_url', 'purchaseURL'],
];

export function PurchaseDetailField({ name, label, register, error, url = false }) {
  const id = useId();
  const { t } = useTranslation();
  return <div>
    <label htmlFor={id} className="block text-sm font-medium ui-text-secondary mb-1">{t(`purchaseDetails.${label}`)}</label>
    <input id={id} {...register(name)} className={inputClass} inputMode={url ? 'url' : undefined}
      aria-invalid={!!error} aria-describedby={error ? `${id}-error` : undefined} />
    {error && <p id={`${id}-error`} className="ui-text-danger text-xs mt-1">{error.message}</p>}
  </div>;
}

export default function PurchaseDetailsFields({ register, errors, prefix = '' }) {
  const { t } = useTranslation();
  return <fieldset className="grid min-w-0 gap-3 sm:grid-cols-2">
    <legend className="mb-2 text-sm font-semibold ui-text-primary">{t('purchaseDetails.optionalDetails')}</legend>
    {fields.map(([name, label]) => <div key={name} className={name === 'purchase_url' ? 'sm:col-span-2' : undefined}>
      <PurchaseDetailField name={prefix + name} label={label} register={register} error={errors?.[name]} url={name === 'purchase_url'} />
    </div>)}
  </fieldset>;
}
