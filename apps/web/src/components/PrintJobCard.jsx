import { useCallback, useState } from 'react';
import { ChevronDown, Pencil, Printer, Trash2 } from 'lucide-react';
import { useApi } from '../hooks/useApi';
import { getPrintJobCost } from '../services/printJobService';
import { printJobCardContributions } from '../modules';
import { useFeatures } from '../contexts/featureState';
import { filamentName, modelName } from './printJobDisplay';
import { VALUE_FALLBACKS, formatCurrency, formatDate, formatDuration, formatNumber, formatWeight, toFiniteNumber } from '../utils/formatters';
import { useTranslation } from 'react-i18next';

function CostBreakdown({ jobId }) {
  const { t } = useTranslation();
  const fetchCost = useCallback(() => getPrintJobCost(jobId), [jobId]);
  const { data: cost, loading, error, refetch } = useApi(fetchCost);
  if (loading) return <p role="status" className="text-sm ui-text-muted">{t('printJobCard.loadingCosts')}</p>;
  if (error || !cost) return <div role="alert" className="text-sm ui-text-danger">{t('printJobCard.costError')}<button onClick={refetch} className="ml-3 underline font-medium">{t('common.retry')}</button></div>;
  if (cost.state !== 'calculated') return <p className="text-sm ui-text-secondary">{t('printJobCard.notCalculated', { reason: cost.reason ?? t('printJobCard.dataUnavailable') })}</p>;
  return (
    <div>
      <h3 className="text-sm font-semibold ui-text-primary mb-3">{t('dashboard.costBreakdown')}</h3>
      <dl className="space-y-2 text-sm">
        {[
          [t('dashboard.filament'), cost.filament_cost], [t('dashboard.electricity'), cost.electricity_cost],
          [t('dashboard.machineWear'), cost.wear_cost], [t('dashboard.labor'), cost.labor_cost], [t('printJobCard.total'), cost.total_cost],
        ].map(([label, value]) => (
          <div key={label} className={`flex justify-between gap-4 ${label === t('printJobCard.total') ? 'border-t ui-border-subtle pt-2 font-semibold' : 'ui-text-secondary'}`}>
            <dt>{label}</dt><dd className="tabular-nums">{formatCurrency(value, { fallback: VALUE_FALLBACKS.notCalculated })}</dd>
          </div>
        ))}
      </dl>
    </div>
  );
}

export default function PrintJobCard({ job, onEdit, onDelete, deleting, selection }) {
  const { t } = useTranslation();
  const { features } = useFeatures();
  const [expanded, setExpanded] = useState(false);
  const [visited, setVisited] = useState(false);
  const materials = job.filaments ?? [];
  const weights = materials.map((row) => toFiniteNumber(row.weight_used));
  const weight = weights.every((value) => value !== null) ? weights.reduce((total, value) => total + value, 0) : null;
  const name = modelName(job);
  return (
    <article aria-label={t('printJobCard.label', { name })} className="overflow-hidden rounded-xl border ui-border-subtle ui-surface-card shadow-sm">
      <div className="p-5 sm:p-6">
        <div className="flex flex-wrap items-start justify-between gap-4">
          <div className="flex min-w-0 items-start gap-3">
            {selection}
            <div className="rounded-lg ui-tone-primary p-2.5 ui-text-primary-action"><Printer className="h-5 w-5" aria-hidden="true" /></div>
            <div className="min-w-0">
              <h2 className="font-semibold ui-text-primary break-words">{name}</h2>
              <p className="mt-1 text-sm ui-text-muted">{formatDate(job.print_date)}</p>
              <p className="mt-1 text-xs ui-text-secondary">{t('printJobCard.printer')}: {job.printer_cost?.printer_name ?? t('dashboard.notAssigned')}</p>
            </div>
          </div>
          <div className="flex gap-3 text-sm">
            <button onClick={() => onEdit(job)} className="flex items-center gap-1.5 rounded-md px-2 py-1 ui-text-primary-action hover:ui-surface-hover"><Pencil className="h-3.5 w-3.5" aria-hidden="true" />{t('common.edit')}</button>
            <button onClick={() => onDelete(job.id)} disabled={deleting} className="flex items-center gap-1.5 rounded-md px-2 py-1 ui-text-muted hover:ui-surface-hover hover:ui-text-danger disabled:opacity-50"><Trash2 className="h-3.5 w-3.5" aria-hidden="true" />{t(deleting ? 'common.deleting' : 'common.delete')}</button>
          </div>
        </div>
        <dl className="mt-5 grid grid-cols-2 gap-4 sm:grid-cols-4">
          {[
            [t('models.printTime'), formatDuration(job.print_time_minutes)],
            [t('printJobCard.materialUsed'), materials.length ? formatWeight(weight) : VALUE_FALLBACKS.notRecorded],
            [t('dashboard.labor'), `${formatNumber(job.labor_hours, { minimumFractionDigits: 1, maximumFractionDigits: 1, fallback: VALUE_FALLBACKS.notRecorded })} h`],
            [t('dashboard.estimatedCostHeading'), formatCurrency(job.estimated_cost, { fallback: VALUE_FALLBACKS.notCalculated })],
          ].map(([label, value]) => (
            <div key={label}>
              <dt className="text-xs font-medium ui-text-muted">{label}</dt>
              <dd className={`mt-1 text-lg font-semibold tabular-nums ${label === t('dashboard.estimatedCostHeading') ? 'ui-text-primary-action' : 'ui-text-primary'}`}>{value}</dd>
            </div>
          ))}
        </dl>
        {printJobCardContributions(features).map((contribution) => <contribution.Component key={contribution.id} jobId={job.id} unitsPrinted={job.units_printed} />)}
        <div className="mt-5 flex flex-wrap gap-2" aria-label={t('printJobs.filamentsUsed')}>
          {materials.length ? materials.map((row, index) => (
            <span key={row.id ?? index} className="rounded-md ui-surface-input px-2.5 py-1 text-xs ui-text-secondary">
              {filamentName(row)} <span className="ui-text-muted mx-1">·</span> {formatWeight(row.weight_used)}
            </span>
          )) : <p className="text-xs ui-text-muted">{t('printJobCard.noFilaments')}</p>}
        </div>
      </div>
      <div className="border-t ui-border-subtle">
        <button aria-expanded={expanded} aria-controls={`cost-${job.id}`} onClick={() => { setExpanded(!expanded); setVisited(true); }} className="flex w-full items-center justify-between gap-3 px-5 py-3 text-sm font-medium ui-text-primary-action hover:ui-surface-hover sm:px-6">
          {t(expanded ? 'printJobCard.hideCosts' : 'printJobCard.showCosts')}
          <ChevronDown aria-hidden="true" className={`h-4 w-4 transition-transform ${expanded ? 'rotate-180' : ''}`} />
        </button>
        <div id={`cost-${job.id}`} hidden={!expanded} className="border-t ui-border-subtle ui-surface-nested p-5 sm:p-6">
          {visited && <CostBreakdown jobId={job.id} />}
        </div>
      </div>
    </article>
  );
}
