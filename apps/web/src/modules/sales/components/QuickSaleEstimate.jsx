import { useCallback, useEffect, useRef, useState } from 'react';
import { yupResolver } from '@hookform/resolvers/yup';
import { useForm } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import * as yup from 'yup';
import { useApi } from '../../../hooks/useApi';
import { getFilaments } from '../../../services/filamentService';
import { getMaterials } from '../../../services/inventoryService';
import { getModel } from '../../../services/modelService';
import { getPrinters } from '../../../services/printerService';
import { formatCurrencyFromCents, formatPercentage } from '../../../utils/formatters';
import { estimateRunMaterials } from '../../../utils/productionEstimates';
import { saveRecord } from '../services/businessService';
import { parseHundredths } from '../utils/moneyInput';
import { numberField } from '../utils/businessValidation';
import { EntityField, Field } from './BusinessFields';

const schema = yup.object({
  source: yup.string().oneOf(['manual', 'model']).required(),
  target: yup.string().oneOf(['', 'margin', 'markup']).defined(),
  rate: yup.number().when('target', {
    is: target => Boolean(target),
    then: field => field.transform((_, text) => parseHundredths(text)).integer().min(0).required()
      .test('rate-limit', (value, { parent }) => parent.target !== 'margin' || value < 10000),
    otherwise: field => field.strip(),
  }),
  manual: yup.number().when('source', {
    is: 'manual', then: () => numberField(1, Number.MAX_SAFE_INTEGER).transform((_, text) => parseHundredths(text)),
    otherwise: field => field.strip(),
  }),
  minutes: yup.number().when('source', {
    is: 'model', then: () => numberField(1, 1_000_000), otherwise: field => field.strip(),
  }),
  labor: yup.number().when('source', {
    is: 'model', then: () => numberField(0, 1_000_000, false), otherwise: field => field.strip(),
  }),
  printer: yup.string().transform(value => value || undefined).uuid(),
});

function ModelAssumptions({ model, control, register, errors, onMapping, invalidate }) {
  const { t } = useTranslation();
  const load = useCallback(async () => {
    const [product, filaments, materials, printers] = await Promise.all([
      getModel(model), getFilaments(), getMaterials(), getPrinters(),
    ]);
    const stock = new Map(materials.map(row => [row.id, row]));
    return { product, filaments, stock, printers };
  }, [model]);
  const { data, loading, error, refetch } = useApi(load);

  return <>
    <p className="text-sm ui-text-secondary sm:col-span-2">{t('business.quickModelAssumptions')}</p>
    {loading && <p role="status" className="text-sm sm:col-span-2">{t('business.quickLoadingCost')}</p>}
    {error && <button type="button" className="page-action page-action-secondary sm:col-span-2" onClick={() => { invalidate(); refetch(); }}>{t('business.quickRetryCost')}</button>}
    {data && <EntityField control={control} label="quickPrinter" name="printer" onSelect={invalidate} error={errors.printer} options={[
      ['', t('business.quickAppCosts')], ...data.printers.map(printer => [printer.id, printer.name]),
    ]} />}
    <Field label="quickMinutes" error={errors.minutes} type="number" min={1} max={1_000_000} step={1} {...register('minutes')} />
    <Field label="quickLabor" error={errors.labor} type="number" min={0} max={1_000_000} step="any" {...register('labor')} />
    {data && <button type="button" className="page-action page-action-secondary self-end justify-self-start" onClick={() => onMapping(data)}>
      {t('business.quickPreview')}
    </button>}
  </>;
}

