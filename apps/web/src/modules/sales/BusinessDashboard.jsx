import { useCallback, useEffect, useState } from 'react';
import KpiCard from '../../components/ui/KpiCard';
import { Link } from 'react-router-dom';
import { Euro, Percent, ReceiptText, TrendingUp } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { getSalesSummary } from './services/salesService';
import { VALUE_FALLBACKS, formatCurrencyFromCents, formatDate, formatNumber, formatPercentage } from '../../utils/formatters';
import { EmptyState, InlineError, TableSkeleton, StatusBadge } from '../../components/ui/PageState';

const fulfilmentTones = {
  draft: 'neutral', confirmed: 'primary', preparing: 'warning', ready_to_ship: 'warning',
  shipped: 'primary', completed: 'success', cancelled: 'danger',
};
const paymentTones = { unpaid: 'warning', paid: 'success', refunded: 'neutral' };

export default function BusinessDashboard({ DashboardSectionHeading } = {}) {
  const { t } = useTranslation();
  const [summary, setSummary] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);

  const fetchSummary = useCallback(async () => {
    try {
      setLoading(true);
      setError(null);
      setSummary(await getSalesSummary());
    } catch {
      setError(t('dashboard.salesLoadError'));
    } finally {
      setLoading(false);
    }
  }, [t]);

  useEffect(() => { fetchSummary(); }, [fetchSummary]);

  const recentOrders = summary?.recent_orders ?? [];
  const pending = recentOrders.flatMap((order) => {
    const actions = [];
    const status = order.fulfilment_status;
    if (status === 'draft') actions.push(['pendingDraft', 'summary']);
    if (['confirmed', 'preparing'].includes(status)) actions.push(['pendingProduction', 'production']);
    if (['ready_to_ship', 'shipped'].includes(status)) actions.push(['pendingFulfilment', 'summary']);
    if (status !== 'cancelled' && order.payment_status === 'unpaid') actions.push(['pendingPayment', 'financials']);
    return actions.map(([label, section]) => ({ order, label, section }));
  });
  const margin = summary?.margin?.state === 'calculated'
    ? formatPercentage(summary.margin.percent, { fallback: VALUE_FALLBACKS.notCalculated })
    : VALUE_FALLBACKS.notCalculated;

  return <section className="dashboard-business-section" aria-labelledby={DashboardSectionHeading ? "business-dashboard-title" : undefined} aria-label={DashboardSectionHeading ? undefined : t('business.overview')}>
    {DashboardSectionHeading && <DashboardSectionHeading id="business-dashboard-title" title={t('dashboard.business')} description={t('dashboard.businessHelp')} />}
    {error && <div className="mb-4"><InlineError title={t('dashboard.salesLoadError')} onRetry={fetchSummary} /></div>}
    {loading && !summary ? <div className="page-panel"><TableSkeleton columns={4} label={t('dashboard.salesLoading')} /></div> : summary && <>
      <div className="dashboard-kpi-grid">
        <KpiCard label={t('dashboard.orders')} value={formatNumber(summary.active_order_count)} tone="blue" icon={ReceiptText} to="/business/orders" note={t('dashboard.businessOrderNote', { completed: formatNumber(summary.completed_order_count), unpaid: formatNumber(summary.unpaid_order_count) })} />
        <KpiCard label={t('dashboard.revenue')} value={formatCurrencyFromCents(summary.revenue_cents)} tone="cyan" icon={Euro} to="/business/orders" note={t('dashboard.completedOrdersOnly')} />
        <KpiCard label={t('dashboard.netProfit')} value={formatCurrencyFromCents(summary.net_profit_cents)} tone="green" icon={TrendingUp} to="/business/orders" note={t('dashboard.completedOrdersOnly')} />
        <KpiCard label={t('dashboard.margin')} value={margin} tone="green" icon={Percent} to="/business/orders" note={t('dashboard.completedOrdersOnly')} />
      </div>

      {!DashboardSectionHeading && <section className="page-panel p-5" aria-labelledby="pending-actions">
        <h2 id="pending-actions" className="text-lg font-semibold">{t('business.pending')}</h2>
        {pending.length ? <ul className="mt-2 divide-y ui-divide-subtle">{pending.map(({ order, label, section }) =>
          <li key={`${order.id}:${section}`} className="flex flex-wrap items-center justify-between gap-2 py-2">
            <span className="min-w-0 break-words">{order.reference}</span>
            <Link className="page-action page-action-secondary" to={`/business/orders/${order.id}?section=${section}`}>{t(`business.${label}`)}</Link>
          </li>)}</ul> : <p className="mt-2 ui-text-muted">{t('business.noPending')}</p>}
      </section>}
      <section className="page-panel" aria-labelledby="recent-orders-title">
        <div className="flex items-center justify-between border-b ui-border-subtle px-6 py-4">
          <h3 id="recent-orders-title" className="text-lg font-semibold ui-text-primary">{t('dashboard.recentOrders')}</h3>
          <Link to="/business/orders" className="text-sm ui-text-primary-action hover:ui-text-primary-action">{t('dashboard.viewOrders')}</Link>
        </div>
        {recentOrders.length === 0 ? <EmptyState icon={ReceiptText} title={t('dashboard.noRecentOrders')} description={t('dashboard.noRecentOrdersHelp')} action={<Link to="/business/orders/new" className="page-action">{t('dashboard.createOrder')}</Link>} /> : <div className="overflow-x-auto">
          <table className="dashboard-responsive-table min-w-full divide-y ui-divide-subtle">
            <thead className="ui-surface-nested"><tr>
              {[t('sales.reference'), t('sales.channel'), t('sales.orderDate'), t('sales.revenue'), t('sales.fulfilmentStatus'), t('sales.paymentStatus')].map((heading, index) => <th key={heading} className={`px-6 py-3 text-xs font-medium ui-text-muted uppercase tracking-wider ${index === 3 ? 'text-right' : 'text-left'}`}>{heading}</th>)}
            </tr></thead>
            <tbody className="ui-surface-card divide-y ui-divide-subtle">
              {recentOrders.map((order) => <tr key={order.id} className="hover:ui-surface-hover">
                <td data-label={t('sales.reference')} className="px-6 py-4 whitespace-nowrap text-sm font-medium"><Link to={`/business/orders/${order.id}`} className="ui-text-primary-action hover:underline">{order.reference}</Link></td>
                <td data-label={t('sales.channel')} className="px-6 py-4 whitespace-nowrap text-sm ui-text-secondary">{order.sales_channel?.name ?? t('sales.noChannel')}</td>
                <td data-label={t('sales.orderDate')} className="px-6 py-4 whitespace-nowrap text-sm ui-text-secondary">{formatDate(order.order_date)}</td>
                <td data-label={t('sales.revenue')} className="px-6 py-4 whitespace-nowrap text-right text-sm ui-text-secondary tabular-nums">{formatCurrencyFromCents(order.revenue_cents)}</td>
                <td data-label={t('sales.fulfilmentStatus')} className="px-6 py-4 whitespace-nowrap text-sm"><StatusBadge tone={fulfilmentTones[order.fulfilment_status]}>{t(`sales.fulfilment.${order.fulfilment_status}`)}</StatusBadge></td>
                <td data-label={t('sales.paymentStatus')} className="px-6 py-4 whitespace-nowrap text-sm"><StatusBadge tone={paymentTones[order.payment_status]}>{t(`sales.payment.${order.payment_status}`)}</StatusBadge></td>
              </tr>)}
            </tbody>
          </table>
        </div>}
      </section>
    </>}
  </section>;
}
