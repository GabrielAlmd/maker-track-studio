import ReorderLink from '../components/ReorderLink';
import { useCallback, useEffect, useId, useRef, useState } from 'react';
import { useForm } from 'react-hook-form';
import { yupResolver } from '@hookform/resolvers/yup';
import * as yup from 'yup';
import toast from 'react-hot-toast';
import { Boxes, Package } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { EmptyState, InlineError, LoadingState, StatusBadge } from '../components/ui/PageState';
import Modal from '../components/ui/Modal';
import ListToolbar from '../components/ui/ListToolbar';
import { usePaginatedList } from '../hooks/usePaginatedList';
import { formatDate, formatNumber } from '../utils/formatters';
import PageHeader from '../components/ui/PageHeader';
import { getFinishedGoods, getMaterialPage, getMovements, postCount, setThreshold, updateLifecycle, stockWarning } from '../services/inventoryService';

const maxGrams = Number.MAX_SAFE_INTEGER / 1000;
const inputClass = 'mt-1 block w-full rounded-md border ui-border-default ui-surface-input px-3 py-2 text-sm';
const grams = (mg, signed = false) => `${formatNumber(mg / 1000, { maximumFractionDigits: 3, signDisplay: signed ? 'exceptZero' : 'auto' })}\u00a0g`;
const materialName = ({ brand, type, color }) => `${brand} ${type} — ${color}`;

const quantitySchema = (t, pieces) => {
  const message = t(pieces ? 'inventory.invalidPieces' : 'inventory.invalidGrams');
  const quantity = yup.number().typeError(message).required(message).min(0, message);
  return pieces
    ? quantity.integer(message).max(Number.MAX_SAFE_INTEGER, message)
    : quantity.max(maxGrams, message).test('milligrams', message, value =>
        Number.isSafeInteger(Math.round(value * 1000)) && Math.round(value * 1000) / 1000 === value);
};

function InventoryField({ label, error, ...input }) {
  const id = useId();
  return <div>
    <label className="block text-sm font-medium" htmlFor={id}>{label}</label>
    <input id={id} className={inputClass} aria-invalid={!!error} aria-describedby={error ? `${id}-error` : undefined} {...input} />
    {error && <p id={`${id}-error`} className="mt-1 text-sm ui-text-danger" role="alert">{error.message}</p>}
  </div>;
}

function Reconcile({ item, onSaved, pieces = false }) {
  const { t } = useTranslation();
  const operation = useRef(null);
  const auditField = max => yup.string().trim().required(t('inventory.fieldRequired')).test(
    'byte-length', t('inventory.fieldTooLong', { max }), value => new TextEncoder().encode(value).length <= max);
  const schema = yup.object({ quantity: quantitySchema(t, pieces), reason: auditField(512), actor: auditField(128) });
  const { register, handleSubmit, formState: { errors, isSubmitting } } = useForm({ resolver: yupResolver(schema) });
  const submit = async (values) => {
    const payload = { item_type: pieces ? 'model' : 'filament', item_id: item.id, kind: 'count', counted_quantity: pieces ? values.quantity : Math.round(values.quantity * 1000), expected_revision: item.balance.revision, reason: values.reason, actor: values.actor };
    // Preserve identifiers on an identical retry after an uncertain response.
    const signature = JSON.stringify(payload);
    if (operation.current?.signature !== signature) operation.current = { signature, operation_id: crypto.randomUUID(), source_id: crypto.randomUUID() };
    try {
      await postCount({ ...payload, operation_id: operation.current.operation_id, source_id: operation.current.source_id });
      toast.success(t('finishedGoods.saved'));
      onSaved();
    } catch { toast.error(t('finishedGoods.saveError')); }
  };
  return <form noValidate onSubmit={handleSubmit(submit)} className="space-y-3">
    <p className="text-sm ui-text-muted">{t('finishedGoods.help')}</p>
    <InventoryField label={t(pieces ? 'finishedGoods.counted' : 'finishedGoods.grams')} error={errors.quantity}
      type="number" min="0" max={pieces ? Number.MAX_SAFE_INTEGER : maxGrams} step={pieces ? '1' : '0.001'} {...register('quantity')} />
    <InventoryField label={t('finishedGoods.reason')} error={errors.reason} maxLength={512} {...register('reason')} />
    <InventoryField label={t('finishedGoods.operator')} error={errors.actor} maxLength={128} {...register('actor')} />
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
              <td data-label={t(columns[1])} className="px-6 py-3 tabular-nums">{balance.verified ? formatNumber(balance.on_hand) : t('finishedGoods.recorded', { count: balance.on_hand, formattedCount: formatNumber(balance.on_hand) })}</td>
              <td data-label={t(columns[2])} className="px-6 py-3 tabular-nums">{formatNumber(balance.reserved)}</td>
              <td data-label={t(columns[3])} className="px-6 py-3 tabular-nums">{balance.verified ? formatNumber(balance.available) : t('finishedGoods.unknown')}</td>
              <td className="px-6 py-3 text-right"><button className="page-action page-action-secondary" onClick={() => setSelected({ ...product })}>{t('finishedGoods.count', { name })}</button></td>
            </tr>;
          })}</tbody>
        </table>}
    {selected && <Modal title={t('finishedGoods.count', { name: selected.name })} onClose={() => setSelected(null)}>
      <Reconcile pieces item={selected} onSaved={() => { setSelected(current => current === selected ? null : current); load(); }} />
    </Modal>}
  </section>;
}

