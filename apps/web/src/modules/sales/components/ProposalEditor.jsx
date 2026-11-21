import { useForm, useWatch } from 'react-hook-form';
import { yupResolver } from '@hookform/resolvers/yup';
import * as yup from 'yup';
import { useTranslation } from 'react-i18next';
import { numberField, textField } from '../utils/businessValidation';
import { Field } from './BusinessFields';

const priceModes = ['manual', 'markup', 'margin'];
const discountModes = ['', 'fixed', 'percent'];
const maxValue = (mode) => mode === 'margin' ? 9999 : mode === 'percent' ? 10000 : Number.MAX_SAFE_INTEGER;
const selection = (modes) => yup.object({
  mode: yup.string().defined().oneOf(modes),
  value: yup.number().integer().min(0).required().when('mode', ([mode], field) => field.max(maxValue(mode))),
});
const dateField = yup.string().matches(/^\d{4}-\d{2}-\d{2}$/, { excludeEmptyString: true });
const schema = yup.object({
  customer_id: yup.string().required(),
  terms: yup.object({
    title: textField(160, true), valid_until: dateField, due_date: dateField,
    lead_time_days: numberField(0, 3650), delivery_terms: textField(2000), payment_terms: textField(2000),
  }),
  lines: yup.array().min(1).of(yup.object({
    budget_line_id: yup.string().required(), description: textField(200, true),
    price: selection(priceModes), discount: selection(discountModes),
  })),
  order_discount: selection(discountModes),
});
const zeroDiscount = { mode: '', value: 0 };

function PricingFields({ register, control, name, modes, error }) {
  const { t } = useTranslation();
  const mode = useWatch({ control, name: `${name}.mode` });
  return <div className="grid gap-3 sm:grid-cols-2">
    <Field label={name.endsWith('price') ? 'pricing' : 'discount'} {...register(`${name}.mode`)} options={modes.map((option) => [option, t(`business.mode_${option || 'none'}`)])} />
    <Field label={mode === 'manual' || mode === 'fixed' ? 'cents' : 'basisPoints'} type="number" min={0} max={maxValue(mode)} step={1}
      {...register(`${name}.value`)} readOnly={!mode} error={error?.value} />
  </div>;
}

export default function ProposalEditor({ record, budget, customers, onSave, command }) {
  const { t } = useTranslation();
  const { register, control, handleSubmit, formState: { errors } } = useForm({
    resolver: yupResolver(schema),
    defaultValues: {
      customer_id: record?.customer_id ?? budget.customer_id ?? '',
      terms: record?.terms ?? { title: budget.title, valid_until: '', due_date: '', lead_time_days: 0, delivery_terms: '', payment_terms: '' },
      lines: record?.lines ?? budget.items.map((item) => ({
        budget_line_id: item.id, description: item.name, price: { mode: 'manual', value: item.estimate.unit_cents }, discount: zeroDiscount,
      })),
      order_discount: record?.order_discount ?? zeroDiscount,
    },
  });
  const customerOptions = customers.filter((customer) => !customer.is_archived && (!budget.customer_id || customer.id === budget.customer_id));
  const discount = ({ mode, value }) => mode ? { mode, value } : zeroDiscount;
  return <form className="page-panel p-5 sm:p-6" noValidate onSubmit={handleSubmit((values) => onSave({
    budget_id: budget.id, budget_revision: budget.revision, customer_id: values.customer_id, terms: values.terms,
    lines: values.lines.map(({ budget_line_id, description, price, discount: lineDiscount }) =>
      ({ budget_line_id, description, price, discount: discount(lineDiscount) })),
    order_discount: discount(values.order_discount),
  }))}>
    <fieldset disabled={command.busy} className="space-y-4">
      <legend className="mb-3 text-lg font-semibold">{t('business.draftEditor')}</legend>
      <Field label="customer" {...register('customer_id')} options={[['', t('business.chooseCustomer')], ...customerOptions.map(({ id, name }) => [id, name])]} error={errors.customer_id} />
      <div className="grid gap-4 sm:grid-cols-2">
        <Field label="title" required maxLength={160} {...register('terms.title')} error={errors.terms?.title} />
        <Field label="validUntil" type="date" {...register('terms.valid_until')} error={errors.terms?.valid_until} />
        <Field label="dueDate" type="date" {...register('terms.due_date')} error={errors.terms?.due_date} />
        <Field label="leadTime" type="number" min={0} max={3650} step={1} {...register('terms.lead_time_days')} error={errors.terms?.lead_time_days} />
        <Field label="deliveryTerms" multiline maxLength={2000} {...register('terms.delivery_terms')} error={errors.terms?.delivery_terms} />
        <Field label="paymentTerms" multiline maxLength={2000} {...register('terms.payment_terms')} error={errors.terms?.payment_terms} />
      </div>
      {(record?.lines ?? budget.items).map((line, index) => <fieldset key={line.budget_line_id ?? line.id} className="space-y-3 rounded-lg border ui-border-subtle p-4">
        <legend className="px-2 font-semibold">{t('sales.itemNumber', { number: index + 1 })}</legend>
        <Field label="description" required maxLength={200} {...register(`lines.${index}.description`)} error={errors.lines?.[index]?.description} />
        <PricingFields register={register} control={control} name={`lines.${index}.price`} modes={priceModes} error={errors.lines?.[index]?.price} />
        <PricingFields register={register} control={control} name={`lines.${index}.discount`} modes={discountModes} error={errors.lines?.[index]?.discount} />
      </fieldset>)}
      <fieldset className="rounded-lg border ui-border-subtle p-4">
        <legend className="px-2 font-semibold">{t('business.orderDiscount')}</legend>
        <PricingFields register={register} control={control} name="order_discount" modes={discountModes} error={errors.order_discount} />
      </fieldset>
      {command.error && <p className="page-form-error" role="alert">{command.error}</p>}
      <button className="page-action" type="submit">{t('business.saveDraft')}</button>
    </fieldset>
  </form>;
}
