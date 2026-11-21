import { useCallback, useState } from 'react';
import { Link, useNavigate, useParams, useSearchParams } from 'react-router-dom';
import { FileText } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import PageHeader from '../../../components/ui/PageHeader';
import { EmptyState, InlineError, LoadingState, StatusBadge } from '../../../components/ui/PageState';
import { useApi } from '../../../hooks/useApi';
import { formatCurrencyFromCents, formatDate, formatPercentage } from '../../../utils/formatters';
import BusinessNavigation from '../components/BusinessNavigation';
import EstimateEditor from '../components/EstimateEditor';
import ProposalEditor from '../components/ProposalEditor';
import ProposalConfirmation from '../components/ProposalConfirmation';
import useBusinessCommand from '../components/useBusinessCommand';
import { getRecord, listRecords } from '../services/businessService';

const pathFor = (kind) => `/business/${kind}`;
const titleFor = (record) => record.terms?.title ?? record.title ?? record.name;

async function allCustomers() {
  const customers = [];
  let page = 1;
  let hasMore = true;
  while (hasMore) {
    const result = await listRecords('customers', page);
    customers.push(...result.items);
    hasMore = result.has_more;
    page += 1;
  }
  return customers;
}

function CommercialList({ kind, proposalSource = false, id }) {
  const { t } = useTranslation();
  const [page, setPage] = useState(1);
  const fetch = useCallback(() => listRecords(kind, page, id ? `/${encodeURIComponent(id)}/revisions` : ''), [kind, page, id]);
  const { data, loading, error, refetch } = useApi(fetch);
  const path = pathFor(kind);
  const newPath = `${path}/new`;
  const create = <Link className="page-action" to={newPath}>{t(`business.new_${kind}`)}</Link>;

  if (loading) return <LoadingState />;
  if (error) return <InlineError title={t('business.loadError')} onRetry={refetch} />;
  if (!data) return null;
  const records = proposalSource ? data.items.filter((record) => !record.is_archived) : data.items;

  return <>
    {!id && <div className="mb-4">{create}</div>}
    {records.length === 0 ? <EmptyState icon={FileText} title={t('business.empty')} /> : <ul className="page-panel divide-y ui-divide-subtle">
      {records.map((record) => <li key={id ? record.revision : record.id} className="flex flex-wrap items-center justify-between gap-3 p-4">
        <div className="min-w-0 break-words">
          <Link className="font-semibold ui-text-primary-action hover:underline"
            to={proposalSource ? `/business/proposals/new?budget=${record.id}` : id ? `${path}/${id}?${record.issued_at ? 'document' : 'revision'}=${record.revision}` : `${path}/${record.id}`}>
            {id ? t('business.revision', { revision: record.revision }) : titleFor(record)}
          </Link>
          {record.customer_name && <p className="text-sm ui-text-muted">{record.customer_name}</p>}
        </div>
        <div className="flex flex-wrap items-center gap-3">
          {kind === 'proposals' && <>
            <StatusBadge tone={record.issued_at ? 'success' : 'neutral'}>{t(record.issued_at ? 'business.issued' : 'sales.fulfilment.draft')}</StatusBadge>
            <span>{formatCurrencyFromCents(record.net_cents)}</span>
          </>}
          {record.is_archived && <StatusBadge>{t('sales.archived')}</StatusBadge>}
          {kind === 'budgets' && <span>{formatCurrencyFromCents(record.estimated_cost_cents)}</span>}
        </div>
      </li>)}
    </ul>}
    {(page > 1 || data.has_more) && <nav aria-label={t('business.pages')} className="mt-4 flex flex-wrap gap-3">
      <button type="button" className="page-action page-action-secondary" disabled={page === 1} onClick={() => setPage(page - 1)}>{t('business.previous')}</button>
      <button type="button" className="page-action page-action-secondary" disabled={!data.has_more} onClick={() => setPage(page + 1)}>{t('business.next')}</button>
    </nav>}
  </>;
}

function ProposalDocument({ document }) {
  const { t } = useTranslation();
  return <article className="page-panel space-y-4 p-5 sm:p-6">
    <h2 className="text-lg font-semibold">{document.terms.title}</h2>
    <p>{document.customer_name}</p>
    <p className="whitespace-pre-wrap break-words">{document.customer_address}</p>
    <dl className="grid gap-3 sm:grid-cols-2">
      <div><dt>{t('business.revision', { revision: document.revision })}</dt><dd>{formatDate(document.issued_at)}</dd></div>
      {['valid_until', 'due_date', 'lead_time_days', 'delivery_terms', 'payment_terms'].map((field) => document.terms[field] ? (
        <div key={field} className="min-w-0 break-words"><dt>{t(`business.term_${field}`)}</dt><dd className="whitespace-pre-wrap">{['valid_until', 'due_date'].includes(field) ? formatDate(document.terms[field]) : document.terms[field]}</dd></div>
      ) : null)}
    </dl>
    <ul className="divide-y ui-divide-subtle">{document.lines.map((line, index) => <li key={index} className="flex flex-wrap justify-between gap-2 py-2">
      <span className="break-words">{line.description} × {line.quantity}</span>
      <span>{formatCurrencyFromCents(line.net_cents)}</span>
    </li>)}</ul>
    <p className="font-semibold">{t('business.documentTotal')} {formatCurrencyFromCents(document.net_cents)}</p>
  </article>;
}

