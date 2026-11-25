import FormSelect from '../../../components/ui/FormSelect';
import { useId } from 'react';
import { useTranslation } from 'react-i18next';

export function Field({ label, error, options, multiline, ...input }) {
  const { t } = useTranslation();
  const id = useId();
  const props = {
    ...input,
    id,
    'aria-invalid': Boolean(error),
    'aria-describedby': error ? `${id}-error` : undefined,
    className: 'mt-1 block w-full rounded-md border ui-border-default px-3 py-2',
  };

  return <div className="min-w-0 text-sm ui-text-secondary">
    <label htmlFor={id}>{t(`business.${label}`)}</label>
    {options ? <select {...props}>{options.map(([value, name]) => <option key={value} value={value}>{name}</option>)}</select>
      : multiline ? <textarea {...props} rows={3} /> : <input {...props} />}
    {error && <p id={`${id}-error`} className="mt-1 ui-text-danger" role="alert">{t('business.invalidField')}</p>}
  </div>;
}

export function EntityField({ label, options, error, ...props }) {
  const { t } = useTranslation();
  const id = useId();
  const [[, placeholder], ...items] = options;
  return <div className="min-w-0 text-sm ui-text-secondary">
    <label htmlFor={id}>{t(`business.${label}`)}</label>
    <FormSelect {...props} id={id} className="mt-1" clearLabel={placeholder}
      options={items} getKey={entry => String(entry[0])} getLabel={entry => entry[1]}
      aria-describedby={error ? id + '-error' : undefined} />
    {error && <p id={id + '-error'} className="mt-1 ui-text-danger" role="alert">{t('business.invalidField')}</p>}
  </div>;
}
