import { useCallback, useEffect, useMemo, useState } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
import { useFieldArray, useForm } from 'react-hook-form';
import { yupResolver } from '@hookform/resolvers/yup';
import * as yup from 'yup';
import { Loader2, Plus, Trash2 } from 'lucide-react';
import toast from 'react-hot-toast';
import { useTranslation } from 'react-i18next';
import PageHeader from '../../../components/ui/PageHeader';
import HelpTooltip from '../../../components/ui/HelpTooltip';
import { InlineError, LoadingState } from '../../../components/ui/PageState';
import SalesAllocationEditor from '../components/SalesAllocationEditor';
import SalesFinancialSummary from '../components/SalesFinancialSummary';
import SalesChannelModal from '../components/SalesChannelModal';
import SalesOrderStatus from '../components/SalesOrderStatus';
import { getModels } from '../../../services/modelService';
import { getPrintJobCost } from '../../../services/printJobService';
import {
  createSalesOrder,
  deleteSalesOrder,
  getCompatiblePrintJobs,
  getPrintJobAllocations,
  getSalesChannels,
  getSalesOrder,
  getSalesOrderAllocations,
  replaceSalesOrderAllocations,
  updateSalesOrder,
  updateSalesOrderStatus,
} from '../services/salesService';
import { formatCurrency, formatCurrencyFromCents, formatDate, formatNumber } from '../../../utils/formatters';
import { suggestedUnitPrice, unitCostFromPrintJobs } from '../utils/salesPricing';

const PAYMENT = ['unpaid', 'paid', 'refunded'];
const NEXT_STATUS = { draft: 'confirmed', confirmed: 'preparing', preparing: 'ready_to_ship', ready_to_ship: 'shipped', shipped: 'completed' };
let clientItemSequence = 0;

function newItem(values = {}) {
  clientItemSequence += 1;
  return { client_id: values.id ?? `new-item-${clientItemSequence}`, id: values.id, model_id: values.model_id ?? '', model_name_snapshot: values.model_name_snapshot ?? '', quantity: values.quantity ?? 1, target_profit_percent: '', unit_price: values.unit_price_cents == null ? '' : values.unit_price_cents / 100 };
}

function optionalMoney(t) {
  return yup.number().transform((value, original) => original === '' ? 0 : value).typeError(t('sales.validation.money')).min(0, t('sales.validation.nonNegative')).required(t('sales.validation.money'));
}

function orderSchema(t) {
  return yup.object({
    reference: yup.string().trim().required(t('sales.validation.referenceRequired')).max(200, t('sales.validation.referenceLength')),
    order_date: yup.string().required(t('sales.validation.dateRequired')).test('valid-date', t('sales.validation.dateInvalid'), (value) => Boolean(value) && !Number.isNaN(new Date(`${value}T00:00:00Z`).valueOf())),
    sales_channel_id: yup.string().default(''),
    payment_status: yup.string().oneOf(PAYMENT, t('sales.validation.statusInvalid')).required(t('sales.validation.statusInvalid')),
    notes: yup.string().max(2000, t('sales.validation.notesLength')),
    packaging_cost: optionalMoney(t), shipping_cost: optionalMoney(t), platform_fee: optionalMoney(t), other_cost: optionalMoney(t),
    items: yup.array().of(yup.object({
      model_id: yup.string().required(t('sales.validation.modelRequired')),
      quantity: yup.number().transform((value, original) => original === '' ? undefined : value).typeError(t('sales.validation.quantity')).integer(t('sales.validation.wholeNumber')).positive(t('sales.validation.quantity')).required(t('sales.validation.quantity')),
      target_profit_percent: yup.number().transform((value, original) => original === '' ? undefined : value).typeError(t('sales.validation.targetProfit')).min(0, t('sales.validation.nonNegative')).test('finite', t('sales.validation.targetProfit'), (value) => value === undefined || Number.isFinite(value)).notRequired(),
      unit_price: yup.number().transform((value, original) => original === '' ? undefined : value).typeError(t('sales.validation.unitPrice')).min(0, t('sales.validation.nonNegative')).required(t('sales.validation.unitPrice')),
    })).min(1, t('sales.validation.itemRequired')).max(100, t('sales.validation.itemLimit')),
  });
}

function cents(value) {
  return Math.round(Number(value || 0) * 100);
}

function apiMessage(error, fallback) {
  if (error?.response) return fallback;
  const message = error?.message;
  return typeof message === 'string' && message ? message : fallback;
}

