import { useCallback, useState } from 'react';
import { yupResolver } from '@hookform/resolvers/yup';
import { useForm } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { Link, useNavigate } from 'react-router-dom';
import * as yup from 'yup';
import PageHeader from '../../../components/ui/PageHeader';
import { InlineError, LoadingState } from '../../../components/ui/PageState';
import { useApi } from '../../../hooks/useApi';
import { getModels } from '../../../services/modelService';
import { formatCurrencyFromCents, formatDate } from '../../../utils/formatters';
import { EntityField, Field } from '../components/BusinessFields';
import useBusinessCommand from '../components/useBusinessCommand';
import { allCustomers, listQuickSaleStock } from '../services/businessService';
import { numberField, textField } from '../utils/businessValidation';

const maxQuantity = 1_000_000_000;
const maxCents = Number.MAX_SAFE_INTEGER;
const schema = yup.object({
  reference: textField(160, true),
  model_id: yup.string().uuid().required(),
  customer_id: yup.string().transform((value) => value || undefined).uuid(),
  quantity: numberField(1, maxQuantity),
  unit_price_cents: numberField(1, maxCents).test('total', (value, { parent }) =>
    Number.isSafeInteger(value * parent.quantity)),
  mode: yup.string().oneOf(['made_to_order', 'stock']).required(),
  stock_run_item_id: yup.string().transform((value) => value || undefined).uuid().when('mode', {
    is: 'stock', then: (field) => field.required(),
  }),
});

function StockChoice({ model, quantity, control, error, clear }) {
  const { t } = useTranslation();
  const [page, setPage] = useState(1);
  const load = useCallback(() => listQuickSaleStock(model, quantity, page), [model, quantity, page]);
  const { data, loading, error: stockError, refetch } = useApi(load);
  const changePage = (next) => { clear(); setPage(next); };
  return <div className="space-y-2">
    {loading ? <LoadingState /> : stockError ? <InlineError title={t('business.loadError')} onRetry={refetch} /> :
      data?.items.length > 0 ? <EntityField required control={control} label="quickLot" error={error} options={[
        ['', t('business.quickSelectLot')],
        ...data.items.map((lot) => [lot.print_run_item_id, t('business.quickLotLabel', {
          date: formatDate(lot.print_date), count: lot.units_available, lot: lot.print_run_item_id.slice(0, 8),
        })]),
      ]} name="stock_run_item_id" /> : <p className="text-sm ui-text-secondary">{t('business.quickNoStock')}</p>}
    <div className="flex flex-wrap gap-2">
      <button type="button" className="page-action page-action-secondary" disabled={loading}
        onClick={() => { clear(); refetch(); }}>{t('business.quickRefreshStock')}</button>
      {(page > 1 || data?.has_more) && <>
        <button type="button" className="page-action page-action-secondary" disabled={loading || page <= 1}
          onClick={() => changePage(page - 1)}>{t('business.previous')}</button>
        <button type="button" className="page-action page-action-secondary" disabled={loading || Boolean(stockError) || !data?.has_more}
          onClick={() => changePage(page + 1)}>{t('business.next')}</button>
      </>}
    </div>
  </div>;
}

async function loadCustomers() {
  const customers = await allCustomers();
  return customers.filter(customer => !customer.is_archived);
}

async function searchModels({ query, cursor, signal }) {
  const page = cursor ?? 1;
  const result = await getModels({ search: query, page, page_size: 25 }, signal);
  return {
    items: result.items.map(model => [model.id, model.name]),
    nextCursor: page * result.page_size < result.total ? page + 1 : null,
  };
}

export default function QuickSale() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const { data: customers, loading, error, refetch } = useApi(loadCustomers);
  const command = useBusinessCommand();
  const { control, register, watch, setValue, handleSubmit, formState: { errors } } = useForm({
    resolver: yupResolver(schema),
    defaultValues: { reference: '', model_id: '', customer_id: '', quantity: 1, unit_price_cents: '', mode: 'made_to_order', stock_run_item_id: '' },
  });
  const [model, quantity, price, mode, lot] = watch(['model_id', 'quantity', 'unit_price_cents', 'mode', 'stock_run_item_id']);
  const fromStock = mode === 'stock';
  const total = quantity * price;
  const clearLot = () => setValue('stock_run_item_id', '', { shouldValidate: false });

  async function submit({ mode, customer_id, stock_run_item_id, ...values }) {
    const order = await command.run('quick-sales', {
      ...values, customer_id: customer_id || undefined,
      stock_run_item_id: mode === 'stock' ? stock_run_item_id : undefined,
    });
    if (order) navigate(`/business/orders/${order.id}`, { replace: true });
  }

  return <>
    <PageHeader title={t('business.quickSale')} description={t('business.quickSaleHelp')}>
      <Link to="/business/orders" className="page-action page-action-secondary">{t('sales.backToOrders')}</Link>
    </PageHeader>
    {loading ? <LoadingState /> : error ? <InlineError title={t('business.loadError')} onRetry={refetch} /> :
      <form className="page-panel max-w-3xl p-5 sm:p-6" noValidate onSubmit={handleSubmit(submit)}>
        <fieldset disabled={command.busy} className="space-y-5">
          <div className="grid gap-4 sm:grid-cols-2">
            <Field label="quickReference" error={errors.reference} maxLength={160} {...register('reference')} />
            <EntityField control={control} label="quickCustomer" error={errors.customer_id} options={[
              ['', t('business.walkIn')], ...customers.map((customer) => [customer.id, customer.name]),
            ]} name="customer_id" />
            <EntityField required control={control} label="quickModel" error={errors.model_id} loadOptions={searchModels}
              options={[['', t('business.quickSelectModel')]]} name="model_id" onSelect={clearLot} />
            <Field label="quantity" error={errors.quantity} type="number" min={1} max={maxQuantity} step={1}
              {...register('quantity', { onChange: clearLot })} />
            <Field label="quickPrice" error={errors.unit_price_cents} type="number" min={1} max={maxCents} step={1}
              {...register('unit_price_cents')} />
            <Field label="quickFulfilment" options={[
              ['made_to_order', t('business.quickMadeToOrder')], ['stock', t('business.quickInStock')],
            ]} {...register('mode', { onChange: clearLot })} />
          </div>
          {fromStock && model && Number.isInteger(Number(quantity)) && quantity >= 1 && quantity <= maxQuantity &&
            <StockChoice key={`${model}-${quantity}`} model={model} quantity={quantity} control={control}
              error={errors.stock_run_item_id} clear={clearLot} />}
          {command.error && <p role="alert" className="page-form-error">{command.error}</p>}
          <div className="flex flex-wrap items-center justify-between gap-3">
            <p className="font-semibold" aria-live="polite">
              {Number.isSafeInteger(total) && total > 0 && t('business.quickTotal', { amount: formatCurrencyFromCents(total) })}
            </p>
            <button type="submit" className="page-action" disabled={command.busy || (fromStock && !lot)}>
              {t(fromStock ? 'business.quickConfirmStock' : 'business.quickConfirm')}
            </button>
          </div>
        </fieldset>
      </form>}
  </>;
}
