import { useCallback, useEffect, useRef, useState } from 'react';
import { useForm } from 'react-hook-form';
import { yupResolver } from '@hookform/resolvers/yup';
import * as yup from 'yup';
import toast from 'react-hot-toast';
import { Boxes, Package } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { EmptyState, InlineError, LoadingState } from '../components/ui/PageState';
import Modal from '../components/ui/Modal';
import PageHeader from '../components/ui/PageHeader';
import { getFinishedGoods, getMaterials, getMovements, postCount, setThreshold, updateLifecycle, stockWarning } from '../services/inventoryService';

const quantity = yup.number().required().min(0);
const schema = (pieces) => yup.object({ quantity: pieces ? quantity.integer().max(Number.MAX_SAFE_INTEGER) : quantity.max(Number.MAX_SAFE_INTEGER / 1000), reason: yup.string().trim().required().max(512), actor: yup.string().trim().required().max(128) });
const grams = (mg) => `${mg / 1000} g`;

function Reconcile({ item, onSaved, pieces = false }) {
  const { t } = useTranslation();
  const operation = useRef(null);
  const { register, handleSubmit, formState: { errors, isSubmitting } } = useForm({ resolver: yupResolver(schema(pieces)) });
  const submit = async (values) => {
    const payload = { item_type: pieces ? 'model' : 'filament', item_id: item.id, kind: 'count', counted_quantity: pieces ? values.quantity : Math.round(values.quantity * 1000), expected_revision: item.balance.revision, reason: values.reason, actor: values.actor };
    // Preserve identifiers on an identical retry after an uncertain response.
    const signature = JSON.stringify(payload);
    if (operation.current?.signature !== signature) operation.current = { signature, operation_id: crypto.randomUUID(), source_id: crypto.randomUUID() };
    try {
      await postCount({ ...payload, operation_id: operation.current.operation_id, source_id: operation.current.source_id });
      toast.success(t('finishedGoods.saved'));
      await onSaved();
    } catch { toast.error(t('finishedGoods.saveError')); }
  };
  return <form onSubmit={handleSubmit(submit)} className="space-y-3">
    <p className="text-sm ui-text-muted">{t('finishedGoods.help')}</p>
    <label className="block">{t(pieces ? 'finishedGoods.counted' : 'finishedGoods.grams')}<input className="mt-1 block w-full rounded-md border ui-border-default ui-surface-input px-3 py-2 text-sm" type="number" min="0" step={pieces ? "1" : "0.001"} {...register('quantity')} /></label>
    <label className="block">{t('finishedGoods.reason')}<input className="mt-1 block w-full rounded-md border ui-border-default ui-surface-input px-3 py-2 text-sm" {...register('reason')} /></label>
    <label className="block">{t('finishedGoods.operator')}<input className="mt-1 block w-full rounded-md border ui-border-default ui-surface-input px-3 py-2 text-sm" {...register('actor')} /></label>
    {Object.values(errors).map((error, i) => <p role="alert" key={i}>{error.message}</p>)}
    <button className="page-action" disabled={isSubmitting}>{t('finishedGoods.record')}</button>
  </form>;
}

function LifecycleControls({ item, onSaved }) {
  const { t } = useTranslation();
  const [confirm, setConfirm] = useState(false);
  const [saving, setSaving] = useState(false);
  const tracked = item.tracking_enabled !== false;
  const update = async changes => {
    setSaving(true);
    try {
      await updateLifecycle(item.id, changes);
      await onSaved();
    } catch (err) {
      if (err.response?.status === 409) {
        toast.error(t('lifecycle.stale'));
        await onSaved();
      } else {
        toast.error(t('lifecycle.failed'));
      }
    }
    finally { setSaving(false); }
  };
  return <fieldset disabled={saving} className="flex flex-wrap items-center gap-3">
    <legend className="mb-2 font-semibold">{t('lifecycle.controls')}</legend>
    <button className="page-action page-action-secondary" onClick={() => setConfirm(true)}>{t(tracked ? 'lifecycle.pause' : 'lifecycle.resume')}</button>
    <button className="page-action page-action-secondary" onClick={() => update({ warnings_enabled: item.warnings_enabled === false })}>{t(item.warnings_enabled === false ? 'lifecycle.enableWarnings' : 'lifecycle.mute')}</button>
    <button className="page-action page-action-secondary" onClick={() => update({ archived: !item.archived })}>{t(item.archived ? 'lifecycle.unarchive' : 'lifecycle.archive')}</button>
    {confirm && <div className="w-full space-y-2"><p>{t('lifecycle.confirmHelp')}</p>
      <button className="page-action" onClick={() => update({ tracking_enabled: !tracked, expected_revision: item.balance.revision })}>{t('lifecycle.confirm')}</button>
      <button className="page-action page-action-secondary" onClick={() => setConfirm(false)}>{t('common.cancel')}</button></div>}
  </fieldset>;
}

