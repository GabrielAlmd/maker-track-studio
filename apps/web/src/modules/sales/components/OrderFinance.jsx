import { useCallback } from 'react';
import { useForm } from 'react-hook-form';
import { yupResolver } from '@hookform/resolvers/yup';
import * as yup from 'yup';
import { useTranslation } from 'react-i18next';
import { useApi } from '../../../hooks/useApi';
import { InlineError, LoadingState } from '../../../components/ui/PageState';
import { formatCurrencyFromCents as money } from '../../../utils/formatters';
import { EntityField, Field } from './BusinessFields';
import useBusinessCommand from './useBusinessCommand';
import { getRecord } from '../services/businessService';
import { textField } from '../utils/businessValidation';

const kinds = ['invoice', 'payment', 'refund', 'void'];
const schema = yup.object({
  kind: yup.string().oneOf(kinds).required(),
  reference: textField(160, true),
  date: yup.string().required(),
  due_date: yup.string(),
  amount: yup.string().matches(/^\d{1,12}(\.\d{1,2})?$/).test('positive', 'amount', value => Number(value) > 0).required(),
  original_revision: yup.string().when('kind', { is: kind => ['refund', 'void'].includes(kind), then: field => field.required() }),
});

export default function OrderFinance({ orderId, onSaved, quickSale = false }) {
  const { t } = useTranslation();
  const path = `orders/${orderId}/finance`;
  const fetchFinance = useCallback(() => getRecord(path), [path]);
  const { data, loading, error, refetch } = useApi(fetchFinance);
  const command = useBusinessCommand();
  const { control, register, watch, handleSubmit, formState: { errors } } = useForm({
    resolver: yupResolver(schema), defaultValues: { kind: 'invoice', date: new Date().toISOString().slice(0, 10) },
  });
  const kind = watch('kind');
  const reversing = kind === 'refund' || kind === 'void';
  if (loading) return <LoadingState />;
  if (error || !data) return <InlineError title={t('business.loadError')} onRetry={refetch} />;
  const entries = data.entries;
  const kindLabel = value => t(`business.activityKinds.${value}`);
  async function record(values) {
    const [euros, fraction = ''] = values.amount.split('.');
    const result = await command.run(path, {
      kind, reference: values.reference, date: values.date, amount_cents: Number(euros) * 100 + Number(fraction.padEnd(2, '0')),
      due_date: kind === 'invoice' ? values.due_date || '' : '',
      original_revision: reversing ? Number(values.original_revision) : 0,
      expected_revision: entries.length,
    });
    if (result) await onSaved();
  }
  return <section className="page-panel mt-4 space-y-4 p-5 sm:p-6">
    <h2 className="text-lg font-semibold">{t('business.financeTitle')}</h2>
    <p className="text-sm ui-text-secondary">{t('business.fiscalScope')}</p>
    <dl className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
      {['invoiced_cents', 'net_paid_cents', 'balance_cents', 'estimated_profit_cents'].filter(key => data[key] != null).map(key =>
        <div key={key}><dt className="text-xs ui-text-muted">{t(key === 'estimated_profit_cents' && quickSale ? 'business.quickEstimatedProfit' : `business.${key}`)}</dt><dd>{money(data[key])}</dd></div>)}
    </dl>
    <p>{t(`business.invoiceStates.${data.invoice_status}`)} · {t(`business.paymentStates.${data.payment_status}`)}
      {data.overdue && <strong className="ui-text-danger"> · {t('business.overdue')}</strong>}</p>
    <ul className="divide-y ui-divide-subtle">{entries.map(e => <li key={e.revision} className="flex flex-wrap justify-between gap-2 py-2">
      <span className="break-words">#{e.revision} {kindLabel(e.kind)} · {e.reference} {e.original_revision > 0 && `(→ #${e.original_revision})`}</span>
      <span>{e.date} {e.due_date && ` · ${t('business.financeDue')} ${e.due_date}`} · {money(e.amount_cents)}</span>
    </li>)}</ul>
    <form onSubmit={handleSubmit(record)}>
      <fieldset disabled={command.busy} className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
        <Field label="financeKind" options={kinds.map(k => [k, kindLabel(k)])} {...register('kind')} />
        <Field label="financeReference" maxLength={160} required {...register('reference')} error={errors.reference} />
        <Field label="financeDate" type="date" required {...register('date')} error={errors.date} />
        {kind === 'invoice' && <Field label="financeDue" type="date" {...register('due_date')} />}
        {reversing && <EntityField control={control} label="financeOriginal" required options={[['', '—'], ...entries.filter(e => e.kind === (kind === 'void' ? 'invoice' : 'payment')).map(e => [e.revision, `#${e.revision} · ${e.reference} · ${money(e.amount_cents)}`])]} name="original_revision" error={errors.original_revision} />}
        <Field label="financeAmount" type="number" min="0.01" max="999999999999.99" step="0.01" required {...register('amount')} error={errors.amount} />
        <button className="page-action self-end justify-self-start" type="submit">{t('business.recordFinance')}</button>
      </fieldset>
      {command.error && <div className="mt-2">
        <p className="page-form-error" role="alert">{command.error}</p>
        <button type="button" className="page-action page-action-secondary" onClick={refetch}>{t('business.refreshFinance')}</button>
      </div>}
    </form>
  </section>;
}
