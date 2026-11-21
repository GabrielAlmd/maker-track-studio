import { useCallback, useEffect, useRef, useState } from 'react';
import { useForm } from 'react-hook-form';
import { yupResolver } from '@hookform/resolvers/yup';
import * as yup from 'yup';
import toast from 'react-hot-toast';
import PageHeader from '../components/ui/PageHeader';
import { getMaterials, getMovements, postCount, setThreshold, stockWarning } from '../services/inventoryService';

const schema = yup.object({ grams: yup.number().typeError('Enter grams').required().min(0).max(Number.MAX_SAFE_INTEGER / 1000), reason: yup.string().trim().required().max(512), actor: yup.string().trim().required().max(128) });
const grams = (mg) => `${mg / 1000} g`;

function Reconcile({ item, onSaved }) {
  const operation = useRef(null);
  const { register, handleSubmit, formState: { errors, isSubmitting } } = useForm({ resolver: yupResolver(schema) });
  const submit = async (values) => {
    const payload = { item_type: 'filament', item_id: item.id, kind: 'count', counted_quantity: Math.round(values.grams * 1000), expected_revision: item.balance.revision, reason: values.reason, actor: values.actor };
    // Preserve identifiers on an identical retry after an uncertain response.
    const signature = JSON.stringify(payload);
    if (operation.current?.signature !== signature) operation.current = { signature, operation_id: crypto.randomUUID(), source_id: crypto.randomUUID() };
    try {
      await postCount({ ...payload, operation_id: operation.current.operation_id, source_id: operation.current.source_id });
      toast.success('Physical count recorded');
      await onSaved();
    } catch { toast.error('Count could not be confirmed. Retry, or reload balances if another movement occurred.'); }
  };
  return <form onSubmit={handleSubmit(submit)} className="space-y-3">
    <h2>Reconcile physical stock</h2><p>Count the total on hand. This records an auditable correction; existing movements stay unchanged.</p>
    <label className="block">Counted grams<input className="block rounded border p-2" type="number" step="0.001" {...register('grams')} /></label>
    <label className="block">Reason<input className="block rounded border p-2" {...register('reason')} /></label>
    <label className="block">Operator<input className="block rounded border p-2" {...register('actor')} /></label>
    {Object.values(errors).map((error, i) => <p role="alert" key={i}>{error.message}</p>)}
    <button className="btn-primary" disabled={isSubmitting}>Record count</button>
  </form>;
}

export default function Inventory() {
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
    <PageHeader title="Inventory" description="Physical material stock, counts and audit history" />
    <label className="block">Search materials<input className="ml-3 rounded border p-2" value={search} onChange={(e) => setSearch(e.target.value)} /></label>
    {error ? <div role="alert">Could not load inventory. <button onClick={load}>Retry</button></div> : !items ? <p role="status">Loading inventory…</p> : <>
      {items.length === 0 && <p>No materials yet. Add a filament to begin tracking stock.</p>}
      {items.length > 0 && !items.some((item) => `${item.brand} ${item.type} ${item.color}`.toLowerCase().includes(search.toLowerCase())) && <p>No matching materials.</p>}
      <div className="grid gap-4 md:grid-cols-2">{items.filter((item) => `${item.brand} ${item.type} ${item.color}`.toLowerCase().includes(search.toLowerCase())).map((item) => <article className="rounded border p-4 space-y-2" key={item.id}>
        <h2>{item.brand} {item.type} — {item.color}</h2>
        <p>On hand: {item.balance.verified ? grams(item.balance.on_hand) : 'Unknown'} · Available: {item.balance.verified ? grams(item.balance.available) : 'Unknown'}</p>
        {!item.balance.verified && <p>Recorded changes: {grams(item.balance.on_hand)} (unverified)</p>}
        <p>Warning threshold: {grams(item.threshold_mg)}</p><p>{stockWarning(item)}</p>
        <button className="btn-secondary" onClick={() => inspect(item)}>Count and audit {item.brand} {item.color}</button>
      </article>)}</div>
      {selected && <section className="rounded border p-4 space-y-5" aria-label="Material details">
        <h2>{selected.brand} {selected.type} — {selected.color}</h2><button onClick={() => setSelected(null)}>Close details</button>
        <form onSubmit={saveThreshold}><label>Warning threshold (g)<input className="m-2 rounded border p-2" type="number" min="0" step="0.001" required value={threshold} onChange={(e) => setThresholdValue(e.target.value)} /></label><button className="btn-secondary">Save threshold</button></form>
        <Reconcile key={`${selected.id}:${selected.balance.revision}`} item={selected} onSaved={async () => { setSelected(null); await load(); }} />
        <h2>Movement history (g)</h2>{historyError ? <p role="alert">Could not load history. <button onClick={() => inspect(selected)}>Retry history</button></p> : history.length === 0 ? <p>No recorded movements.</p> : <ol>{history.map((row) => <li key={row.sequence}>#{row.sequence} · {row.created_at} · {row.kind} · {grams(row.quantity)} · {row.reason} · {row.actor}</li>)}</ol>}
        {after > 0 && <button onClick={() => inspect(selected, after)}>Load more movements</button>}
      </section>}
    </>}
  </div>;
}
