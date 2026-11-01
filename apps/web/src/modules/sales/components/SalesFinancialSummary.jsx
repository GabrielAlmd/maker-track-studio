import { useTranslation } from 'react-i18next';
import { VALUE_FALLBACKS, formatCurrencyFromCents, formatPercentage } from '../../../utils/formatters';

export default function SalesFinancialSummary({ profitability, fulfilmentStatus, preview }) {
  const { t } = useTranslation();
  const values = profitability ?? preview ?? {};
  const calculated = values.state === 'calculated';
  const frozen = fulfilmentStatus === 'completed';
  const money = (value, fallback = VALUE_FALLBACKS.notCalculated) => formatCurrencyFromCents(value, { fallback });

  return (
    <section className="page-panel p-5 sm:p-6" aria-labelledby="sales-financial-summary">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h2 id="sales-financial-summary" className="text-lg font-semibold ui-text-primary">{t('sales.summary')}</h2>
          <p className="mt-1 text-sm ui-text-muted">
            {t(frozen ? 'sales.frozenSummaryHelp' : calculated ? 'sales.liveSummaryHelp' : 'sales.unavailableSummaryHelp')}
          </p>
        </div>
        <span className={`status-badge ${frozen ? 'status-badge-success' : calculated ? 'status-badge-primary' : ''}`}>
          {t(frozen ? 'sales.frozenCost' : calculated ? 'sales.liveCost' : 'sales.unavailableCost')}
        </span>
      </div>
      {values.reason && <p className="mt-3 text-sm ui-text-secondary" role="note">{values.reason}</p>}
      <dl className="sales-financial-grid mt-5">
        {[
          [t('sales.revenue'), money(values.revenue_cents, VALUE_FALLBACKS.dataUnavailable)],
          [t('sales.totalCost'), money(values.total_cost_cents)],
          [t('sales.netProfit'), money(values.net_profit_cents)],
          [t('sales.productionCost'), money(values.production_cost_cents)],
          [t('sales.additionalCosts'), money(values.additional_costs_cents, VALUE_FALLBACKS.dataUnavailable)],
          [t('sales.margin'), values.margin?.state === 'calculated' ? formatPercentage(values.margin.percent, { fallback: VALUE_FALLBACKS.notCalculated }) : VALUE_FALLBACKS.notCalculated],
        ].map(([label, value], index) => (
          <div key={label} className="sales-financial-metric" data-primary={index < 3 ? 'true' : undefined}>
            <dt className="text-xs font-medium uppercase tracking-wide ui-text-muted">{label}</dt>
            <dd className={`mt-1 font-semibold tabular-nums ui-text-primary ${index < 3 ? 'text-xl' : 'text-base'}`}>{value}</dd>
          </div>
        ))}
      </dl>
    </section>
  );
}