export default function QuickSaleEstimate({ context, quote, onQuote, onApply, finalTotal }) {
  const { t } = useTranslation();
  const { model_id: model, stock_run_item_id: lot, quantity } = context;
  const { control, register, watch, handleSubmit, formState: { errors } } = useForm({
    resolver: yupResolver(schema),
    defaultValues: { source: lot ? 'manual' : 'model', manual: '', target: 'margin', rate: '', minutes: '', labor: 0, printer: '' },
  });
  const [source, target] = watch(['source', 'target']);
  const [error, setError] = useState(null);
  const [busy, setBusy] = useState(false);
  const attempt = useRef(0);

  useEffect(() => () => { attempt.current++; }, []);

  function invalidate() {
    attempt.current++;
    setBusy(false);
    setError(null);
    onQuote(null);
  }

  const field = name => register(name, { onChange: invalidate });
  const preview = data => handleSubmit(async values => {
    const cost = { source: values.source };
    if (values.target) cost.target = { mode: values.target, value: values.rate };
    if (values.source === 'manual') cost.manual_unit_cents = values.manual;
    else {
      const rows = estimateRunMaterials([{ model_id: model, planned: quantity }], [data.product], data.filaments, data.stock);
      cost.filament_mapping = Object.fromEntries(rows.flatMap(row => row.filament_id
        ? row._sources.filter(item => item.requirementID).map(item => [item.requirementID, row.filament_id]) : []));
      Object.assign(cost, { minutes_per_unit: values.minutes, labor_hours_per_unit: values.labor, printer_id: values.printer });
    }
    const request = ++attempt.current;
    onQuote(null);
    setBusy(true);
    setError(null);
    try {
      const pricing = await saveRecord('quick-sales/estimate', { ...context, cost });
      if (request === attempt.current) onQuote({ cost, pricing });
    } catch (failure) {
      if (request === attempt.current) {
        const code = failure?.response?.data?.error?.code;
        setError(code === 'cost_unavailable' ? 'quickMissingCost' : code === 'stock_unavailable' ? 'quickStockChanged' : 'quickEstimateError');
      }
    } finally {
      if (request === attempt.current) setBusy(false);
    }
  });

  const pricing = quote?.pricing;
  const suggested = pricing?.suggested_unit_price_cents;
  const revenue = finalTotal || (suggested == null ? 0 : suggested * quantity);
  const profit = pricing && revenue ? revenue - pricing.total_cost_cents : null;

  return <fieldset disabled={busy} className="space-y-3 rounded-md border ui-border-default p-4">
    <legend className="sr-only">{t('business.quickEstimate')}</legend>
    {lot && <p className="text-sm ui-text-secondary">{t('business.quickLotCostUnavailable')}</p>}
    <div className="grid gap-3 sm:grid-cols-2">
      {!lot && <Field label="quickCostSource" options={[
        ['model', t('business.quickModelCost')], ['manual', t('business.quickManualCost')],
      ]} {...field('source')} />}
      {source === 'manual' && <Field label="quickManualUnit" error={errors.manual} inputMode="decimal" {...field('manual')} />}
      <Field label="quickPriceTarget" {...field('target')} options={[
        ['margin', t('business.quickMargin')], ['markup', t('business.quickMarkup')], ['', t('business.quickCostOnly')],
      ]} />
      {target && <Field label="quickTargetRate" error={errors.rate} inputMode="decimal" {...field('rate')} />}
      {source === 'model' ? <ModelAssumptions model={model} control={control} register={field} errors={errors}
        invalidate={invalidate} onMapping={data => preview(data)()} /> :
        <button type="button" className="page-action page-action-secondary self-end justify-self-start" onClick={() => preview()()}>
          {t('business.quickPreview')}
        </button>}
    </div>
    {error && <p role="alert" className="page-form-error">{t(`business.${error}`)}</p>}
    {busy && <p role="status" className="text-sm">{t('business.quickLoadingCost')}</p>}
    {pricing && <div className="space-y-2 text-sm" aria-live="polite">
      <p>{t('business.quickCostSummary', {
        unit: formatCurrencyFromCents(pricing.unit_cost_cents), total: formatCurrencyFromCents(pricing.total_cost_cents),
      })}</p>
      {suggested != null && <div className="flex flex-wrap items-center gap-2">
        <span>{t('business.quickSuggestion', {
          unit: formatCurrencyFromCents(suggested), total: formatCurrencyFromCents(suggested * quantity),
        })}</span>
        <button type="button" className="page-action page-action-secondary" onClick={() => onApply(suggested)}>
          {t('business.quickApplyPrice')}
        </button>
      </div>}
      {profit != null && <p>{t(finalTotal ? 'business.quickFinalProfit' : 'business.quickSuggestedProfit', {
        profit: formatCurrencyFromCents(profit), margin: formatPercentage(100 * profit / revenue, { fractionDigits: 2 }),
      })}</p>}
    </div>}
  </fieldset>;
}
