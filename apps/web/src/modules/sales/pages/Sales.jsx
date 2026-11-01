import { useCallback } from 'react';
import { Link } from 'react-router-dom';
import { ReceiptText } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import PageHeader from '../../../components/ui/PageHeader';
import ListToolbar from '../../../components/ui/ListToolbar';
import BulkActions, { RowSelectionCheckbox, SelectAllCheckbox } from '../../../components/ui/BulkActions';
import { EmptyState, InlineError, TableSkeleton } from '../../../components/ui/PageState';
import SalesOrderStatus from '../components/SalesOrderStatus';
import { useApi } from '../../../hooks/useApi';
import { usePaginatedList } from '../../../hooks/usePaginatedList';
import { useBulkSelection } from '../../../hooks/useBulkSelection';
import useMediaQuery from '../../../hooks/useMediaQuery';
import { getModels } from '../../../services/modelService';
import { deleteSalesOrder, getSalesChannels, getSalesOrders } from '../services/salesService';
import { VALUE_FALLBACKS, formatCurrencyFromCents, formatDate, formatNumber } from '../../../utils/formatters';

function itemSummary(order, t) {
  const items = order.items ?? [];
  const units = items.reduce((total, item) => total + (Number(item.quantity) || 0), 0);
  return {
    models: items.map((item) => item.model_name_snapshot).filter(Boolean).join(', ') || VALUE_FALLBACKS.dataUnavailable,
    units: t('sales.unitSummary', { count: items.length, units: formatNumber(units) }),
  };
}