function FinishedGoods() {
  const { t } = useTranslation();
  const [selected, setSelected] = useState(null);
  const [products, setProducts] = useState(null);
  const [error, setError] = useState(false);
  const load = useCallback(async () => {
    setError(false);
    try { setProducts(await getFinishedGoods()); }
    catch { setError(true); }
  }, []);
  useEffect(() => { load(); }, [load]);

  const columns = ['production.model', 'finishedGoods.onHand', 'finishedGoods.reserved', 'finishedGoods.available'];
  return <section className="page-panel overflow-hidden" aria-labelledby="finished-goods-title">
    <div className="flex items-center gap-3 border-b ui-border-subtle px-6 py-4">
      <Boxes className="h-5 w-5 ui-text-secondary" aria-hidden="true" />
      <h2 id="finished-goods-title" className="text-lg font-semibold ui-text-primary">{t('finishedGoods.title')}</h2>
    </div>
    {error ? <InlineError title={t('finishedGoods.loadError')} onRetry={load} />
      : !products ? <LoadingState />
      : products.length === 0 ? <EmptyState icon={Boxes} title={t('finishedGoods.empty')} description={t('finishedGoods.emptyHelp')} />
      : <table className="dashboard-responsive-table w-full text-sm">
          <thead className="ui-surface-nested"><tr>
            {columns.map(column => <th key={column} scope="col" className="px-6 py-3 text-left text-xs font-medium uppercase tracking-wider ui-text-muted">{t(column)}</th>)}
            <th scope="col"><span className="sr-only">{t('common.actions')}</span></th>
          </tr></thead>
          <tbody className="divide-y ui-divide-subtle">{products.map(product => {
            const { id, name, balance } = product;
            return <tr key={id} className="hover:ui-surface-hover">
              <td data-label={t(columns[0])} className="px-6 py-3 font-medium ui-text-primary">{name}</td>
              <td data-label={t(columns[1])} className="px-6 py-3 tabular-nums">{balance.verified ? balance.on_hand : t('finishedGoods.recorded', { count: balance.on_hand })}</td>
              <td data-label={t(columns[2])} className="px-6 py-3 tabular-nums">{balance.reserved}</td>
              <td data-label={t(columns[3])} className="px-6 py-3 tabular-nums">{balance.verified ? balance.available : t('finishedGoods.unknown')}</td>
              <td className="px-6 py-3 text-right"><button className="page-action page-action-secondary" onClick={() => setSelected(product)}>{t('finishedGoods.count', { name })}</button></td>
            </tr>;
          })}</tbody>
        </table>}
    {selected && <Modal title={t('finishedGoods.count', { name: selected.name })} onClose={() => setSelected(null)}>
      <Reconcile pieces item={selected} onSaved={async () => { setSelected(null); await load(); }} />
    </Modal>}
  </section>;
}

