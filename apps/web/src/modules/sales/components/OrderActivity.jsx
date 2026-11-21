import { useCallback, useState } from 'react';
import { Link } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { useApi } from '../../../hooks/useApi';
import { InlineError, LoadingState } from '../../../components/ui/PageState';
import { activeIntlLocale } from '../../../utils/formatters';
import { listRecords } from '../services/businessService';

export default function OrderActivity({ orderId }) {
  const { t } = useTranslation();
  const [page, setPage] = useState(1);
  const fetchActivity = useCallback(() => listRecords(`orders/${orderId}/activity`, page), [orderId, page]);
  const { data, loading, error, refetch } = useApi(fetchActivity);
  const timestamp = new Intl.DateTimeFormat(activeIntlLocale(), { dateStyle: 'medium', timeStyle: 'short' });

  return <section className="page-panel p-5 sm:p-6">
    <h2 className="text-lg font-semibold">{t('business.orderActivity')}</h2>
    {loading ? <LoadingState /> : error ? <InlineError title={t('business.loadError')} onRetry={refetch} /> :
      data?.items.length ? <ol className="mt-3 divide-y ui-divide-subtle">
        {data.items.map((event) => <li key={event.id} className="grid gap-1 py-3 sm:grid-cols-[1fr_auto]">
          <div className="min-w-0">
            <p className="font-medium">{t(`business.activityKinds.${event.kind}`, { revision: event.revision, count: event.units })}</p>
            {event.proposal_revision && <Link className="text-sm underline" to={`/business/proposals/${event.reference_id}?document=${event.proposal_revision}`}>
              {t('business.activityProposalRevision', { revision: event.proposal_revision })}
            </Link>}
            {event.detail && <p className="break-words text-sm ui-text-secondary">{event.detail}</p>}
            {event.kind.startsWith('run_') && <p className="break-all text-sm ui-text-secondary">
              {t('business.activityRun', { id: event.reference_id })}
            </p>}
          </div>
          <time className="text-sm ui-text-secondary" dateTime={event.created_at}>{timestamp.format(new Date(event.created_at))}</time>
        </li>)}
      </ol> : <p className="mt-3 ui-text-secondary">{t('business.noActivity')}</p>}
    {(page > 1 || data?.has_more) && <div className="mt-3 flex flex-wrap gap-2">
      <button type="button" className="page-action page-action-secondary" disabled={page <= 1 || loading}
        onClick={() => setPage(page - 1)}>{t('business.newerActivity')}</button>
      <button type="button" className="page-action page-action-secondary" disabled={!data?.has_more || loading || Boolean(error)}
        onClick={() => setPage(page + 1)}>{t('business.olderActivity')}</button>
    </div>}
  </section>;
}
