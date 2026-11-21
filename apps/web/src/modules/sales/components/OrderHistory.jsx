import { useCallback, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useApi } from '../../../hooks/useApi';
import { InlineError, LoadingState } from '../../../components/ui/PageState';
import { formatDate } from '../../../utils/formatters';
import { getRecord } from '../services/businessService';

export default function OrderHistory({ order, latest }) {
  const { t } = useTranslation();
  const [revision, setRevision] = useState(latest?.revision);
  const fetchRevision = useCallback(() => latest && revision !== latest.revision
    ? getRecord(`orders/${order.id}`, revision) : Promise.resolve(latest), [latest, order.id, revision]);
  const { data, loading, error, refetch } = useApi(fetchRevision);

  return <section className="page-panel p-5 sm:p-6">
    <h2 className="text-lg font-semibold">{t('business.history')}</h2>
    {!latest ? <p className="mt-3 ui-text-secondary">{t('business.legacyHistory', { date: formatDate(order.order_date) })}</p> : <>
      <div className="my-4 flex flex-wrap items-center gap-2">
        <button type="button" className="page-action page-action-secondary" disabled={revision <= 1} onClick={() => setRevision(revision - 1)}>{t('business.previousRevision')}</button>
        <span>{t('business.revision', { revision })}</span>
        <button type="button" className="page-action page-action-secondary" disabled={revision >= latest.revision} onClick={() => setRevision(revision + 1)}>{t('business.nextRevision')}</button>
      </div>
      {loading ? <LoadingState /> : error ? <InlineError title={t('business.loadError')} onRetry={refetch} /> : data && <dl className="grid gap-4 sm:grid-cols-2">
        <div><dt>{t('sales.createdAt')}</dt><dd>{formatDate(data.created_at ?? data.confirmed_at)}</dd></div>
        <div><dt>{t('business.reason')}</dt><dd>{data.reason || t('sales.acceptedProposal')}</dd></div>
        <div><dt>{t('sales.purchaseOrder')}</dt><dd>{data.purchase_order || t('common.notRecorded')}</dd></div>
        <div><dt>{t('sales.deliveryTerms')}</dt><dd>{data.delivery_terms || t('common.notRecorded')}</dd></div>
      </dl>}
    </>}
  </section>;
}