function Sales() {
  const { t } = useTranslation();
  const fetchOrders = useCallback((params) => getSalesOrders(params), []);
  const fetchChannels = useCallback(() => getSalesChannels('all'), []);
  const fetchModels = useCallback(() => getModels(), []);
  const list = usePaginatedList(fetchOrders, 'date', 'desc');
  const { data: orderPage, loading, error, refetch } = list;
  const orders = orderPage?.items;
  const { data: channels } = useApi(fetchChannels);
  const { data: models } = useApi(fetchModels);
  const wideList = useMediaQuery('(min-width: 1280px)');
  const bulk = useBulkSelection({
    items: orders ?? [],
    deleteItem: deleteSalesOrder,
    getLabel: (order) => order.reference,
    onDeleted: refetch,
    failureFallback: t('bulk.failureFallback'),
  });

  return (
    <div>
      <PageHeader title={t('sales.title')} description={t('sales.description')}>
        <Link to="/sales/new" className="page-action">{t('sales.newOrder')}</Link>
      </PageHeader>

      <div className="list-toolbar-panel">
        <ListToolbar
          list={list}
          sorts={[
            { value: 'date', label: t('sales.orderDate') },
            { value: 'reference', label: t('sales.reference') },
            { value: 'created_at', label: t('sales.createdAt') },
            { value: 'updated_at', label: t('sales.updatedAt') },
          ]}
        >
          <div className="space-y-3 text-sm">
            <label className="block">{t('sales.fulfilmentStatus')}
              <select value={list.filters.fulfilment_status || ''} onChange={(event) => list.changeFilters({ ...list.filters, fulfilment_status: event.target.value })} className="mt-1 w-full rounded border p-2">
                <option value="">{t('sales.allFulfilmentStatuses')}</option>
                {['draft', 'confirmed', 'preparing', 'ready_to_ship', 'shipped', 'completed', 'cancelled'].map((status) => <option key={status} value={status}>{t(`sales.fulfilment.${status}`)}</option>)}
              </select>
            </label>
            <label className="block">{t('sales.paymentStatus')}
              <select value={list.filters.payment_status || ''} onChange={(event) => list.changeFilters({ ...list.filters, payment_status: event.target.value })} className="mt-1 w-full rounded border p-2">
                <option value="">{t('sales.allPaymentStatuses')}</option>
                {['unpaid', 'paid', 'refunded'].map((status) => <option key={status} value={status}>{t(`sales.payment.${status}`)}</option>)}
              </select>
            </label>
            <label className="block">{t('sales.channel')}
              <select value={list.filters.sales_channel_id || ''} onChange={(event) => list.changeFilters({ ...list.filters, sales_channel_id: event.target.value })} className="mt-1 w-full rounded border p-2">
                <option value="">{t('sales.allChannels')}</option>
                {(channels ?? []).map((channel) => <option key={channel.id} value={channel.id}>{channel.name}{channel.is_archived ? ` (${t('sales.archived').toLowerCase()})` : ''}</option>)}
              </select>
            </label>
            <label className="block">{t('sales.model')}
              <select value={list.filters.model_id || ''} onChange={(event) => list.changeFilters({ ...list.filters, model_id: event.target.value })} className="mt-1 w-full rounded border p-2">
                <option value="">{t('sales.allModels')}</option>
                {(models ?? []).map((model) => <option key={model.id} value={model.id}>{model.name}</option>)}
              </select>
            </label>
            <label className="block">{t('sales.from')}<input type="date" value={list.filters.date_from || ''} onChange={(event) => list.changeFilters({ ...list.filters, date_from: event.target.value })} className="mt-1 w-full rounded border p-2" /></label>
            <label className="block">{t('sales.to')}<input type="date" value={list.filters.date_to || ''} onChange={(event) => list.changeFilters({ ...list.filters, date_to: event.target.value })} className="mt-1 w-full rounded border p-2" /></label>
          </div>
        </ListToolbar>
        {orders?.length > 0 && <div className="px-4 pt-4"><label className="inline-flex items-center gap-2 text-sm ui-text-secondary"><SelectAllCheckbox bulk={bulk} />{t('bulk.selectAllVisible')}</label></div>}
        <div className="px-4 pt-4"><BulkActions bulk={bulk} /></div>

        {error && <div className="p-4"><InlineError title={t('sales.loadError')} message={orders ? t('common.existingDataSafe') : error} onRetry={refetch} /></div>}
        {loading && !orders ? <TableSkeleton columns={8} label={t('sales.loading')} /> : !orders || orders.length === 0 ? (
          !error && <EmptyState icon={ReceiptText} filtered={list.hasFilters} title={list.hasFilters ? t('common.noResults') : t('sales.empty')} description={list.hasFilters ? t('common.changeSearch') : t('sales.emptyHelp')} action={list.hasFilters ? <button type="button" onClick={list.clear} className="page-action page-action-secondary">{t('common.clearFilters')}</button> : <Link to="/sales/new" className="page-action">{t('sales.createFirst')}</Link>} />
        ) : (
          <>
            {!wideList && <div className="divide-y ui-divide-subtle">
              {orders.map((order) => {
                const summary = itemSummary(order, t);
                const profitability = order.profitability ?? {};
                const frozen = order.fulfilment_status === 'completed';
                return (
                  <article key={order.id} className="p-4 sm:p-5" aria-label={t('sales.orderSummaryLabel', { reference: order.reference })}>
                    <div className="flex flex-wrap items-start justify-between gap-3">
                      <div className="flex items-start gap-3">
                        <RowSelectionCheckbox bulk={bulk} id={order.id} label={order.reference} />
                        <div>
                          <Link to={`/sales/${order.id}`} className="font-semibold ui-text-primary-action hover:underline">{order.reference}</Link>
                          <p className="mt-1 text-sm ui-text-muted">{formatDate(order.order_date)} · {order.sales_channel?.name ?? t('sales.noChannel')}</p>
                        </div>
                      </div>
                      <SalesOrderStatus fulfilmentStatus={order.fulfilment_status} paymentStatus={order.payment_status} />
                    </div>
                    <div className="mt-4 border-t ui-border-subtle pt-4">
                      <p className="text-sm ui-text-primary">{summary.models}</p>
                      <p className="mt-0.5 text-xs ui-text-muted">{summary.units}</p>
                    </div>
                    <dl className="mt-4 grid grid-cols-3 gap-3">
                      <div><dt className="text-xs ui-text-muted">{t('sales.revenue')}</dt><dd className="mt-1 font-semibold tabular-nums">{formatCurrencyFromCents(profitability.revenue_cents)}</dd></div>
                      <div><dt className="text-xs ui-text-muted">{t('sales.totalCost')}</dt><dd className="mt-1 font-semibold tabular-nums">{formatCurrencyFromCents(profitability.total_cost_cents, { fallback: VALUE_FALLBACKS.notCalculated })}</dd><span className="text-xs ui-text-muted">{t('sales.productionCost')}: <span>{formatCurrencyFromCents(profitability.production_cost_cents, { fallback: VALUE_FALLBACKS.notCalculated })}</span></span></div>
                      <div><dt className="text-xs ui-text-muted">{t('sales.netProfit')}</dt><dd className="mt-1 font-semibold ui-text-primary tabular-nums">{formatCurrencyFromCents(profitability.net_profit_cents, { fallback: VALUE_FALLBACKS.notCalculated })}</dd><span className="text-xs ui-text-muted">{t(frozen ? 'sales.frozenCost' : profitability.state === 'calculated' ? 'sales.liveCost' : 'sales.unavailableCost')}</span></div>
                    </dl>
                  </article>
                );
              })}
            </div>}
            {wideList && <div className="overflow-x-auto">
            <table className="min-w-full divide-y ui-divide-subtle">
              <thead className="sticky top-0 z-10 ui-surface-nested">
                <tr>
                  <th scope="col" className="w-12 px-4 py-3"><SelectAllCheckbox bulk={bulk} /></th>
                  <th className="px-4 py-3 text-left text-xs font-medium ui-text-muted uppercase tracking-wider">{t('sales.reference')}</th>
                  <th className="px-4 py-3 text-left text-xs font-medium ui-text-muted uppercase tracking-wider">{t('sales.channel')}</th>
                  <th className="px-4 py-3 text-left text-xs font-medium ui-text-muted uppercase tracking-wider">{t('sales.items')}</th>
                  {[t('sales.revenue'), t('sales.costs'), t('sales.netProfit')].map((heading) => <th key={heading} className="px-4 py-3 text-right text-xs font-medium ui-text-muted uppercase tracking-wider">{heading}</th>)}
                  <th className="px-4 py-3 text-left text-xs font-medium ui-text-muted uppercase tracking-wider">{t('sales.statuses')}</th>
                </tr>
              </thead>
              <tbody className="ui-surface-card divide-y ui-divide-subtle">
                {orders.map((order) => {
                  const summary = itemSummary(order, t);
                  const profitability = order.profitability ?? {};
                  const frozen = order.fulfilment_status === 'completed';
                  return (
                    <tr key={order.id} className="hover:ui-surface-hover">
                      <td className="w-12 px-4 py-4"><RowSelectionCheckbox bulk={bulk} id={order.id} label={order.reference} /></td>
                      <td className="px-4 py-4 text-sm"><Link to={`/sales/${order.id}`} className="font-semibold ui-text-primary-action hover:underline">{order.reference}</Link><span className="mt-1 block whitespace-nowrap text-xs ui-text-muted">{formatDate(order.order_date)}</span></td>
                      <td className="px-4 py-4 whitespace-nowrap text-sm ui-text-secondary">{order.sales_channel?.name ?? t('sales.noChannel')}</td>
                      <td className="px-4 py-4 text-sm ui-text-secondary"><span className="block max-w-52 truncate" title={summary.models}>{summary.models}</span><span className="text-xs ui-text-muted">{summary.units}</span></td>
                      <td className="px-4 py-4 whitespace-nowrap text-right text-sm ui-text-secondary tabular-nums">{formatCurrencyFromCents(profitability.revenue_cents)}</td>
                      <td className="px-4 py-4 whitespace-nowrap text-right text-sm ui-text-secondary tabular-nums" title={profitability.reason || undefined}><span className="font-medium ui-text-primary">{formatCurrencyFromCents(profitability.total_cost_cents, { fallback: VALUE_FALLBACKS.notCalculated })}</span><span className="block text-xs ui-text-muted">{t('sales.productionCost')}: <span>{formatCurrencyFromCents(profitability.production_cost_cents, { fallback: VALUE_FALLBACKS.notCalculated })}</span></span></td>
                      <td className="px-4 py-4 whitespace-nowrap text-right text-sm font-semibold ui-text-primary tabular-nums">{formatCurrencyFromCents(profitability.net_profit_cents, { fallback: VALUE_FALLBACKS.notCalculated })}<span className="block text-xs font-normal ui-text-muted">{t(frozen ? 'sales.frozenCost' : profitability.state === 'calculated' ? 'sales.liveCost' : 'sales.unavailableCost')}</span></td>
                      <td className="px-4 py-4 whitespace-nowrap text-sm"><SalesOrderStatus fulfilmentStatus={order.fulfilment_status} paymentStatus={order.payment_status} /></td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
            </div>}
          </>
        )}
      </div>
    </div>
  );
}

export default Sales;
