import SearchableSelect from '../../../components/ui/SearchableSelect';
import { useEffect, useState } from 'react';
import { useFieldArray, useForm } from 'react-hook-form';
import { yupResolver } from '@hookform/resolvers/yup';
import * as yup from 'yup';
import toast from 'react-hot-toast';
import { useTranslation } from 'react-i18next';
import { InlineError, LoadingState } from '../../../components/ui/PageState';
import { formatDate, formatNumber } from '../../../utils/formatters';
import { getOrderProduction, getOrderProductionSources, saveOrderProduction } from '../services/salesService';

const fieldClass = 'mt-1 w-full rounded-md border ui-border-default px-3 py-2';
const quantity = yup.number().transform((value, input) => input === '' ? 0 : value)
  .integer().min(0).max(1_000_000_000).required();
const schema = yup.object({
  links: yup.array().of(yup.object({ units_planned: quantity, units_allocated: quantity })).max(1000).required(),
});
const metrics = ['required', 'planned', 'accepted', 'allocated', 'outstanding'];

function RunPicker({ orderId, lineId, linked, onAdd }) {
  const { t } = useTranslation();
  const [sources, setSources] = useState([]);
  const [cursor, setCursor] = useState();
  const [selected, setSelected] = useState('');
  const [loaded, setLoaded] = useState(false);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(false);
  const available = sources.filter((source) => !linked.some((link) => link.print_run_item_id === source.print_run_item_id));
  const choice = available.find((source) => source.print_run_item_id === selected);

  async function loadSources() {
    setLoading(true);
    setError(false);
    try {
      const result = await getOrderProductionSources(orderId, lineId, cursor);
      setSources((previous) => [...previous, ...result.items]);
      setCursor(result.next_cursor);
      setLoaded(true);
    } catch {
      setError(true);
    } finally {
      setLoading(false);
    }
  }

  return (
    <div className="mt-3 flex flex-wrap items-end gap-2">
      {loaded && available.length > 0 && <>
        <div className="min-w-0 flex-1 text-sm">
          <SearchableSelect label={t('sales.production.select')} value={choice?.print_run_item_id ?? ''}
            options={available} getKey={source => source.print_run_item_id} onChange={setSelected}
            clearLabel={t('sales.production.choose')} getLabel={source => t('sales.production.source', {
              date: formatDate(source.print_date), run: source.print_run_id.slice(0, 8),
              item: source.print_run_item_id.slice(0, 8), status: t(`production.${source.status}`),
            })} />
        </div>
        <button type="button" className="page-action page-action-secondary" disabled={!choice} onClick={() => {
          onAdd(choice);
          setSelected('');
        }}>{t('sales.production.add')}</button>
      </>}
      {(!loaded || cursor) && <button type="button" className="page-action page-action-secondary" disabled={loading} onClick={loadSources}>
        {t(loading ? 'common.loading' : loaded ? 'sales.production.more' : 'sales.production.find')}
      </button>}
      {loaded && available.length === 0 && !cursor && <p className="text-sm ui-text-muted">{t('sales.production.empty')}</p>}
      {error && <p role="alert" className="page-form-error">{t('sales.production.sourcesError')}</p>}
    </div>
  );
}

