import { useCallback } from 'react';
import { useFieldArray, useForm } from 'react-hook-form';
import { yupResolver } from '@hookform/resolvers/yup';
import * as yup from 'yup';
import { useTranslation } from 'react-i18next';
import { useApi } from '../../../hooks/useApi';
import { getFilaments } from '../../../services/filamentService';
import { getPrinters } from '../../../services/printerService';
import { InlineError, LoadingState } from '../../../components/ui/PageState';
import { getModels } from '../../../services/modelService';
import { formatCurrencyFromCents } from '../../../utils/formatters';
import { numberField, textField } from '../utils/businessValidation';
import { EntityField, Field } from './BusinessFields';

const assumptions = {
  print_time_minutes: [0, 1_000_000],
  labor_hours: [0, 1_000_000, false],
  post_process_cents: [0, Number.MAX_SAFE_INTEGER],
  other_cents: [0, Number.MAX_SAFE_INTEGER],
};
const blankItem = { quantity: 1, name: '', model_id: '', assumptions: { ...Object.fromEntries(Object.keys(assumptions).map((key) => [key, 0])), printer_id: '', materials: [] } };
const material = yup.object({
  filament_id: yup.string().required(),
  weight_g: numberField(0, 1_000_000, false).moreThan(0),
  waste_g: yup.number().nullable().transform((value, original) => original === '' ? null : value)
    .min(0).max(yup.ref('weight_g')),
});
const customerSchema = yup.object({
  name: textField(160, true), email: textField(254), phone: textField(64), address: textField(500), is_archived: yup.boolean(),
});
const budgetSchema = yup.object({
  title: textField(160, true), customer_id: yup.string(), is_archived: yup.boolean(),
  items: yup.array().min(1).max(100).of(yup.object({
    id: yup.string(), name: textField(200).when('model_id', { is: '', then: (field) => field.required() }),
    model_id: yup.string(), quantity: numberField(1, 1_000_000),
    assumptions: yup.object({
      ...Object.fromEntries(Object.entries(assumptions).map(([key, bounds]) => [key, numberField(...bounds)])),
      printer_id: yup.string(),
      materials: yup.array().max(20).of(material).test('distinct', 'Choose distinct materials', (rows) =>
        !rows || new Set(rows.map((row) => row.filament_id)).size === rows.length),
    }),
  })),
});

function MaterialInputs({ index, control, register, errors, options, current }) {
  const { t } = useTranslation();
  const name = `items.${index}.assumptions.materials`;
  const { fields, append, remove } = useFieldArray({ control, name, keyName: 'fieldKey' });
  return <div className="space-y-3">
    {fields.map((row, i) => {
      const retained = options.some(([id]) => id === row.filament_id) ? options :
        [...options, [row.filament_id, current?.sources?.materials[i]?.name ?? t('common.notRecorded')]];
      return <div key={row.fieldKey} className="grid items-end gap-3 sm:grid-cols-4">
        <EntityField required control={control} label="filament" options={retained} name={`${name}.${i}.filament_id`} error={errors?.[i]?.filament_id} />
        <Field label="weight" type="number" min={0} max={1_000_000} step="any" {...register(`${name}.${i}.weight_g`)} error={errors?.[i]?.weight_g} />
        <Field label="waste" type="number" min={0} max={1_000_000} step="any" {...register(`${name}.${i}.waste_g`)} error={errors?.[i]?.waste_g} />
        <button className="page-action page-action-secondary" type="button" onClick={() => remove(i)}>{t('common.remove')}</button>
      </div>;
    })}
    {errors?.message && <p role="alert" className="ui-text-danger">{t('business.invalidField')}</p>}
    <button className="page-action page-action-secondary" type="button" disabled={fields.length >= 20} onClick={() => append({ filament_id: '', weight_g: '', waste_g: '' })}>{t('business.addMaterial')}</button>
  </div>;
}

