import PageHeader from '../components/ui/PageHeader';
import { useCallback, useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import { Boxes, ChevronRight, Euro, FileSpreadsheet, Layers, Package, Printer, ShoppingCart, WalletCards, Weight } from 'lucide-react';
import { getDashboardSummary } from '../services/printJobService';
import { VALUE_FALLBACKS, formatCurrency, formatDate, formatNumber, formatPercentage, formatWeight, isFiniteNumber } from '../utils/formatters';
import { EmptyState, InlineError, TableSkeleton, StatusBadge } from '../components/ui/PageState';
import { useTranslation } from 'react-i18next';
import { useFeatures } from '../contexts/featureState';
import { dashboardContributions } from '../modules';

function KpiCard({ label, value, tone, icon, to, note }) {
  const Icon = icon;
  return <Link to={to} className="dashboard-kpi-card" data-tone={tone}>
    <div className="dashboard-kpi-heading">
      <h2 className="dashboard-kpi-label">{label}</h2>
      <span className="dashboard-kpi-icon" aria-hidden="true"><Icon className="h-4 w-4" /></span>
    </div>
    <p className="dashboard-kpi-value">{value}</p>
    {note && <p className="dashboard-kpi-note">{note}</p>}
  </Link>;
}

function formatLaborHours(value) {
  const formatted = formatNumber(value, { minimumFractionDigits: 1, maximumFractionDigits: 1, fallback: VALUE_FALLBACKS.notRecorded });
  return isFiniteNumber(value) ? `${formatted} h` : formatted;
}

function jobStatus(cost, t) {
  if (cost?.state === 'calculated') return t('dashboard.complete');
  if (cost?.state === 'not_calculated') return t('common.notCalculated');
  return t('dashboard.incomplete');
}

function costPresentation(stats, t) {
  const calculatedJobs = isFiniteNumber(stats?.calculated_jobs) ? Number(stats.calculated_jobs) : null;
  const hasCalculatedCosts = calculatedJobs !== null && calculatedJobs > 0;
  return {
    value: (amount) => hasCalculatedCosts
      ? formatCurrency(amount, { fallback: VALUE_FALLBACKS.notCalculated })
      : VALUE_FALLBACKS.notCalculated,
    note: hasCalculatedCosts
      ? t('dashboard.basedOn', { count: calculatedJobs })
      : t('dashboard.noCalculated'),
  };
}

function CostBreakdown({ stats, formatCost }) {
  const { t } = useTranslation();
  const rows = [
    [t('dashboard.filament'), stats?.total_filament_cost],
    [t('dashboard.electricity'), stats?.total_electricity_cost],
    [t('dashboard.machineWear'), stats?.total_wear_cost],
    [t('dashboard.labor'), stats?.total_labor_cost],
  ];
  return <section className="dashboard-secondary-panel" aria-labelledby="cost-breakdown-title">
    <div className="dashboard-panel-header">
      <h2 id="cost-breakdown-title">{t('dashboard.costBreakdown')}</h2>
      <Euro className="dashboard-panel-icon dashboard-panel-icon-success" aria-hidden="true" />
    </div>
    <dl className="dashboard-breakdown-list">
      {rows.map(([label, amount]) => <div key={label} className="dashboard-breakdown-row">
        <dt>{label}</dt>
        <dd>{formatCost(amount)}</dd>
      </div>)}
    </dl>
  </section>;
}

function InventoryPanel({ summary }) {
  const { t } = useTranslation();
  const rows = [
    { label: t('dashboard.models'), value: summary?.total_models, to: '/models', icon: <Boxes className="h-4 w-4" aria-hidden="true" />, tone: 'blue' },
    { label: t('dashboard.filaments'), value: summary?.total_filaments, to: '/filaments', icon: <Package className="h-4 w-4" aria-hidden="true" />, tone: 'cyan' },
    { label: t('dashboard.purchases'), value: summary?.total_purchases, to: '/purchases', icon: <ShoppingCart className="h-4 w-4" aria-hidden="true" />, tone: 'orange' },
  ];
  return <section className="dashboard-secondary-panel" aria-labelledby="inventory-title">
    <div className="dashboard-panel-header"><h2 id="inventory-title">{t('dashboard.inventory')}</h2></div>
    <div className="dashboard-inventory-list">
      {rows.map(({ label, value, to, icon, tone }) => <Link key={label} to={to} className="dashboard-inventory-row" data-tone={tone}>
        <span className="dashboard-inventory-label">{icon}{label}</span>
        <span className="dashboard-inventory-value">{formatNumber(value, { fallback: VALUE_FALLBACKS.unknown })}<ChevronRight className="h-4 w-4" aria-hidden="true" /></span>
      </Link>)}
    </div>
  </section>;
}

function ExpensesPanel({ expenses }) {
  const { t } = useTranslation();
  const total = isFiniteNumber(expenses?.total_expenses) ? Number(expenses.total_expenses) : 0;
  const rows = [
    [t('dashboard.printerAcquisition'), expenses?.printer_cost],
    [t('dashboard.filamentPurchases'), expenses?.filament_cost],
    [t('dashboard.spareParts'), expenses?.spare_parts_cost],
    [t('dashboard.additionalExpenses'), expenses?.additional_cost],
    [t('dashboard.otherExpenses'), expenses?.other_cost],
  ];
  return <section className="dashboard-expenses-panel" aria-labelledby="expenses-title">
    <div className="dashboard-panel-header">
      <div><h2 id="expenses-title">{t('dashboard.expenses')}</h2><p className="dashboard-panel-help">{t('dashboard.expensesHelp')}</p></div>
      <WalletCards className="dashboard-panel-icon" aria-hidden="true" />
    </div>
    <div className="dashboard-expenses-total"><span>{t('dashboard.totalExpenses')}</span><strong>{formatCurrency(total)}</strong></div>
    {total === 0 ? <EmptyState icon={WalletCards} title={t('dashboard.noExpenses')} description={t('dashboard.noExpensesHelp')} /> : <dl className="dashboard-breakdown-list">
      {rows.map(([label, amount]) => {
        const value = isFiniteNumber(amount) ? Number(amount) : 0;
        return <div key={label} className="dashboard-breakdown-row">
          <dt>{label}</dt>
          <dd><span>{formatCurrency(value)}</span><small>{formatPercentage((value / total) * 100)}</small></dd>
        </div>;
      })}
    </dl>}
  </section>;
}

function DashboardSectionHeading({ id, title, description }) {
  return <div className="dashboard-section-heading">
    <h2 id={id}>{title}</h2>
    {description && <p>{description}</p>}
  </div>;
}

function Dashboard() {
  const { t } = useTranslation();
  const { features } = useFeatures();
  const [summary, setSummary] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);

  const fetchSummary = useCallback(async () => {
    try {
      setLoading(true);
      setError(null);
      setSummary(await getDashboardSummary());
    } catch (err) {
      setError(err?.message || t('dashboard.loadError'));
    } finally {
      setLoading(false);
    }
  }, [t]);
  useEffect(() => { fetchSummary(); }, [fetchSummary]);

  const costStats = summary?.cost_stats;
  const recentJobs = summary?.recent_print_jobs ?? [];
  const costs = costPresentation(costStats, t);
  const isFirstRun = summary && [summary.total_models, summary.total_filaments, summary.total_purchases, summary.total_print_jobs].every((value) => Number(value) === 0);

  return <div>
    <PageHeader title={t('dashboard.title')} description={t('dashboard.description')} />

    {error && <div className="mb-6"><InlineError title={t('dashboard.loadError')} message={summary ? t('common.existingDataSafe') : error} onRetry={fetchSummary} /></div>}
    {loading && !summary ? <div className="page-panel"><TableSkeleton columns={4} label={t('dashboard.loading')} /></div> : <div className="dashboard-content">
      {isFirstRun && <section className="page-panel p-6" aria-labelledby="migration-first-run-title"><div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between"><div className="flex items-start gap-3"><FileSpreadsheet className="mt-0.5 h-6 w-6 ui-text-primary-action" aria-hidden="true" /><div><h2 id="migration-first-run-title" className="font-semibold ui-text-primary">{t('migration.firstRunTitle')}</h2><p className="mt-1 text-sm ui-text-muted">{t('migration.firstRunHelp')}</p></div></div><Link to="/settings?section=migration" className="page-action shrink-0">{t('migration.firstRunAction')}</Link></div></section>}
      <DashboardSectionHeading id="production-dashboard-title" title={t('dashboard.production')} description={t('dashboard.productionHelp')} />
      <section className="dashboard-kpi-grid" aria-label={t('dashboard.overview')}>
        <KpiCard label={t('dashboard.printJobs')} value={formatNumber(summary?.total_print_jobs, { fallback: VALUE_FALLBACKS.unknown })} tone="blue" icon={Printer} to="/print-jobs" />
        <KpiCard label={t('dashboard.unitsPrinted')} value={formatNumber(costStats?.total_units_printed, { fallback: VALUE_FALLBACKS.unknown })} tone="blue" icon={Layers} to="/print-jobs" />
        <KpiCard label={t('dashboard.filamentUsed')} value={formatWeight(costStats?.total_filament_weight_g, { fallback: VALUE_FALLBACKS.unknown })} tone="cyan" icon={Weight} to="/print-jobs" />
        <KpiCard label={t('dashboard.estimatedCost')} value={costs.value(costStats?.total_cost)} tone="green" icon={Euro} to="/print-jobs" note={costs.note} />
      </section>

      {dashboardContributions(features).map((contribution) => <contribution.Component key={contribution.id} KpiCard={KpiCard} DashboardSectionHeading={DashboardSectionHeading} />)}

      <div className="dashboard-secondary-grid">
        <CostBreakdown stats={costStats} formatCost={costs.value} />
        <InventoryPanel summary={summary} />
      </div>

      <ExpensesPanel expenses={summary?.expenses} />

      <section className="page-panel" aria-labelledby="recent-print-jobs-title">
        <div className="flex items-center justify-between border-b ui-border-subtle px-6 py-4">
          <h2 id="recent-print-jobs-title" className="text-lg font-semibold ui-text-primary">{t('dashboard.recent')}</h2>
          <Link to="/print-jobs" className="text-sm ui-text-primary-action hover:ui-text-primary-action">{t('dashboard.viewAll')}</Link>
        </div>
        {recentJobs.length === 0 ? <EmptyState icon={Printer} title={t('dashboard.noJobs')} description={t('dashboard.noJobsHelp')} action={<Link to="/print-jobs" className="page-action">{t('dashboard.addFirst')}</Link>} /> : <div className="overflow-x-auto">
          <table className="dashboard-responsive-table min-w-full divide-y ui-divide-subtle">
            <thead className="ui-surface-nested"><tr>
              {[t('dashboard.model'), t('dashboard.printer'), t('dashboard.date'), t('dashboard.unitsPrinted'), t('dashboard.laborHeading'), t('dashboard.estimatedCostHeading'), t('common.status')].map((heading, index) => <th key={heading} className={`px-6 py-3 text-xs font-medium ui-text-muted uppercase tracking-wider ${[3, 4, 5].includes(index) ? 'text-right' : 'text-left'}`}>{heading}</th>)}
            </tr></thead>
            <tbody className="ui-surface-card divide-y ui-divide-subtle">
              {recentJobs.map((job) => <tr key={job.id} className="hover:ui-surface-hover">
                <td data-label={t('dashboard.model')} className="px-6 py-4 whitespace-nowrap text-sm font-medium ui-text-primary">
                  {job.model?.state === 'assigned' && job.model?.id ? <Link to="/models" className="ui-text-primary-action hover:ui-text-primary-action hover:underline">{job.model.name}</Link> : (job.model?.name ?? job.model_name ?? VALUE_FALLBACKS.dataUnavailable)}
                </td>
                <td data-label={t('dashboard.printer')} className="px-6 py-4 whitespace-nowrap text-sm ui-text-secondary">{job.printer_name ?? t('dashboard.notAssigned')}</td>
                <td data-label={t('dashboard.date')} className="px-6 py-4 whitespace-nowrap text-sm ui-text-secondary">{formatDate(job.print_date)}</td>
                <td data-label={t('dashboard.unitsPrinted')} className="px-6 py-4 whitespace-nowrap text-right text-sm ui-text-secondary tabular-nums">{formatNumber(job.units_printed, { fallback: VALUE_FALLBACKS.dataUnavailable })}</td>
                <td data-label={t('dashboard.laborHeading')} className="px-6 py-4 whitespace-nowrap text-right text-sm ui-text-secondary tabular-nums">{formatLaborHours(job.labor_hours)}</td>
                <td data-label={t('dashboard.estimatedCostHeading')} className="px-6 py-4 whitespace-nowrap text-right text-sm ui-text-secondary tabular-nums">{formatCurrency(job.cost?.total ?? job.estimated_cost, { fallback: VALUE_FALLBACKS.notCalculated })}</td>
                <td data-label={t('common.status')} className="px-6 py-4 whitespace-nowrap text-sm ui-text-secondary"><StatusBadge>{jobStatus(job.cost, t)}</StatusBadge></td>
              </tr>)}
            </tbody>
          </table>
        </div>}
      </section>
    </div>}
  </div>;
}

export default Dashboard;