export default function Inventory() {
  const { t } = useTranslation();
  const [items, setItems] = useState(null), [error, setError] = useState(false), [search, setSearch] = useState(''), [selected, setSelected] = useState(null), [history, setHistory] = useState([]), [historyError, setHistoryError] = useState(false), [after, setAfter] = useState(0), [threshold, setThresholdValue] = useState('');
  const load = useCallback(async () => { setError(false); try { setItems(await getMaterials()); } catch { setError(true); } }, []);
  useEffect(() => { load(); }, [load]);
  const inspect = async (item, cursor = 0) => {
    if (!cursor) { setSelected(item); setThresholdValue(item.threshold_mg / 1000); setHistory([]); }
    setHistoryError(false);
    try { const rows = await getMovements(item.id, cursor); setHistory((old) => cursor ? [...old, ...rows] : rows); setAfter(rows.length === 100 ? rows.at(-1).sequence : 0); } catch { setHistoryError(true); }
  };
  const saveThreshold = async (event) => { event.preventDefault(); const mg = Math.round(Number(threshold) * 1000); if (threshold === '' || !Number.isSafeInteger(mg) || mg < 0) { toast.error('Enter a valid non-negative threshold'); return; } try { await setThreshold(selected.id, mg); setSelected({ ...selected, threshold_mg: mg }); await load(); toast.success('Threshold saved'); } catch { toast.error('Could not save threshold'); } };
  return <div className="space-y-6">
    <PageHeader icon={Package} title={t('dashboard.inventory')} description={t('finishedGoods.description')} />
    <label className="block">Search materials<input className="ml-3 rounded border p-2" value={search} onChange={(e) => setSearch(e.target.value)} /></label>
    {error ? <div role="alert">Could not load inventory. <button onClick={load}>Retry</button></div> : !items ? <p role="status">Loading inventory…</p> : <>
      {items.length === 0 && <p>No materials yet. Add a filament to begin tracking stock.</p>}
      {items.length > 0 && !items.some((item) => `${item.brand} ${item.type} ${item.color}`.toLowerCase().includes(search.toLowerCase())) && <p>No matching materials.</p>}
      <div className="grid gap-4 md:grid-cols-2">{items.filter((item) => `${item.brand} ${item.type} ${item.color}`.toLowerCase().includes(search.toLowerCase())).map((item) => <article className="page-panel p-5 space-y-2 text-sm ui-text-secondary" key={item.id}>
        <h2 className="text-lg font-semibold ui-text-primary">{item.brand} {item.type} — {item.color}</h2>
        {item.warnings_enabled === false && <p>{t('lifecycle.muted')}</p>}
        {item.archived && <p>{t('lifecycle.archived')}</p>}
        {item.tracking_enabled === false ? <p>{t('lifecycle.untracked')}</p> : <p>On hand: {item.balance.verified ? grams(item.balance.on_hand) : 'Unknown'} · Available: {item.balance.verified ? grams(item.balance.available) : 'Unknown'}</p>}
        {item.tracking_enabled !== false && !item.balance.verified && <p>Recorded changes: {grams(item.balance.on_hand)} (unverified)</p>}
        <p>Warning threshold: {grams(item.threshold_mg)}</p><p>{stockWarning(item)}</p>
        <button className="page-action page-action-secondary" onClick={() => inspect(item)}>{t(item.tracking_enabled === false ? 'lifecycle.review' : 'lifecycle.countAudit', { name: [item.brand, item.color].join(' ') })}</button>
      </article>)}</div>
      {selected && <section className="rounded border p-4 space-y-5" aria-label="Material details">
        <h2>{selected.brand} {selected.type} — {selected.color}</h2><button onClick={() => setSelected(null)}>Close details</button>
        <LifecycleControls key={selected.id} item={selected} onSaved={async () => { setSelected(null); await load(); }} />
        <form onSubmit={saveThreshold}><label>Warning threshold (g)<input className="m-2 rounded border p-2" type="number" min="0" step="0.001" required value={threshold} onChange={(e) => setThresholdValue(e.target.value)} /></label><button className="page-action page-action-secondary">Save threshold</button></form>
        {selected.tracking_enabled !== false && <Reconcile key={`${selected.id}:${selected.balance.revision}`} item={selected} onSaved={async () => { setSelected(null); await load(); }} />}
        <h2>Movement history (g)</h2>{historyError ? <p role="alert">Could not load history. <button onClick={() => inspect(selected)}>Retry history</button></p> : history.length === 0 ? <p>No recorded movements.</p> : <ol>{history.map((row) => <li key={row.sequence}>#{row.sequence} · {row.created_at} · {['tracking_paused', 'tracking_resumed'].includes(row.kind)
          ? t(`lifecycle.${row.kind}`) : `${row.kind} · ${grams(row.quantity)} · ${row.reason}`} · {row.actor}</li>)}</ol>}
        {after > 0 && <button onClick={() => inspect(selected, after)}>Load more movements</button>}
      </section>}
    </>}
    <FinishedGoods />
  </div>;
}
