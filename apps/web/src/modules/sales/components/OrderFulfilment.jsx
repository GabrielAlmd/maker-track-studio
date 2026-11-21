import { useEffect, useState } from 'react';
import { useForm } from 'react-hook-form';
import { yupResolver } from '@hookform/resolvers/yup';
import * as yup from 'yup';
import { useTranslation } from 'react-i18next';
import { InlineError, LoadingState } from '../../../components/ui/PageState';
import useBusinessCommand from './useBusinessCommand';
import { getRecord } from '../services/businessService';

function actionsFor(doc) {
  const actions = [];
  for (const line of doc.lines) {
    const add = (kind, reference, max) => actions.push({ line, kind, reference, max });
    if (!doc.cancelled && line.reservable > 0) add('reserve', line.allocation_id, line.reservable);
    for (const event of line.events) {
      if (event.remaining <= 0) continue;
      if (event.kind === 'reserve' && !doc.cancelled) {
        add('dispatch', event.operation_id, event.remaining);
        add('release', event.operation_id, event.remaining);
      } else if (event.kind === 'dispatch') {
        add('restock', event.operation_id, event.remaining);
        add('reject_return', event.operation_id, event.remaining);
      }
    }
  }
  return actions;
}

function FulfilmentForm({ path, doc, refresh, onCancelled }) {
  const { t } = useTranslation();
  const command = useBusinessCommand();
  const actions = actionsFor(doc);
  const [index, setIndex] = useState(0);
  const selected = actions[index];
  const { register, handleSubmit, formState: { errors } } = useForm({
    resolver: yupResolver(yup.object({
      units: yup.number().integer().min(1).max(selected?.max ?? 0).required(),
      reason: yup.string().trim().required().max(512),
    })), defaultValues: { units: 1, reason: '' },
  });

  async function save(values) {
    const result = await command.run(path, {
      ...values, allocation_id: selected.line.allocation_id, reference: selected.reference,
      expected_revision: selected.line.revision, kind: selected.kind,
    });
    if (result) refresh();
  }

  async function cancel() {
    if (window.confirm(t('shipping.confirmCancel')) &&
      await command.run(path.replace('/fulfilment', '/cancel-remainder'), {}, 'put')) onCancelled();
  }

  return <>
    <p role="status" className="mt-2">{t(`shipping.${doc.status}`)}{doc.cancelled && ` · ${t('sales.fulfilment.cancelled')}`}</p>
    {doc.lines.map((line) => <div key={line.allocation_id} className="mt-3 border-t ui-border-subtle pt-3">
      <h3 className="font-semibold">{line.name} · {line.run_item_id.slice(0, 8)}</h3>
      <p className="text-sm ui-text-secondary">{t('shipping.totals', line)}</p>
      {line.events.length > 0 && <details className="mt-2 text-sm"><summary>{t('business.history')}</summary>
        <ul>{line.events.map((event) => <li key={event.operation_id}>{t(`shipping.${event.kind}`)} · {event.units} · {event.reason}</li>)}</ul>
      </details>}
    </div>)}
    {selected ? <form onSubmit={handleSubmit(save)} className="mt-4 flex flex-wrap items-end gap-3">
      <label>{t('shipping.action')}<select className="block" value={index} onChange={(e) => setIndex(Number(e.target.value))}>
        {actions.map((a, i) => <option value={i} key={a.kind + a.reference}>
          {t(`shipping.${a.kind}`)} · {a.line.name} · {a.reference.slice(0, 8)} ({a.max})
        </option>)}
      </select></label>
      <label>{t('shipping.units')}<input type="number" className="block w-24" min="1" max={selected.max} step="1" required {...register('units')} /></label>
      <label>{t('shipping.reason')}<input className="block" required maxLength={512} {...register('reason')} /></label>
      <button className="page-action" disabled={command.busy}>{t('shipping.record')}</button>
      {Object.keys(errors).length > 0 && <p role="alert">{t('business.invalid')}</p>}
    </form> : <p className="mt-3 text-sm ui-text-muted">{t('shipping.noActions')}</p>}
    <div className="mt-4 flex flex-wrap gap-2">
      <button type="button" className="page-action page-action-secondary" disabled={command.busy} onClick={refresh}>{t('shipping.refresh')}</button>
      {doc.can_cancel && <button type="button" className="page-action page-action-secondary ui-text-danger" disabled={command.busy} onClick={cancel}>{t('shipping.cancel')}</button>}
    </div>
    {command.error && <p role="alert" className="page-form-error">{command.error}</p>}
  </>;
}

export default function OrderFulfilment({ id, onCancelled }) {
  const { t } = useTranslation();
  const path = `orders/${id}/fulfilment`;
  const [doc, setDoc] = useState(null);
  const [error, setError] = useState(false);
  const [request, setRequest] = useState(0);
  function refresh() { setDoc(null); setError(false); setRequest((n) => n + 1); }
  useEffect(() => {
    let current = true;
    getRecord(path).then((result) => { if (current) setDoc(result); })
      .catch(() => { if (current) setError(true); });
    return () => { current = false; };
  }, [path, request]);
  return <section className="page-panel p-5 sm:p-6">
    <h2 className="text-lg font-semibold">{t('shipping.title')}</h2>
    {error ? <InlineError title={t('shipping.loadError')} onRetry={refresh} />
      : doc ? <FulfilmentForm path={path} doc={doc} refresh={refresh} onCancelled={onCancelled} /> : <LoadingState />}
  </section>;
}