const materialColumns = ['inventory.material', 'inventory.onHand', 'finishedGoods.reserved', 'finishedGoods.available', 'inventory.stockState'];
const catalogueFields = ['brand', 'type', 'color'];
const stockStates = ['tracked', 'untracked', 'archived', 'unverified', 'low_stock'];
const lifecycleKinds = new Set(['tracking_paused', 'tracking_resumed']);
const movementKinds = new Set(['opening', 'count', 'adjustment', 'purchase', 'consumption', 'compensation']);
const movementColumns = ['date', 'quantity', 'source', 'reason', 'actor'];

function MaterialStatus({ item }) {
  const { t } = useTranslation();
  const warning = stockWarning(item);
  return <div className="flex flex-wrap justify-end gap-1.5 sm:justify-start">
    {item.tracking_enabled === false
      ? <StatusBadge>{t('lifecycle.untracked')}</StatusBadge>
      : <StatusBadge tone={item.balance.verified ? 'success' : 'warning'}>{t(item.balance.verified ? 'inventory.verified' : 'inventory.unverified')}</StatusBadge>}
    {warning && warning !== 'unknown' && <StatusBadge tone="warning">{t(`inventory.warnings.${warning}`)}</StatusBadge>}
    {item.warnings_enabled === false && <StatusBadge>{t('lifecycle.muted')}</StatusBadge>}
    {item.archived && <StatusBadge>{t('lifecycle.archived')}</StatusBadge>}
  </div>;
}

function MaterialsTable({ items, onInspect }) {
  const { t } = useTranslation();
  return <table className="dashboard-responsive-table w-full text-sm">
    <thead className="ui-surface-nested"><tr>
      {materialColumns.map(column => <th key={column} scope="col" className="px-4 py-3 text-left text-xs font-medium ui-text-muted">{t(column)}</th>)}
      <th scope="col"><span className="sr-only">{t('common.actions')}</span></th>
    </tr></thead>
    <tbody className="divide-y ui-divide-subtle">
      {items.map(item => {
        const { balance } = item;
        const name = materialName(item);
        const unknown = item.tracking_enabled === false ? <span aria-label={t('lifecycle.untracked')}>—</span> : t('inventory.unknown');
        return <tr className="hover:ui-surface-hover" key={item.id}>
          <td data-label={t(materialColumns[0])} className="px-4 py-3 font-medium break-words">{name}</td>
          <td data-label={t(materialColumns[1])} className="px-4 py-3 tabular-nums">
            {balance.verified ? grams(balance.on_hand) : unknown}
          </td>
          <td data-label={t(materialColumns[2])} className="px-4 py-3 tabular-nums">{balance.verified ? grams(balance.reserved) : unknown}</td>
          <td data-label={t(materialColumns[3])} className="px-4 py-3 tabular-nums">{balance.verified ? grams(balance.available) : unknown}</td>
          <td data-label={t(materialColumns[4])} className="px-4 py-3"><MaterialStatus item={item} /></td>
          <td className="px-4 py-3 text-right">
            <button className="page-action page-action-secondary" aria-label={t('inventory.inspectMaterial', { name })} onClick={() => onInspect(item)}>{t('inventory.inspect')}</button>
          </td>
        </tr>;
      })}
    </tbody>
  </table>;
}

