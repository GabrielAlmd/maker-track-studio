import { Loader2 } from 'lucide-react';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { formatDate, formatNumber, VALUE_FALLBACKS } from '../../../utils/formatters';
import useMediaQuery from '../../../hooks/useMediaQuery';

export default function SalesAllocationEditor({ item, entry, allocations, editable, error, onChange, onRetry, defaultOpen = false }) {
  const { t } = useTranslation();
  const currentByJob = new Map((allocations ?? []).map((allocation) => [allocation.print_job_id, allocation.units_allocated]));
  const assigned = (allocations ?? []).reduce((total, allocation) => total + (Number(allocation.units_allocated) || 0), 0);
  const wideLayout = useMediaQuery('(min-width: 1280px)');
  const [expanded, setExpanded] = useState(defaultOpen);

  return (
    <details className="mt-4 rounded-lg border ui-border-default ui-surface-nested" open={expanded} onToggle={(event) => setExpanded(event.currentTarget.open)}>
      <summary className="cursor-pointer px-4 py-3" aria-label={`${t('sales.allocationsFor', { model: item.model_name_snapshot || item.model_name || t('sales.item') })}. ${t('sales.allocationProgress', { assigned: formatNumber(assigned), quantity: formatNumber(item.quantity || 0) })}`}>
        <div className="ml-1 inline-flex max-w-[calc(100%-1.5rem)] flex-wrap items-center justify-between gap-x-4 gap-y-1 align-middle">
          <h3 className="inline text-sm font-semibold ui-text-primary">{t('sales.productionAllocations')}</h3>
          <span className="text-xs ui-text-secondary">{t('sales.allocationProgress', { assigned: formatNumber(assigned), quantity: formatNumber(item.quantity || 0) })}</span>
        </div>
      </summary>
      <div className="border-t ui-border-subtle px-4 pb-4">
        <div className="pt-3">
          <p className="text-xs ui-text-muted">{t('sales.allocationHelp', { quantity: item.quantity || 0 })}</p>
        </div>

      {entry?.loading && <p className="mt-4 flex items-center gap-2 text-sm ui-text-muted" role="status"><Loader2 className="h-4 w-4 animate-spin" aria-hidden="true" />{t('sales.loadingCompatibleJobs')}</p>}
      {entry?.error && <div className="mt-4 page-form-error" role="alert">{entry.error}<button type="button" onClick={onRetry} className="ml-2 underline">{t('common.retry')}</button></div>}
      {!entry?.loading && !entry?.error && (entry?.jobs ?? []).length === 0 && <p className="mt-4 text-sm ui-text-muted">{t('sales.noCompatibleJobs')}</p>}
      {error && <p className="mt-3 ui-text-danger text-sm" role="alert">{error}</p>}

      {(entry?.jobs ?? []).length > 0 && (
        <>
          {!wideLayout ? <div className="mt-4 divide-y ui-divide-subtle">
            {entry.jobs.map((job) => {
              const capacity = job.capacity ?? {};
              const current = currentByJob.get(job.id) ?? '';
              return (
                <div key={job.id} className="py-3 first:pt-0 last:pb-0">
                  <div className="flex flex-wrap items-start justify-between gap-3">
                    <div><span className="block text-sm font-medium ui-text-primary">{job.print_date ? formatDate(job.print_date) : VALUE_FALLBACKS.dataUnavailable}</span><span className="text-xs ui-text-muted">{job.id.slice(0, 8)}</span></div>
                    <label className="text-xs ui-text-secondary">{t('sales.currentOrderUnits')}
                      {editable ? <input type="number" min="0" step="1" value={current} onChange={(event) => onChange(job.id, event.target.value)} aria-label={t('sales.allocateToJobCompact', { id: job.id.slice(0, 8) })} className="mt-1 block w-24 rounded border ui-border-default px-2 py-1.5 text-sm" /> : <span className="mt-1 block text-sm tabular-nums ui-text-primary">{formatNumber(current || 0)}</span>}
                    </label>
                  </div>
                  <dl className="mt-3 grid grid-cols-3 gap-3 text-xs"><div><dt className="ui-text-muted">{t('sales.unitsPrinted')}</dt><dd className="mt-1 tabular-nums">{formatNumber(capacity.units_printed ?? job.units_printed, { fallback: VALUE_FALLBACKS.dataUnavailable })}</dd></div><div><dt className="ui-text-muted">{t('sales.unitsAllocated')}</dt><dd className="mt-1 tabular-nums">{formatNumber(capacity.units_allocated, { fallback: VALUE_FALLBACKS.dataUnavailable })}</dd></div><div><dt className="ui-text-muted">{t('sales.unitsAvailable')}</dt><dd className="mt-1 tabular-nums">{formatNumber(capacity.units_available, { fallback: VALUE_FALLBACKS.dataUnavailable })}</dd></div></dl>
                </div>
              );
            })}
          </div> : <div className="mt-4 overflow-x-auto">
          <table className="min-w-full text-sm">
            <thead>
              <tr className="text-left text-xs uppercase tracking-wide ui-text-muted">
                <th className="pb-2 pr-4">{t('sales.printJob')}</th>
                <th className="pb-2 px-3">{t('sales.unitsPrinted')}</th>
                <th className="pb-2 px-3">{t('sales.unitsAllocated')}</th>
                <th className="pb-2 px-3">{t('sales.unitsAvailable')}</th>
                <th className="pb-2 pl-3">{t('sales.currentOrderUnits')}</th>
              </tr>
            </thead>
            <tbody className="divide-y ui-divide-subtle">
              {entry.jobs.map((job) => {
                const capacity = job.capacity ?? {};
                const current = currentByJob.get(job.id) ?? '';
                return (
                  <tr key={job.id}>
                    <td className="py-3 pr-4 ui-text-secondary"><span className="block font-medium ui-text-primary">{job.print_date ? formatDate(job.print_date) : VALUE_FALLBACKS.dataUnavailable}</span><span className="text-xs ui-text-muted">{job.id.slice(0, 8)}</span></td>
                    <td className="py-3 px-3 tabular-nums">{formatNumber(capacity.units_printed ?? job.units_printed, { fallback: VALUE_FALLBACKS.dataUnavailable })}</td>
                    <td className="py-3 px-3 tabular-nums">{formatNumber(capacity.units_allocated, { fallback: VALUE_FALLBACKS.dataUnavailable })}</td>
                    <td className="py-3 px-3 tabular-nums">{formatNumber(capacity.units_available, { fallback: VALUE_FALLBACKS.dataUnavailable })}</td>
                    <td className="py-3 pl-3">
                      {editable ? <input type="number" min="0" step="1" value={current} onChange={(event) => onChange(job.id, event.target.value)} aria-label={t('sales.allocateToJob', { id: job.id.slice(0, 8) })} className="w-24 rounded border ui-border-default px-2 py-1.5" /> : <span className="tabular-nums">{formatNumber(current || 0)}</span>}
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
          </div>}
        </>
      )}
      </div>
    </details>
  );
}