function fieldError(error, id) {
  return error && <p id={id} className="mt-1 text-xs ui-text-danger" role="alert">{error.message}</p>;
}

function SalesOrder() {
  const { id } = useParams();
  const isNew = !id;
  const navigate = useNavigate();
  const { t } = useTranslation();
  const [order, setOrder] = useState(null);
  const [channels, setChannels] = useState([]);
  const [models, setModels] = useState([]);
  const [allocations, setAllocations] = useState({});
  const [originalByJob, setOriginalByJob] = useState({});
  const [linkedJobs, setLinkedJobs] = useState({});
  const [jobCache, setJobCache] = useState({});
  const [allocationErrors, setAllocationErrors] = useState({});
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState(null);
  const [submitError, setSubmitError] = useState(null);
  const [actionLoading, setActionLoading] = useState(false);
  const [paymentStatus, setPaymentStatus] = useState('unpaid');
  const [showChannelModal, setShowChannelModal] = useState(false);
  const [pendingChannelID, setPendingChannelID] = useState(null);

  const schema = useMemo(() => orderSchema(t), [t]);
  const { control, register, handleSubmit, reset, setValue, watch, formState: { errors, isSubmitting } } = useForm({
    resolver: yupResolver(schema),
    defaultValues: { reference: '', order_date: new Date().toISOString().slice(0, 10), sales_channel_id: '', payment_status: 'unpaid', notes: '', packaging_cost: 0, shipping_cost: 0, platform_fee: 0, other_cost: 0, items: [newItem()] },
  });
  const { fields, append, remove } = useFieldArray({ control, name: 'items', keyName: 'fieldKey' });
  const watchedItems = watch('items', []);
  const watchedCosts = watch(['packaging_cost', 'shipping_cost', 'platform_fee', 'other_cost']);

  const applyOrder = useCallback((loadedOrder, allocationDocument) => {
    const formItems = loadedOrder.items.map(newItem);
    reset({
      reference: loadedOrder.reference,
      order_date: loadedOrder.order_date.slice(0, 10),
      sales_channel_id: loadedOrder.sales_channel_id ?? '',
      payment_status: loadedOrder.payment_status,
      notes: loadedOrder.notes ?? '',
      packaging_cost: loadedOrder.packaging_cost_cents / 100,
      shipping_cost: loadedOrder.shipping_cost_cents / 100,
      platform_fee: loadedOrder.platform_fee_cents / 100,
      other_cost: loadedOrder.other_cost_cents / 100,
      items: formItems,
    });
    const byClient = {};
    const originals = {};
    const linked = {};
    for (const item of formItems) {
      const itemAllocations = allocationDocument?.items?.find((entry) => entry.sales_order_item_id === item.id)?.allocations ?? [];
      byClient[item.client_id] = itemAllocations.filter((entry) => entry.print_job_id).map((entry) => ({ print_job_id: entry.print_job_id, units_allocated: entry.units_allocated }));
      for (const allocation of itemAllocations) {
        if (allocation.print_job_id) {
          originals[allocation.print_job_id] = (originals[allocation.print_job_id] ?? 0) + allocation.units_allocated;
          if (allocation.print_job) {
            linked[item.model_id] ??= [];
            linked[item.model_id].push({ id: allocation.print_job_id, model_id: item.model_id, print_date: null, units_printed: allocation.print_job.units_printed, capacity: allocation.print_job });
          }
        }
      }
    }
    setAllocations(byClient);
    setOriginalByJob(originals);
    setLinkedJobs(linked);
    setJobCache({});
    setPaymentStatus(loadedOrder.payment_status);
  }, [reset]);

  const load = useCallback(async () => {
    setLoading(true);
    setLoadError(null);
    try {
      const metadata = Promise.all([getSalesChannels(isNew ? 'active' : 'all'), getModels()]);
      if (isNew) {
        const [loadedChannels, loadedModels] = await metadata;
        setChannels(loadedChannels);
        setModels(loadedModels);
        setOrder(null);
        reset({ reference: '', order_date: new Date().toISOString().slice(0, 10), sales_channel_id: '', payment_status: 'unpaid', notes: '', packaging_cost: 0, shipping_cost: 0, platform_fee: 0, other_cost: 0, items: [newItem()] });
        setAllocations({});
        setOriginalByJob({});
        setLinkedJobs({});
        setJobCache({});
        setPaymentStatus('unpaid');
      } else {
        const [[loadedChannels, loadedModels], loadedOrder, allocationDocument] = await Promise.all([metadata, getSalesOrder(id), getSalesOrderAllocations(id)]);
        setChannels(loadedChannels);
        setModels(loadedModels);
        setOrder(loadedOrder);
        applyOrder(loadedOrder, allocationDocument);
      }
    } catch (error) {
      setLoadError(apiMessage(error, t('sales.orderLoadError')));
    } finally {
      setLoading(false);
    }
  }, [applyOrder, id, isNew, reset, t]);

  useEffect(() => { load(); }, [load]);

  useEffect(() => {
    if (!pendingChannelID || !channels.some((channel) => channel.id === pendingChannelID)) return;
    setValue('sales_channel_id', pendingChannelID, { shouldValidate: true });
    setPendingChannelID(null);
  }, [channels, pendingChannelID, setValue]);

  const loadJobs = useCallback(async (modelId, force = false) => {
    if (!modelId || (!force && jobCache[modelId])) return;
    setJobCache((current) => ({ ...current, [modelId]: { ...current[modelId], loading: true, error: null } }));
    try {
      const page = await getCompatiblePrintJobs(modelId);
      const jobs = await Promise.all((page.items ?? []).map(async (job) => {
        const [usage, cost] = await Promise.all([
          getPrintJobAllocations(job.id),
          getPrintJobCost(job.id).catch(() => null),
        ]);
        return { ...job, capacity: usage.print_job, cost };
      }));
      const merged = new Map(jobs.map((job) => [job.id, job]));
      for (const linked of linkedJobs[modelId] ?? []) {
        if (!merged.has(linked.id)) merged.set(linked.id, linked);
      }
      setJobCache((current) => ({ ...current, [modelId]: { loading: false, error: null, jobs: [...merged.values()] } }));
    } catch (error) {
      setJobCache((current) => ({ ...current, [modelId]: { loading: false, error: apiMessage(error, t('sales.compatibleJobsError')), jobs: current[modelId]?.jobs ?? linkedJobs[modelId] ?? [] } }));
    }
  }, [jobCache, linkedJobs, t]);

  const modelKey = watchedItems.map((item) => item?.model_id).filter(Boolean).join('|');
  useEffect(() => {
    [...new Set(modelKey.split('|').filter(Boolean))].forEach((modelId) => loadJobs(modelId));
  }, [loadJobs, modelKey]);

  const editableDetails = isNew || order?.fulfilment_status === 'draft';
  const editableAllocations = isNew || (order && !['completed', 'cancelled'].includes(order.fulfilment_status));

  const changeAllocation = (clientId, printJobId, value) => {
    setAllocations((current) => {
      const rows = current[clientId] ?? [];
      const without = rows.filter((row) => row.print_job_id !== printJobId);
      return { ...current, [clientId]: value === '' || Number(value) === 0 ? without : [...without, { print_job_id: printJobId, units_allocated: value }] };
    });
    setAllocationErrors((current) => ({ ...current, [clientId]: null, general: null }));
  };

  const validateAllocationRows = (items) => {
    const nextErrors = {};
    const jobTotals = {};
    const prepared = [];
    items.forEach((item, index) => {
      const rows = allocations[item.client_id] ?? [];
      let itemTotal = 0;
      rows.forEach((row) => {
        const units = Number(row.units_allocated);
        if (!Number.isSafeInteger(units) || units <= 0) nextErrors[item.client_id] = t('sales.validation.allocationUnits');
        else {
          itemTotal += units;
          jobTotals[row.print_job_id] = (jobTotals[row.print_job_id] ?? 0) + units;
          prepared.push({ itemIndex: index, print_job_id: row.print_job_id, units_allocated: units });
        }
      });
      if (itemTotal > Number(item.quantity)) nextErrors[item.client_id] = t('sales.validation.itemAllocation', { quantity: item.quantity });
    });
    Object.entries(jobTotals).forEach(([jobId, assigned]) => {
      const job = Object.values(jobCache).flatMap((entry) => entry.jobs ?? []).find((candidate) => candidate.id === jobId);
      const available = Number(job?.capacity?.units_available);
      if (!job || !Number.isFinite(available)) nextErrors.general = t('sales.validation.capacityUnavailable');
      else if (assigned > available + (originalByJob[jobId] ?? 0)) nextErrors.general = t('sales.validation.jobCapacity');
    });
    setAllocationErrors(nextErrors);
    return Object.keys(nextErrors).length ? null : prepared;
  };

  const allocationRequest = (prepared, savedItems) => prepared.map((row) => ({ sales_order_item_id: savedItems[row.itemIndex].id, print_job_id: row.print_job_id, units_allocated: row.units_allocated }));

  const formPayload = (data) => ({
    reference: data.reference.trim(), order_date: new Date(`${data.order_date}T00:00:00.000Z`).toISOString(), sales_channel_id: data.sales_channel_id || null,
    fulfilment_status: order?.fulfilment_status ?? 'draft', payment_status: isNew ? data.payment_status : order.payment_status,
    packaging_cost_cents: cents(data.packaging_cost), shipping_cost_cents: cents(data.shipping_cost), platform_fee_cents: cents(data.platform_fee), other_cost_cents: cents(data.other_cost), notes: data.notes?.trim() ?? '',
    items: data.items.map((item) => ({ model_id: item.model_id, quantity: Number(item.quantity), unit_price_cents: cents(item.unit_price) })),
  });

  const onSubmit = async (data) => {
    setSubmitError(null);
    const prepared = validateAllocationRows(data.items);
    if (!prepared) return;
    try {
      const saved = isNew ? await createSalesOrder(formPayload(data)) : await updateSalesOrder(id, formPayload(data));
      await replaceSalesOrderAllocations(saved.id, allocationRequest(prepared, saved.items));
      toast.success(t(isNew ? 'sales.created' : 'sales.updated'));
      if (isNew) navigate(`/sales/${saved.id}`, { replace: true });
      else await load();
    } catch (error) {
      setSubmitError(apiMessage(error, t('sales.saveError')));
    }
  };

  const saveAllocations = async () => {
    const prepared = validateAllocationRows(watchedItems);
    if (!prepared) return;
    setActionLoading(true);
    setSubmitError(null);
    try {
      await replaceSalesOrderAllocations(id, allocationRequest(prepared, order.items));
      toast.success(t('sales.allocationsSaved'));
      await load();
    } catch (error) {
      setSubmitError(apiMessage(error, t('sales.allocationsSaveError')));
    } finally { setActionLoading(false); }
  };

  const changeStatus = async (status) => {
    if (status === 'cancelled' && !window.confirm(t('sales.confirmCancel'))) return;
    setActionLoading(true);
    setSubmitError(null);
    try {
      await updateSalesOrderStatus(id, { fulfilment_status: status });
      toast.success(t('sales.statusUpdated'));
      await load();
    } catch (error) { setSubmitError(apiMessage(error, t('sales.statusError'))); }
    finally { setActionLoading(false); }
  };

  const savePayment = async () => {
    setActionLoading(true);
    setSubmitError(null);
    try {
      await updateSalesOrderStatus(id, { payment_status: paymentStatus });
      toast.success(t('sales.paymentUpdated'));
      await load();
    } catch (error) { setSubmitError(apiMessage(error, t('sales.statusError'))); }
    finally { setActionLoading(false); }
  };

  const removeOrder = async () => {
    if (!window.confirm(t('sales.confirmDelete'))) return;
    setActionLoading(true);
    try { await deleteSalesOrder(id); toast.success(t('sales.deleted')); navigate('/sales'); }
    catch (error) { setSubmitError(apiMessage(error, t('sales.deleteError'))); setActionLoading(false); }
  };

  const preview = useMemo(() => {
    const revenue = watchedItems.reduce((total, item) => total + cents(item?.unit_price) * (Number(item?.quantity) || 0), 0);
    const additional = watchedCosts.reduce((total, value) => total + cents(value), 0);
    return { state: 'not_calculated', revenue_cents: revenue, additional_costs_cents: additional, production_cost_cents: null, total_cost_cents: null, net_profit_cents: null, margin: { state: 'not_calculated' } };
  }, [watchedCosts, watchedItems]);

  if (loading || (!isNew && !order)) return <LoadingState label={t('sales.loadingOrder')} />;
  if (loadError) return <InlineError title={t('sales.orderLoadError')} message={loadError} onRetry={load} />;

  return (
    <div>
      <PageHeader title={isNew ? t('sales.newOrder') : order.reference} description={isNew ? t('sales.newOrderHelp') : t('sales.orderDetailHelp')}>
        <Link to="/sales" className="page-action page-action-secondary">{t('sales.backToOrders')}</Link>
      </PageHeader>
      {submitError && <div className="mb-5 page-form-error" role="alert">{submitError}</div>}
      {actionLoading && <p className="mb-5 flex items-center gap-2 text-sm ui-text-secondary" role="status"><Loader2 className="h-4 w-4 animate-spin" aria-hidden="true" />{t('sales.updatingOrder')}</p>}
      {!isNew && <div className="mb-5 flex flex-wrap items-center gap-4"><SalesOrderStatus fulfilmentStatus={order.fulfilment_status} paymentStatus={order.payment_status} labelled />{order.fulfilment_status === 'completed' && <span className="text-sm ui-text-secondary">{t('sales.completedFrozen')}</span>}</div>}

      {editableDetails ? (
        <form onSubmit={handleSubmit(onSubmit)} noValidate className="space-y-6">
          <section className="page-panel p-5 sm:p-6" aria-labelledby="sales-order-details"><h2 id="sales-order-details" className="text-lg font-semibold ui-text-primary">{t('sales.orderDetails')}</h2>
            <div className="mt-4 grid gap-4 lg:grid-cols-2">
              <label className="text-sm ui-text-secondary">{t('sales.reference')}<input {...register('reference')} maxLength="200" aria-invalid={Boolean(errors.reference)} aria-describedby={errors.reference ? 'sales-reference-error' : undefined} className="mt-1 w-full rounded-md border ui-border-default px-3 py-2" />{fieldError(errors.reference, 'sales-reference-error')}</label>
              <label className="text-sm ui-text-secondary">{t('sales.orderDate')}<input type="date" {...register('order_date')} aria-invalid={Boolean(errors.order_date)} aria-describedby={errors.order_date ? 'sales-order-date-error' : 'sales-order-date-help'} className="mt-1 w-full rounded-md border ui-border-default px-3 py-2" /><span id="sales-order-date-help" className="mt-1 block text-xs ui-text-muted">{t('sales.orderDateFormatHelp')}</span>{fieldError(errors.order_date, 'sales-order-date-error')}</label>
              <div className="text-sm ui-text-secondary">
                <label htmlFor="sales-channel">{t('sales.channelOptional')}</label>
                <div className="mt-1 flex flex-col gap-2 lg:flex-row">
                  <select id="sales-channel" aria-label={t('sales.channel')} {...register('sales_channel_id')} className="min-w-0 flex-1 rounded-md border ui-border-default px-3 py-2">
                    <option value="">{t('sales.noChannel')}</option>
                    {channels.map((channel) => <option key={channel.id} value={channel.id}>{channel.name}{channel.is_archived ? ` (${t('sales.archived').toLowerCase()})` : ''}</option>)}
                  </select>
                  <button type="button" onClick={() => setShowChannelModal(true)} className="page-action page-action-secondary whitespace-nowrap">
                    <Plus className="h-4 w-4" aria-hidden="true" />{t('sales.addChannel')}
                  </button>
                </div>
                {fieldError(errors.sales_channel_id)}
              </div>
              {isNew && <label className="text-sm ui-text-secondary">{t('sales.paymentStatus')}<select {...register('payment_status')} aria-invalid={Boolean(errors.payment_status)} aria-describedby={errors.payment_status ? 'sales-payment-error' : undefined} className="mt-1 w-full rounded-md border ui-border-default px-3 py-2">{PAYMENT.map((status) => <option key={status} value={status}>{t(`sales.payment.${status}`)}</option>)}</select>{fieldError(errors.payment_status, 'sales-payment-error')}</label>}
              <label className="text-sm ui-text-secondary lg:col-span-2">{t('sales.notesOptional')}<textarea rows="3" maxLength="2000" {...register('notes')} aria-invalid={Boolean(errors.notes)} aria-describedby={errors.notes ? 'sales-notes-error' : undefined} className="mt-1 w-full rounded-md border ui-border-default px-3 py-2" />{fieldError(errors.notes, 'sales-notes-error')}</label>
            </div>
          </section>

          <section className="page-panel p-5 sm:p-6" aria-labelledby="sales-items"><div className="flex items-center justify-between gap-3"><h2 id="sales-items" className="text-lg font-semibold ui-text-primary">{t('sales.items')}</h2><button type="button" onClick={() => append(newItem())} className="page-action page-action-secondary"><Plus className="h-4 w-4" />{t('sales.addItem')}</button></div>{typeof errors.items?.message === 'string' && fieldError(errors.items)}
            <div className="mt-4 divide-y ui-divide-subtle">{fields.map((field, index) => {
              const item = watchedItems[index] ?? field;
              const model = models.find((candidate) => candidate.id === item.model_id);
              const lineTotal = cents(item.unit_price) * (Number(item.quantity) || 0);
              const unitCost = unitCostFromPrintJobs(jobCache[item.model_id]?.jobs);
              const suggestion = suggestedUnitPrice(unitCost, item.target_profit_percent);
              return <article key={field.fieldKey} className="py-5 first:pt-0 last:pb-0"><div className="mb-3 flex items-center justify-between gap-3"><h3 className="text-sm font-semibold ui-text-primary">{t('sales.itemNumber', { number: index + 1 })}</h3>{fields.length > 1 && <button type="button" onClick={() => { setAllocations((current) => { const next = { ...current }; delete next[item.client_id]; return next; }); remove(index); }} className="inline-flex min-h-10 items-center gap-2 rounded-md border ui-border-danger px-3 py-2 text-sm ui-text-danger" aria-label={t('sales.removeItem', { number: index + 1 })}><Trash2 className="h-4 w-4" aria-hidden="true" />{t('common.remove')}</button>}</div><div className="grid gap-4 xl:grid-cols-[minmax(14rem,1fr)_7rem_9rem_11rem_9rem]">
                <label className="text-sm ui-text-secondary">{t('sales.model')}<select {...register(`items.${index}.model_id`)} onChange={(event) => { register(`items.${index}.model_id`).onChange(event); setAllocations((current) => ({ ...current, [item.client_id]: [] })); setValue(`items.${index}.unit_price`, ''); }} aria-invalid={Boolean(errors.items?.[index]?.model_id)} aria-describedby={errors.items?.[index]?.model_id ? `sales-item-${index}-model-error` : undefined} className="mt-1 w-full rounded-md border ui-border-default px-3 py-2"><option value="">{t('sales.selectModel')}</option>{models.map((entry) => <option key={entry.id} value={entry.id}>{entry.name}</option>)}</select>{fieldError(errors.items?.[index]?.model_id, `sales-item-${index}-model-error`)}</label>
                <label className="text-sm ui-text-secondary">{t('sales.quantity')}<input type="number" min="1" step="1" {...register(`items.${index}.quantity`)} aria-invalid={Boolean(errors.items?.[index]?.quantity)} aria-describedby={errors.items?.[index]?.quantity ? `sales-item-${index}-quantity-error` : undefined} className="mt-1 w-full rounded-md border ui-border-default px-3 py-2" />{fieldError(errors.items?.[index]?.quantity, `sales-item-${index}-quantity-error`)}</label>
                <div className="text-sm ui-text-secondary">
                  <div className="flex items-center"><label htmlFor={`sales-item-${field.fieldKey}-target`}>{t('sales.targetProfit')}</label><HelpTooltip label={t('sales.targetProfit')}>{t('sales.targetProfitHelp')}</HelpTooltip></div>
                  <input id={`sales-item-${field.fieldKey}-target`} type="number" min="0" step="0.01" {...register(`items.${index}.target_profit_percent`)} aria-invalid={Boolean(errors.items?.[index]?.target_profit_percent)} aria-describedby={errors.items?.[index]?.target_profit_percent ? `sales-item-${index}-target-error` : undefined} className="mt-1 w-full rounded-md border ui-border-default px-3 py-2" />
                  {fieldError(errors.items?.[index]?.target_profit_percent, `sales-item-${index}-target-error`)}
                </div>
                <div className="text-sm ui-text-secondary">
                  <label htmlFor={`sales-item-${field.fieldKey}-price`}>{t('sales.unitPrice')}</label>
                  <input id={`sales-item-${field.fieldKey}-price`} type="number" min="0" step="0.01" {...register(`items.${index}.unit_price`)} aria-invalid={Boolean(errors.items?.[index]?.unit_price)} aria-describedby={errors.items?.[index]?.unit_price ? `sales-item-${index}-price-error` : undefined} className="mt-1 w-full rounded-md border ui-border-default px-3 py-2" />
                  {fieldError(errors.items?.[index]?.unit_price, `sales-item-${index}-price-error`)}
                </div>
                <div className="text-sm ui-text-secondary"><span>{t('sales.lineTotal')}</span><p className="mt-2 min-h-10 rounded-md ui-surface-nested px-3 py-2 font-semibold tabular-nums ui-text-primary">{formatCurrencyFromCents(lineTotal)}</p></div>
              </div>{item.model_id && <div className="mt-4 flex flex-wrap items-center justify-between gap-3 rounded-lg ui-surface-nested px-4 py-3 text-sm"><p className="ui-text-secondary">{unitCost === null ? t('sales.unitCostUnavailable') : t('sales.unitCostValue', { value: formatCurrency(unitCost) })}</p>{suggestion !== null && <div className="flex flex-wrap items-center gap-3"><p className="font-semibold tabular-nums ui-text-primary">{t('sales.suggestedUnitPrice', { value: formatCurrency(suggestion) })}</p><button type="button" onClick={() => setValue(`items.${index}.unit_price`, suggestion, { shouldDirty: true, shouldValidate: true })} className="page-action page-action-secondary">{t('sales.useSuggestedPrice')}</button></div>}</div>}{item.model_id && <SalesAllocationEditor item={{ ...item, model_name: model?.name }} entry={jobCache[item.model_id]} allocations={allocations[item.client_id]} editable={editableAllocations} error={allocationErrors[item.client_id]} onChange={(jobId, value) => changeAllocation(item.client_id, jobId, value)} onRetry={() => loadJobs(item.model_id, true)} defaultOpen={!isNew} />}</article>;
            })}</div>{allocationErrors.general && <p className="mt-4 page-form-error" role="alert">{allocationErrors.general}</p>}
          </section>

          <section className="page-panel p-5 sm:p-6" aria-labelledby="sales-additional-costs"><h2 id="sales-additional-costs" className="text-lg font-semibold ui-text-primary">{t('sales.additionalCosts')}</h2><div className="mt-4 grid gap-4 sm:grid-cols-2 lg:grid-cols-4">{[['packaging_cost', 'packaging'], ['shipping_cost', 'shipping'], ['platform_fee', 'platformFee'], ['other_cost', 'other']].map(([name, key]) => {
            const errorId = `sales-${name.replaceAll('_', '-')}-error`;
            return <label key={name} className="text-sm ui-text-secondary">{t(`sales.${key}`)}<input type="number" min="0" step="0.01" {...register(name)} aria-invalid={Boolean(errors[name])} aria-describedby={errors[name] ? errorId : undefined} className="mt-1 w-full rounded-md border ui-border-default px-3 py-2" />{fieldError(errors[name], errorId)}</label>;
          })}</div></section>
          <SalesFinancialSummary profitability={order?.profitability} fulfilmentStatus={order?.fulfilment_status} preview={preview} />
          <div className="flex justify-end"><button type="submit" disabled={isSubmitting} aria-busy={isSubmitting} className="page-action">{isSubmitting && <Loader2 className="h-4 w-4 animate-spin" />}{t(isSubmitting ? 'common.saving' : isNew ? 'sales.createOrder' : 'common.saveChanges')}</button></div>
        </form>
      ) : (
        <div className="space-y-6">
          <section className="page-panel p-5 sm:p-6"><h2 className="text-lg font-semibold ui-text-primary">{t('sales.orderDetails')}</h2><dl className="mt-4 grid gap-4 sm:grid-cols-2 lg:grid-cols-3"><div><dt className="text-xs ui-text-muted">{t('sales.orderDate')}</dt><dd>{formatDate(order.order_date)}</dd></div><div><dt className="text-xs ui-text-muted">{t('sales.channel')}</dt><dd>{order.sales_channel?.name ?? t('sales.noChannel')}</dd></div><div><dt className="text-xs ui-text-muted">{t('sales.notes')}</dt><dd>{order.notes || t('common.notRecorded')}</dd></div></dl></section>
          <section className="page-panel p-5 sm:p-6"><h2 className="text-lg font-semibold ui-text-primary">{t('sales.items')}</h2><div className="mt-4 divide-y ui-divide-subtle">{watchedItems.map((item, index) => <article key={item.client_id} className="py-5 first:pt-0 last:pb-0"><div className="flex flex-wrap justify-between gap-3"><div><h3 className="font-semibold ui-text-primary">{item.model_name_snapshot}</h3><p className="text-sm ui-text-muted">{t('sales.itemQuantityAndPrice', { quantity: formatNumber(item.quantity), price: formatCurrencyFromCents(order.items[index].unit_price_cents) })}</p></div><div className="text-right"><span className="block text-xs ui-text-muted">{t('sales.lineTotal')}</span><span className="font-semibold tabular-nums ui-text-primary">{formatCurrencyFromCents(order.items[index].unit_price_cents * item.quantity)}</span></div></div><SalesAllocationEditor item={item} entry={jobCache[item.model_id]} allocations={allocations[item.client_id]} editable={editableAllocations} error={allocationErrors[item.client_id]} onChange={(jobId, value) => changeAllocation(item.client_id, jobId, value)} onRetry={() => loadJobs(item.model_id, true)} defaultOpen /></article>)}</div>{allocationErrors.general && <p className="mt-4 page-form-error" role="alert">{allocationErrors.general}</p>}{editableAllocations && <div className="mt-4 flex justify-end"><button type="button" onClick={saveAllocations} disabled={actionLoading} className="page-action">{t('sales.saveAllocations')}</button></div>}</section>
          <section className="page-panel p-5 sm:p-6"><h2 className="text-lg font-semibold ui-text-primary">{t('sales.additionalCosts')}</h2><dl className="mt-4 grid gap-4 sm:grid-cols-2 lg:grid-cols-4">{[['packaging_cost_cents', 'packaging'], ['shipping_cost_cents', 'shipping'], ['platform_fee_cents', 'platformFee'], ['other_cost_cents', 'other']].map(([name, key]) => <div key={name}><dt className="text-xs ui-text-muted">{t(`sales.${key}`)}</dt><dd>{formatCurrencyFromCents(order[name])}</dd></div>)}</dl></section>
          <SalesFinancialSummary profitability={order.profitability} fulfilmentStatus={order.fulfilment_status} />
        </div>
      )}

      {!isNew && <section className="page-panel mt-6 p-5 sm:p-6" aria-labelledby="sales-lifecycle"><h2 id="sales-lifecycle" className="text-lg font-semibold ui-text-primary">{t('sales.lifecycle')}</h2><p className="mt-1 text-sm ui-text-muted">{t('sales.lifecycleHelp')}</p><div className="mt-5 grid gap-5 xl:grid-cols-2"><div><h3 className="text-sm font-semibold ui-text-primary">{t('sales.paymentStatus')}</h3><div className="mt-2 flex flex-wrap items-end gap-3"><label className="sr-only" htmlFor="sales-payment-status">{t('sales.paymentStatus')}</label><select id="sales-payment-status" value={paymentStatus} onChange={(event) => setPaymentStatus(event.target.value)} className="rounded-md border ui-border-default px-3 py-2">{PAYMENT.map((status) => <option key={status} value={status}>{t(`sales.payment.${status}`)}</option>)}</select><button type="button" onClick={savePayment} disabled={actionLoading || paymentStatus === order.payment_status} className="page-action page-action-secondary">{t('sales.updatePayment')}</button></div></div>{NEXT_STATUS[order.fulfilment_status] && <div><h3 className="text-sm font-semibold ui-text-primary">{t('sales.fulfilmentStatus')}</h3><p className="mt-1 text-xs ui-text-muted">{t('sales.nextStatusHelp', { status: t(`sales.fulfilment.${NEXT_STATUS[order.fulfilment_status]}`) })}</p><button type="button" onClick={() => changeStatus(NEXT_STATUS[order.fulfilment_status])} disabled={actionLoading} className="page-action mt-2">{t('sales.moveToStatus', { status: t(`sales.fulfilment.${NEXT_STATUS[order.fulfilment_status]}`) })}</button></div>}</div>{!['completed', 'cancelled'].includes(order.fulfilment_status) && <div className="mt-5 border-t ui-border-subtle pt-4"><h3 className="text-sm font-semibold ui-text-danger">{t(order.fulfilment_status === 'draft' ? 'sales.dangerZone' : 'sales.cancelOrder')}</h3><p className="mt-1 text-xs ui-text-muted">{t(order.fulfilment_status === 'draft' ? 'sales.dangerZoneHelp' : 'sales.cancelHelp')}</p><div className="mt-3 flex flex-wrap gap-3"><button type="button" onClick={() => changeStatus('cancelled')} disabled={actionLoading} className="rounded-md border ui-border-danger px-4 py-2 text-sm ui-text-danger">{t('sales.cancelOrder')}</button>{order.fulfilment_status === 'draft' && <button type="button" onClick={removeOrder} disabled={actionLoading} className="button-danger rounded-md px-4 py-2 text-sm">{t('sales.deleteOrder')}</button>}</div></div>}</section>}
      {showChannelModal && <SalesChannelModal onClose={() => setShowChannelModal(false)} onSaved={(channel) => {
        setChannels((current) => [...current.filter((entry) => entry.id !== channel.id), channel].sort((left, right) => left.name.localeCompare(right.name)));
        setPendingChannelID(channel.id);
        setShowChannelModal(false);
        toast.success(t('sales.channelCreated'));
      }} />}
    </div>
  );
}

export default SalesOrder;