function MovementHistory({ itemId }) {
  const { t } = useTranslation();
  const [history, setHistory] = useState([]);
  const [loading, setLoading] = useState(true);
  const [failed, setFailed] = useState(false);
  const [hasMore, setHasMore] = useState(false);
  const request = useRef(null);
  const load = useCallback(async (after) => {
    if (request.current && !request.current.signal.aborted) return;
    const controller = new AbortController();
    request.current = controller;
    setLoading(true);
    setFailed(false);
    try {
      const rows = await getMovements(itemId, after, controller.signal);
      if (!controller.signal.aborted) {
        setHistory(previous => after ? [...previous, ...rows] : rows);
        setHasMore(rows.length === 100);
      }
    } catch {
      if (!controller.signal.aborted) setFailed(true);
    } finally {
      if (!controller.signal.aborted) {
        request.current = null;
        setLoading(false);
      }
    }
  }, [itemId]);

  useEffect(() => {
    load(0);
    return () => request.current?.abort();
  }, [load]);

  return <section className="space-y-3" aria-label={t('inventory.history')}>
    <h3 className="font-semibold">{t('inventory.history')}</h3>
    {history.length > 0 && <table className="dashboard-responsive-table w-full text-sm">
      <thead className="ui-surface-nested"><tr>{movementColumns.map(key =>
        <th scope="col" className="p-2 text-left ui-text-muted" key={key}>{t(`inventory.movement.${key}`)}</th>)}</tr></thead>
      <tbody className="divide-y ui-divide-subtle">{history.map(row => {
        const lifecycle = lifecycleKinds.has(row.kind);
        const sourceType = row.source_key?.split(':')[0];
        const source = lifecycle ? t('lifecycle.controls') : t(`inventory.movement.${movementKinds.has(row.kind) ? row.kind : 'other'}`);
        const values = [
          formatDate(row.created_at, { includeTime: true }),
          grams(row.quantity, true),
          <>{source}{row.source_key && <span className="block break-all text-xs ui-text-muted">{row.source_key}</span>}</>,
          lifecycle ? t(`lifecycle.${row.kind}`) : row.reason === row.source_key && ['purchase', 'material'].includes(sourceType)
            ? t(`inventory.movement.source_${sourceType}`) : row.reason,
          lifecycle && row.actor === 'local operator' ? t('inventory.movement.localOperator')
            : sourceType && row.actor === 'local application' ? t('inventory.movement.localApplication') : row.actor,
        ];
        return <tr key={row.sequence}>{values.map((value, i) =>
          <td key={movementColumns[i]} data-label={t(`inventory.movement.${movementColumns[i]}`)} className="break-words p-2 align-top">{value}</td>)}</tr>;
      })}</tbody>
    </table>}
    {loading && <LoadingState label={t('inventory.loadingHistory')} />}
    {failed && <InlineError title={t('inventory.historyError')} onRetry={() => load(history.at(-1)?.sequence ?? 0)} />}
    {!loading && !failed && !history.length && <p className="text-sm ui-text-muted" role="status">{t('inventory.emptyHistory')}</p>}
    {!loading && !failed && hasMore && <button className="page-action page-action-secondary" onClick={() => load(history.at(-1).sequence)}>{t('inventory.moreHistory')}</button>}
  </section>;
}

function ThresholdForm({ item, onSaved }) {
  const { t } = useTranslation();
  const { register, handleSubmit, formState: { errors, isSubmitting } } = useForm({
    resolver: yupResolver(yup.object({ threshold: quantitySchema(t, false) })),
    defaultValues: { threshold: item.threshold_mg / 1000 },
  });
  const submit = async ({ threshold }) => {
    try {
      await setThreshold(item.id, Math.round(threshold * 1000));
      toast.success(t('inventory.thresholdSaved'));
      onSaved();
    } catch { toast.error(t('inventory.thresholdError')); }
  };
  return <form noValidate onSubmit={handleSubmit(submit)} className="flex flex-wrap items-end gap-3">
    <InventoryField label={t('inventory.threshold')} error={errors.threshold}
      type="number" min="0" max={maxGrams} step="0.001" {...register('threshold')} />
    <button className="page-action page-action-secondary" disabled={isSubmitting}>{t('inventory.saveThreshold')}</button>
  </form>;
}