export default function OrderProduction({ orderId }) {
  const { t } = useTranslation();
  const [document, setDocument] = useState(null);
  const [loadError, setLoadError] = useState(false);
  const [saveError, setSaveError] = useState('');
  const [request, setRequest] = useState(0);
  const { control, formState: { errors, isDirty, isSubmitting }, handleSubmit, register, reset } = useForm({
    resolver: yupResolver(schema), defaultValues: { links: [] },
  });
  const { fields, append, remove } = useFieldArray({ control, name: 'links' });

  useEffect(() => {
    let current = true;
    getOrderProduction(orderId).then((result) => {
      if (!current) return;
      setDocument(result);
      reset({ links: result.items.flatMap((item) => item.links) });
    }).catch(() => {
      if (current) setLoadError(true);
    });
    return () => { current = false; };
  }, [orderId, request, reset]);

  async function save({ links }) {
    setSaveError('');
    try {
      const payload = links.filter((link) => link.units_planned || link.units_allocated)
        .map(({ sales_order_item_id, print_run_item_id, units_planned, units_allocated }) => ({
          sales_order_item_id, print_run_item_id, units_planned, units_allocated,
        }));
      const result = await saveOrderProduction(orderId, document.revision, payload);
      setDocument(result);
      reset({ links: result.items.flatMap((item) => item.links) });
      toast.success(t('sales.production.saved'));
    } catch (error) {
      const detail = error?.response?.data?.error;
      setSaveError(error?.response?.status === 409 && typeof detail === 'string' ? detail : t('sales.production.saveError'));
    }
  }

  function reload() {
    setLoadError(false);
    setSaveError('');
    setDocument(null);
    setRequest((previous) => previous + 1);
  }

  return (
    <section className="page-panel p-5 sm:p-6" aria-labelledby="order-production">
      <h2 id="order-production" className="text-lg font-semibold ui-text-primary">{t('sales.production.title')}</h2>
      {loadError && <InlineError title={t('sales.production.loadError')} onRetry={reload} />}
      {!document && !loadError && <LoadingState />}
      {document && (
        <form onSubmit={handleSubmit(save)} noValidate className="mt-3">
          <p role="status" className="mb-4 text-sm font-semibold ui-text-primary">
            {t('sales.production.orderStatus', { status: t(`sales.production.states.${document.status}`) })}
          </p>
          <fieldset disabled={isSubmitting} className="min-w-0 divide-y ui-divide-subtle">
            {document.items.map((item) => {
              const linked = fields.map((link, index) => ({ link, index })).filter(({ link }) => link.sales_order_item_id === item.sales_order_item_id);
              return (
                <article key={item.sales_order_item_id} className="py-4 first:pt-0">
                  <div className="flex flex-wrap items-baseline justify-between gap-2">
                    <h3 className="font-semibold">{item.model_name}</h3>
                    <p className="text-sm ui-text-secondary">{t(`sales.production.states.${item.status}`)}</p>
                  </div>
                  <dl className="mt-2 grid grid-cols-2 gap-3 sm:grid-cols-5">
                    {metrics.map((metric) => <div key={metric} className="rounded-md ui-surface-nested px-3 py-2">
                      <dt className="text-xs ui-text-muted">{t(`sales.production.${metric}`)}</dt>
                      <dd className="font-semibold tabular-nums">{formatNumber(item[`units_${metric}`])}</dd>
                    </div>)}
                  </dl>
                  {linked.map(({ link, index }) => {
                    const source = link.source;
                    const rowErrors = errors.links?.[index];
                    const plannedEditable = document.editable && (link.units_planned > 0 || ['planned', 'in_progress'].includes(source.status));
                    const allocatedEditable = document.editable && source.status === 'completed' && source.units_accepted > 0;
                    return (
                      <div key={link.id} className="mt-3 grid items-end gap-3 rounded-md border ui-border-subtle p-3 sm:grid-cols-[minmax(10rem,1fr)_7rem_7rem_auto]">
                        <div className="text-sm">
                          <p title={source.print_job_id}>{t('sales.production.source', {
                            date: formatDate(source.print_date), run: source.print_run_id.slice(0, 8),
                            item: source.print_run_item_id.slice(0, 8), status: t(`production.${source.status}`),
                          })}</p>
                          <p className="ui-text-muted">{t('sales.production.sourceCapacity', {
                            planned: formatNumber(source.units_planned), accepted: formatNumber(source.units_accepted),
                            available: formatNumber(source.units_available),
                          })}</p>
                        </div>
                        <label className="text-sm">{t('sales.production.planned')}
                          <input className={fieldClass} type="number" min="0" max="1000000000" step="1" readOnly={!plannedEditable}
                            aria-invalid={Boolean(rowErrors?.units_planned)} {...register(`links.${index}.units_planned`)} />
                        </label>
                        <label className="text-sm">{t('sales.production.allocated')}
                          <input className={fieldClass} type="number" min="0" max="1000000000" step="1" readOnly={!allocatedEditable}
                            aria-invalid={Boolean(rowErrors?.units_allocated)} {...register(`links.${index}.units_allocated`)} />
                        </label>
                        {document.editable && <button type="button" className="page-action page-action-secondary" onClick={() => remove(index)}>{t('common.remove')}</button>}
                      </div>
                    );
                  })}
                  {document.editable && item.can_link && <RunPicker orderId={orderId} lineId={item.sales_order_item_id}
                    linked={linked.map(({ link }) => link)} onAdd={(source) => append({
                      sales_order_item_id: item.sales_order_item_id, print_run_item_id: source.print_run_item_id,
                      units_planned: 0, units_allocated: 0, source,
                    })} />}
                  {document.editable && !item.can_link && <p className="mt-3 text-sm ui-text-muted">{t('sales.production.modelRequired')}</p>}
                </article>
              );
            })}
          </fieldset>
          {errors.links && <p className="page-form-error" role="alert">{t('sales.production.invalid')}</p>}
          {saveError && <p className="page-form-error" role="alert">{saveError}</p>}
          <div className="mt-3 flex flex-wrap justify-end gap-2">
            <button type="button" className="page-action page-action-secondary" onClick={reload}
              disabled={isSubmitting || (isDirty && !saveError)}>
              {t(saveError ? 'sales.production.reload' : 'sales.production.refresh')}
            </button>
            {document.editable && <button type="submit" className="page-action" disabled={!isDirty || isSubmitting}>
              {t(isSubmitting ? 'common.saving' : 'sales.production.save')}
            </button>}
          </div>
        </form>
      )}
    </section>
  );
}