export default function EstimateEditor({ kind, record, customers, onSave, command }) {
  const { t } = useTranslation();
  const budget = kind === 'budgets';
  const fetchCatalog = useCallback(() => budget ? Promise.all([getModels(), getPrinters(), getFilaments()]) : Promise.resolve([]), [budget]);
  const { data: catalog, loading, error, refetch } = useApi(fetchCatalog);
  const [models = [], printers = [], filaments = []] = catalog ?? [];
  const { register, control, handleSubmit, formState: { errors } } = useForm({
    resolver: yupResolver(budget ? budgetSchema : customerSchema),
    defaultValues: budget ? {
      title: record?.title ?? '', customer_id: record?.customer_id ?? '', is_archived: record?.is_archived ?? false,
      items: record?.items.map((item) => ({
        ...item, model_id: item.model_id ?? '',
        assumptions: { ...item.assumptions, printer_id: item.assumptions.printer_id ?? '', materials: item.assumptions.materials ?? [] },
      })) ?? [blankItem],
    } : { name: '', email: '', phone: '', address: '', is_archived: false, ...record },
  });
  const { fields, append, remove } = useFieldArray({ control, name: 'items', keyName: 'fieldKey' });
  const modelOptions = [['', t('business.adHoc')], ...(models ?? []).map(({ id, name }) => [id, name])];
  const printerOptions = [['', t('business.defaultPrinter')], ...printers.map(({ id, name }) => [id, name])];
  const filamentOptions = [['', t('business.chooseFilament')], ...filaments.filter(f => !f.archived).map(({ id, brand, type, color }) => [id, [brand, type, color].filter(Boolean).join(' ')])];
  if (budget && loading) return <LoadingState />;
  if (budget && error) return <InlineError title={t('business.loadError')} onRetry={refetch} />;
  return <form className="page-panel p-5 sm:p-6" noValidate onSubmit={handleSubmit((values) => {
    if (!budget) {
      const { name, email, phone, address, is_archived } = values;
      return onSave({ name, email, phone, address, is_archived });
    }
    return onSave({
      title: values.title, customer_id: values.customer_id || null, is_archived: values.is_archived,
      items: values.items.map((item) => ({
        id: item.id, model_id: item.model_id || null, name: item.name, quantity: item.quantity,
        assumptions: {
          ...item.assumptions, printer_id: item.assumptions.printer_id || null,
        },
      })),
    });
  })}>
    <fieldset disabled={command.busy} className="space-y-4">
      {budget ? <>
        <Field label="title" {...register('title')} maxLength={160} required error={errors.title} />
        <EntityField control={control} label="customer" name="customer_id" options={[['', t('business.unassigned')], ...customers.filter((customer) => !customer.is_archived || record?.customer_id === customer.id).map(({ id, name }) => [id, name])]} />
        {fields.map((item, index) => {
          const current = record?.items.find(({ id }) => id === item.id);
          const printerId = item.assumptions.printer_id;
          const availablePrinters = printerId && !printerOptions.some(([id]) => id === printerId) ?
            [...printerOptions, [printerId, current?.sources?.printer_name ?? t('common.notRecorded')]] : printerOptions;

          return <fieldset key={item.fieldKey} className="rounded-lg border ui-border-subtle p-4">
            <legend className="px-2 font-semibold">{t('sales.itemNumber', { number: index + 1 })}</legend>
            <div className="grid gap-4 sm:grid-cols-2">
              <EntityField control={control} label="model" name={`items.${index}.model_id`} options={item.model_id && !modelOptions.some(([id]) => id === item.model_id) ? [...modelOptions, [item.model_id, item.name]] : modelOptions} />
              <Field label="name" {...register(`items.${index}.name`)} maxLength={200} error={errors.items?.[index]?.name} />
              <Field label="quantity" type="number" min={1} max={1_000_000} step={1} {...register(`items.${index}.quantity`)} error={errors.items?.[index]?.quantity} />
              <EntityField control={control} label="printer" name={`items.${index}.assumptions.printer_id`} options={availablePrinters} />
              {Object.entries(assumptions).map(([key, [min, max, whole]]) => <Field key={key} label={key} type="number" min={min} max={max} step={whole === false ? 'any' : 1} {...register(`items.${index}.assumptions.${key}`)} error={errors.items?.[index]?.assumptions?.[key]} />)}
            </div>
            <div className="mt-4">
              <MaterialInputs key={index} index={index} control={control} register={register} errors={errors.items?.[index]?.assumptions?.materials} options={filamentOptions} current={current} />
            </div>
            {fields.length > 1 && <button className="page-action page-action-secondary mt-3" type="button" onClick={() => remove(index)}>{t('sales.removeItem', { number: index + 1 })}</button>}
          </fieldset>;
        })}
        <button className="page-action page-action-secondary" type="button" disabled={fields.length >= 100} onClick={() => append(blankItem)}>{t('sales.addItem')}</button>
        {record && <p>{t('business.estimate')} {formatCurrencyFromCents(record.estimated_cost_cents)}</p>}
      </> : Object.entries({ name: 160, email: 254, phone: 64, address: 500 }).map(([key, maxLength]) =>
        <Field key={key} label={key} multiline={key === 'address'} maxLength={maxLength} required={key === 'name'} {...register(key)} error={errors[key]} />)}
      {record && <label className="flex items-center gap-2"><input type="checkbox" {...register('is_archived')} />{t('sales.archived')}</label>}
      {command.error && <p className="page-form-error" role="alert">{command.error}</p>}
      <button className="page-action" type="submit">{t('common.saveChanges')}</button>
    </fieldset>
  </form>;
}