function MaterialDetails({ item, onClose, onSaved }) {
  const { t } = useTranslation();
  const tracked = item.tracking_enabled !== false;
  return <Modal title={materialName(item)} onClose={onClose} size="wide">
    <div className="space-y-5">
      <MaterialStatus item={item} />
      <ReorderLink url={item.product_url} />
      {tracked && !item.balance.verified && <p className="text-sm ui-text-muted">{t('inventory.recordedChanges', { weight: grams(item.balance.on_hand) })}</p>}
      <LifecycleControls item={item} onSaved={onSaved} />
      <ThresholdForm item={item} onSaved={onSaved} />
      {tracked && <section className="space-y-2">
        <h3 className="font-semibold">{t('inventory.physicalCount')}</h3>
        <Reconcile item={item} onSaved={onSaved} />
      </section>}
      <MovementHistory itemId={item.id} />
    </div>
  </Modal>;
}

function Materials() {
  const { t } = useTranslation();
  const [selected, setSelected] = useState(null);
  const list = usePaginatedList(getMaterialPage, 'brand');
  const { data, page, pageSize, loading, error, setPage, refetch } = list;
  const currentRefetch = useRef(refetch);
  useEffect(() => { currentRefetch.current = refetch; }, [refetch]);
  const total = data?.total ?? 0;
  const outOfRange = page > Math.max(1, Math.ceil(total / pageSize));
  useEffect(() => {
    if (data?.page === page && !loading && !error && outOfRange) setPage(1);
  }, [data, page, loading, error, setPage, outOfRange]);
  const save = (item) => {
    setSelected(current => current === item ? null : current);
    currentRefetch.current();
  };
  return <section className="page-panel" aria-label={t('inventory.materials')}>
    <ListToolbar list={list} nativeSort sorts={catalogueFields.map(value => ({ value, label: t(`filaments.${value}`) }))}>
      <div className="space-y-2">
        {catalogueFields.map(field => <label key={field} className="block text-sm">{t(`filaments.${field}`)}
          <input className={inputClass} type="search" value={list.filters[field] ?? ''}
            onChange={event => list.changeFilters({ ...list.filters, [field]: event.target.value })} />
        </label>)}
        <label className="block text-sm">{t('inventory.stockState')}
          <select className={inputClass} value={list.filters.status ?? ''}
            onChange={event => list.changeFilters({ ...list.filters, status: event.target.value })}>
            <option value="">{t('inventory.allStates')}</option>
            {stockStates.map(state => <option key={state} value={state}>{t(`inventory.filters.${state}`)}</option>)}
          </select>
        </label>
      </div>
    </ListToolbar>
    {list.error ? <InlineError title={t('inventory.loadError')} onRetry={list.refetch} />
      : list.loading || outOfRange ? <LoadingState />
      : total ? <MaterialsTable items={list.data.items} onInspect={item => setSelected({ ...item })} />
      : <EmptyState icon={Package} filtered={list.hasFilters} title={t(list.hasFilters ? 'inventory.noMatches' : 'inventory.empty')}
          description={t(list.hasFilters ? 'inventory.noMatchesHelp' : 'inventory.emptyHelp')} />}
    {selected && <MaterialDetails key={selected.id} item={selected} onClose={() => setSelected(null)} onSaved={() => save(selected)} />}
  </section>;
}

export default function Inventory() {
  const { t } = useTranslation();
  const [view, setView] = useState('materials');
  return <div className="space-y-6">
    <PageHeader icon={Package} title={t('dashboard.inventory')} description={t('finishedGoods.description')} />
    <div role="group" aria-label={t('dashboard.inventory')} className="flex flex-wrap gap-2">
      <button aria-pressed={view === 'materials'} className={`page-action ${view === 'materials' ? '' : 'page-action-secondary'}`} onClick={() => setView('materials')}>{t('inventory.materials')}</button>
      <button aria-pressed={view === 'finished'} className={`page-action ${view === 'finished' ? '' : 'page-action-secondary'}`} onClick={() => setView('finished')}>{t('finishedGoods.title')}</button>
    </div>
    <div hidden={view !== 'materials'}><Materials /></div>
    {view === 'finished' && <FinishedGoods />}
  </div>;
}