function CommercialDetail({ kind, id, budgetId, documentRevision, revision }) {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const command = useBusinessCommand();
  const proposal = kind === 'proposals';
  const creating = id === 'new';
  const path = creating ? kind : `${kind}/${encodeURIComponent(id)}`;
  const fetch = useCallback(async () => {
    if (documentRevision) return { record: await getRecord(`${path}/document`, documentRevision) };
    const record = creating ? null : await getRecord(path, revision);
    if (revision) return { record };
    const budgetSource = record?.budget_id ?? budgetId;
    const [budget, customers] = await Promise.all([
      proposal && budgetSource ? getRecord(`budgets/${encodeURIComponent(budgetSource)}`, record?.budget_revision) : null,
      kind === 'customers' ? [] : allCustomers(),
    ]);
    return { record, budget, customers };
  }, [budgetId, creating, documentRevision, kind, path, proposal, revision]);
  const { data, loading, error, refetch } = useApi(fetch);

  async function save(values) {
    const result = await command.run(path, { ...values, expected_revision: data.record?.revision ?? 0 }, creating ? 'post' : 'put');
    if (result) {
      if (creating) navigate(`${pathFor(kind)}/${result.id}`, { replace: true });
      else await refetch();
    }
  }

  if (loading) return <LoadingState />;
  if (error) return <InlineError title={t('business.loadError')} onRetry={refetch} />;
  if (!data) return null;
  const { record, budget, customers } = data;

  if (!proposal) return <div className="space-y-4">
    {!creating && <h2 className="text-lg font-semibold">{titleFor(record)}</h2>}
    {kind === 'budgets' && record && !record.is_archived && <Link className="page-action page-action-secondary" to={`/business/proposals/new?budget=${record.id}`}>{t('business.quote')}</Link>}
    <EstimateEditor key={record?.revision} kind={kind} record={record} customers={customers} command={command} onSave={save} />
  </div>;

  if (documentRevision) return <div className="space-y-4">
    <Link className="page-action page-action-secondary" to={`${pathFor(kind)}/${id}`}>{t('business.backToProposal')}</Link>
    <ProposalDocument document={record} />
    <ProposalConfirmation document={record} />
  </div>;

  if (creating && !budget) return <CommercialList kind="budgets" proposalSource />;

  return <div className="space-y-4">
    {record && <section className="page-panel space-y-3 p-5 sm:p-6">
      <h2 className="break-words text-lg font-semibold">{titleFor(record)}</h2>
      <StatusBadge tone={record.issued_at ? 'success' : 'neutral'}>{t(record.issued_at ? 'business.issued' : 'sales.fulfilment.draft')}</StatusBadge>
      <p>{t('business.revision', { revision: record.revision })} · {record.customer_name}</p>
      <dl className="grid gap-3 sm:grid-cols-2 lg:grid-cols-5">
        {[
          ['estimate', formatCurrencyFromCents(record.estimated_cost_cents)],
          ['net', formatCurrencyFromCents(record.net_cents)],
          ['profit', formatCurrencyFromCents(record.profit_cents)],
          ['margin', formatPercentage(record.margin_percent)],
          ['markup', formatPercentage(record.markup_percent)],
        ].map(([label, value]) => <div key={label}><dt>{t(`business.${label}`)}</dt><dd className="font-semibold">{value}</dd></div>)}
      </dl>
      {revision ? <Link className="page-action page-action-secondary" to={`${pathFor(kind)}/${id}`}>{t('business.backToProposal')}</Link>
        : record.issued_at ? <Link className="page-action page-action-secondary" to={`?document=${record.revision}`}>{t('business.viewDocument')}</Link>
          : <button type="button" className="page-action" disabled={command.busy || !record.terms.valid_until} onClick={async () => {
            if (await command.run(`${path}/issue`, { expected_revision: record.revision })) await refetch();
          }}>{t('business.issue')}</button>}
      {!revision && !record.issued_at && !record.terms.valid_until && <p className="text-sm ui-text-muted">{t('business.validityRequired')}</p>}
    </section>}
    {!revision && <ProposalEditor key={record?.revision ?? budget.id} record={record} budget={budget} customers={customers} command={command} onSave={save} />}
    {!revision && record && <section className="space-y-3">
      <h2 className="text-lg font-semibold">{t('business.history')}</h2>
      <CommercialList kind="proposals" id={id} />
    </section>}
  </div>;
}

function parseRevision(value) {
  return /^\d+$/.test(value ?? '') && Number.isSafeInteger(Number(value)) && Number(value) > 0 ? Number(value) : undefined;
}

export default function CommercialRecords({ kind }) {
  const { t } = useTranslation();
  const { id } = useParams();
  const [params] = useSearchParams();
  const documentRevision = kind === 'proposals' && id && id !== 'new' ? parseRevision(params.get('document')) : undefined;
  const revision = kind === 'proposals' && id && id !== 'new' ? parseRevision(params.get('revision')) : undefined;
  const budgetId = kind === 'proposals' && id === 'new' ? params.get('budget') : null;

  return <>
    <PageHeader title={t(`business.${kind}`)}>
      {id && <Link className="page-action page-action-secondary" to={pathFor(kind)}>{t('business.backToList')}</Link>}
    </PageHeader>
    <BusinessNavigation />
    {id ? <CommercialDetail key={JSON.stringify([kind, id, budgetId, documentRevision, revision])} {...{ kind, id, budgetId, documentRevision, revision }} />
      : <CommercialList key={kind} kind={kind} />}
  </>;
}
