import { useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import { useApi } from '../../../hooks/useApi';
import { VALUE_FALLBACKS, formatNumber } from '../../../utils/formatters';
import { getPrintJobAllocations } from '../services/salesService';

export default function AllocationCapacity({ jobId, unitsPrinted }) {
  const { t } = useTranslation();
  const fetchAllocations = useCallback(() => getPrintJobAllocations(jobId), [jobId]);
  const { data, loading, error, refetch } = useApi(fetchAllocations);
  const capacity = data?.print_job;
  const values = [
    [t('printJobCard.unitsPrinted'), capacity?.units_printed ?? unitsPrinted],
    [t('printJobCard.unitsAllocated'), capacity?.units_allocated],
    [t('printJobCard.unitsAvailable'), capacity?.units_available],
  ];

  return (
    <div className="mt-5 rounded-lg ui-surface-nested p-4">
      <dl aria-label={t('printJobCard.productionCapacity')} className="grid grid-cols-3 gap-3">
        {values.map(([label, value]) => <div key={label}>
          <dt className="text-xs font-medium ui-text-muted">{label}</dt>
          <dd className="mt-1 text-lg font-semibold tabular-nums ui-text-primary">{formatNumber(value, { fallback: VALUE_FALLBACKS.dataUnavailable })}</dd>
        </div>)}
      </dl>
      {loading && <p role="status" className="mt-2 text-xs ui-text-muted">{t('printJobCard.loadingAllocations')}</p>}
      {error && <div role="alert" className="mt-2 text-xs ui-text-danger">{t('printJobCard.allocationError')}<button type="button" onClick={refetch} className="ml-2 underline font-medium">{t('common.retry')}</button></div>}
    </div>
  );
}
