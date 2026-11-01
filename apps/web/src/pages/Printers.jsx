import { useCallback, useState } from 'react';
import { useForm } from 'react-hook-form';
import { yupResolver } from '@hookform/resolvers/yup';
import * as yup from 'yup';
import toast from 'react-hot-toast';
import { Archive, Check, Loader2, Pencil, Plus, Printer, RotateCcw, Star } from 'lucide-react';
import PageHeader from '../components/ui/PageHeader';
import Modal from '../components/ui/Modal';
import { EmptyState, InlineError, TableSkeleton } from '../components/ui/PageState';
import { useApi } from '../hooks/useApi';
import { COST_SETTINGS, valueError } from './costSettings';
import CostConfigFields from './CostConfigFields';
import {
  archivePrinter, createPrinter, getPrinters, restorePrinter,
  setDefaultPrinter, updatePrinter, updatePrinterCostConfig,
} from '../services/printerService';
import { useTranslation } from 'react-i18next';
import { A1_REFERENCE_PROFILE, PRINTER_PRESETS } from './printerPresets';

const metadataSchema = (t) => yup.object({
  name: yup.string().trim().required(t('validation.nameRequired')).max(100, t('validation.max100')),
  manufacturer: yup.string().trim().required(t('validation.manufacturerRequired')).max(100, t('validation.max100')),
  model: yup.string().trim().required(t('validation.modelRequired')).max(100, t('validation.max100')),
  notes: yup.string().trim().max(2000, t('validation.max2000')),
});

const costKeys = Object.keys(COST_SETTINGS);
const costSchema = (t) => yup.object(Object.fromEntries(costKeys.map((key) => [key,
  yup.string().required(t('validation.valueRequired')).test('cost', function validate(value) {
    const message = valueError(key, value);
    return message ? this.createError({ message }) : true;
  }),
])));

function PresetPicker({ preset, onSelect, onApply, allowComplete = false }) {
  const { t, i18n } = useTranslation();
  const [query, setQuery] = useState('');
  const matchingPresets = PRINTER_PRESETS.filter(item => `${item.manufacturer} ${item.model}`.toLowerCase().includes(query.trim().toLowerCase()));
  const visiblePresets = preset && !matchingPresets.some(item => item.id === preset.id) ? [...matchingPresets, preset] : matchingPresets;
  return <section className="rounded-xl border ui-border-subtle p-4 space-y-3">
    <div><div className="flex flex-wrap items-center gap-2"><h3 className="font-semibold">{t('printers.presetTitle')}</h3><span className="rounded-full ui-tone-primary px-2 py-0.5 text-xs font-medium">{t('printers.previewCatalog')}</span></div><p className="text-sm ui-text-secondary">{t('printers.presetHelp')}</p></div>
    <label className="block text-sm font-medium" htmlFor="printer-preset">{t('printers.choosePreset')}</label>
    <input type="search" aria-label={t('printers.searchPresets')} placeholder={t('printers.searchPresets')} value={query} onChange={event => setQuery(event.target.value)} className="w-full rounded-md border ui-border-default px-3 py-2" />
    <select id="printer-preset" value={preset?.id ?? ''} onChange={(event) => onSelect(PRINTER_PRESETS.find(item => item.id === event.target.value) ?? null)} className="w-full rounded-md border ui-border-default px-3 py-2">
      <option value="">{t('printers.customPreset')}</option>
      {['Bambu Lab', 'Prusa', 'Creality', 'Anycubic', 'ELEGOO'].map(manufacturer => <optgroup key={manufacturer} label={manufacturer}>
        {visiblePresets.filter(item => item.manufacturer === manufacturer).map(item => <option key={item.id} value={item.id}>{item.displayName}</option>)}
      </optgroup>)}
    </select>
    {preset && <div className="rounded-md ui-surface-input p-3 text-sm space-y-2">
      <p>{t('printers.examplePrice', { price: preset.profileDefaults.printer_purchase_cost, currency: preset.currency, region: preset.region, date: preset.reviewedAt })}</p>
      <p>{t('printers.powerEvidence', { average: preset.profileDefaults.printer_power_watts, rated: preset.evidence.ratedPowerWatts })}</p>
      <p>{preset.evidence.averagePowerEvidence[i18n.resolvedLanguage] ?? preset.evidence.averagePowerEvidence.en}</p>
      <p>{t('printers.confidence', { level: preset.evidence.confidence })}</p>
      <a className="underline" href={preset.evidence.sourceUrl} target="_blank" rel="noreferrer">{t('printers.presetSource')}</a>
      {allowComplete && preset.id === 'bambu-lab-a1-v1' && <label className="flex items-center gap-2"><input type="checkbox" id="complete-reference" />{t('printers.completeReference')}</label>}
      <button type="button" onClick={() => onApply(preset, allowComplete && preset.id === 'bambu-lab-a1-v1' && document.getElementById('complete-reference')?.checked)} className="page-action">{t('printers.applyPreset')}</button>
    </div>}
  </section>;
}

function PrinterModal({ printer, onClose, onSaved }) {
  const { t } = useTranslation();
  const [selectedPreset, setSelectedPreset] = useState(null);
  const [appliedConfig, setAppliedConfig] = useState(null);
  const [presetErrors, setPresetErrors] = useState({});
  const { register, handleSubmit, setValue, formState: { errors, isSubmitting } } = useForm({
    resolver: yupResolver(metadataSchema(t)),
    defaultValues: printer ?? { name: '', manufacturer: '', model: '', notes: '' },
  });
  const save = async (data) => {
    try {
      if (appliedConfig) {
        const invalid = Object.fromEntries(Object.entries(appliedConfig).flatMap(([key, value]) => {
          const message = value === '' || !Number.isFinite(Number(value)) ? t('costSettings.validation.number') : COST_SETTINGS[key] ? valueError(key, value) : Number(value) < 0 || Number(value) > 1e9 ? t('costSettings.validation.maximum') : null;
          return message ? [[key, message]] : [];
        }));
        setPresetErrors(invalid);
        if (Object.keys(invalid).length) return;
      }
      const payload = Object.fromEntries(Object.entries(data).map(([key, value]) => [key, value.trim()]));
      if (printer) await updatePrinter(printer.id, payload);
      else await createPrinter({ ...payload, ...(appliedConfig ? { cost_config: Object.fromEntries(Object.entries(appliedConfig).map(([key, value]) => [key, Number(value)])) } : {}) });
      toast.success(t(printer ? 'printers.updated' : 'printers.created'));
      onSaved();
    } catch (error) {
      toast.error(error?.message || t('printers.saveError'));
    }
  };
  return <Modal title={t(printer ? 'printers.editTitle' : 'printers.add')} onClose={onClose}>
    <form onSubmit={handleSubmit(save)} noValidate className="space-y-4">
      {!printer && <PresetPicker preset={selectedPreset} onSelect={setSelectedPreset} allowComplete onApply={(preset, complete) => {
        setValue('manufacturer', preset.manufacturer, { shouldDirty: true }); setValue('model', preset.model, { shouldDirty: true });
        setAppliedConfig({ ...preset.profileDefaults, ...(complete ? A1_REFERENCE_PROFILE : {}) });
        setPresetErrors({});
      }} />}
      {appliedConfig && <section className="rounded-xl border ui-border-subtle p-4" aria-label={t('printers.editPresetValues')}>
        <h3 className="font-semibold">{t('printers.editPresetValues')}</h3><p className="mb-3 text-sm ui-text-secondary">{t('printers.hardwareOnlyNote')}</p>
        {Object.entries(appliedConfig).map(([key, value]) => <label key={key} className="mb-2 grid grid-cols-[1fr_8rem] items-center gap-3 text-sm">{COST_SETTINGS[key]?.[0] ?? key}
          <input type="number" step="any" value={value} aria-invalid={!!presetErrors[key]} onChange={event => { setAppliedConfig(current => ({ ...current, [key]: event.target.value })); setPresetErrors(current => ({ ...current, [key]: undefined })); }} className="rounded-md border ui-border-default px-2 py-1" />
          {presetErrors[key] && <span role="alert" className="col-span-2 text-xs ui-text-danger">{presetErrors[key]}</span>}
        </label>)}
      </section>}
      {[
        ['name', t('printers.name'), 'e.g. A1 – Workshop'],
        ['manufacturer', t('printers.manufacturer'), 'e.g. Bambu Lab'],
        ['model', t('printers.model'), 'e.g. A1'],
      ].map(([key, label, placeholder]) => <div key={key}>
        <label htmlFor={`printer-${key}`} className="mb-1 block text-sm font-medium ui-text-secondary">{label}</label>
        <input id={`printer-${key}`} {...register(key)} maxLength={100} placeholder={placeholder} aria-invalid={!!errors[key]}
          className="w-full rounded-md border ui-border-default px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ui-focus-ring" />
        {errors[key] && <p role="alert" className="mt-1 text-xs ui-text-danger">{errors[key].message}</p>}
      </div>)}
      <div>
        <label htmlFor="printer-notes" className="mb-1 block text-sm font-medium ui-text-secondary">{t('printers.notes')}</label>
        <textarea id="printer-notes" {...register('notes')} maxLength={2000} rows={3}
          className="w-full rounded-md border ui-border-default px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ui-focus-ring" />
        {errors.notes && <p role="alert" className="mt-1 text-xs ui-text-danger">{errors.notes.message}</p>}
      </div>
      <div className="flex justify-end gap-2 pt-2">
        <button type="button" onClick={onClose} className="page-action page-action-secondary">{t('common.cancel')}</button>
        <button type="submit" disabled={isSubmitting} className="page-action disabled:opacity-50">
          {isSubmitting && <Loader2 className="h-4 w-4 animate-spin" />}{t(printer ? 'common.saveChanges' : 'printers.create')}
        </button>
      </div>
    </form>
  </Modal>;
}

function CostEditor({ printer, onClose, onSaved }) {
  const { t } = useTranslation();
  const defaults = Object.fromEntries(costKeys.map((key) => [key, String(printer.cost_config?.[key] ?? '')]));
  const [selectedPreset, setSelectedPreset] = useState(null);
  const [pendingConfig, setPendingConfig] = useState(null);
  const [extraCostOverrides, setExtraCostOverrides] = useState({});
  const { register, handleSubmit, watch, setValue, formState: { errors, isSubmitting, isDirty } } = useForm({
    resolver: yupResolver(costSchema(t)), defaultValues: defaults,
  });
  const values = watch();
  const save = async (data) => {
    try {
      await updatePrinterCostConfig(printer.id, { ...printer.cost_config, ...Object.fromEntries(Object.entries(data).map(([key, value]) => [key, Number(value)])), ...extraCostOverrides });
      toast.success(t('printers.costsSaved'));
      onSaved();
    } catch (error) {
      toast.error(error?.message || t('printers.costsError'));
    }
  };
  return <section className="page-panel mt-6" aria-labelledby="printer-cost-title">
    <div className="page-panel-header flex flex-wrap items-start justify-between gap-3">
      <div><h2 id="printer-cost-title" className="text-lg font-semibold">{t('printers.costsFor', { name: printer.name })}</h2>
        <p className="mt-1 text-sm ui-text-secondary">{t('printers.costsHelp')}</p></div>
      <button type="button" onClick={onClose} className="page-action page-action-secondary">{t('common.close')}</button>
    </div>
    <form onSubmit={handleSubmit(save)} noValidate className="p-4 sm:p-6">
      <PresetPicker preset={selectedPreset} onSelect={setSelectedPreset} allowComplete onApply={(preset, complete) => {
        const next = { ...(complete ? A1_REFERENCE_PROFILE : preset.profileDefaults) };
        const diff = Object.fromEntries(Object.entries(next).filter(([key, value]) => Number(defaults[key]) !== Number(value)));
        setExtraCostOverrides(complete ? { margin_tier_1: 80, margin_tier_2: 90, margin_tier_3: 99 } : {});
        setPendingConfig({ next, diff });
      }} />
      {pendingConfig && <div className="my-4 rounded-md border ui-border-primary p-3" role="dialog" aria-label={t('printers.presetDiff')}>
        <p className="font-semibold">{t('printers.presetDiff')}</p>
        <ul className="mt-2 max-h-40 overflow-auto text-sm">{Object.entries(pendingConfig.diff).map(([key, value]) => <li key={key}>{COST_SETTINGS[key]?.[0] ?? key}: {defaults[key] ?? '—'} → {value}</li>)}</ul>
        <button type="button" className="page-action mt-3" onClick={() => { Object.entries(pendingConfig.next).forEach(([key, value]) => setValue(key, String(value), { shouldDirty: true })); setPendingConfig(null); }}>{t('printers.confirmApplyPreset')}</button>
      </div>}
      <fieldset disabled={isSubmitting}>
        <CostConfigFields values={values} availableKeys={costKeys} register={register} errors={errors} dirty={isDirty} idPrefix="printer-cost" />
      </fieldset>
      <div className="mt-6 flex justify-end">
        <button type="submit" disabled={isSubmitting} className="page-action disabled:opacity-50">
          {isSubmitting ? <Loader2 className="h-4 w-4 animate-spin" /> : <Check className="h-4 w-4" />}{t('printers.saveCosts')}
        </button>
      </div>
    </form>
  </section>;
}

export default function Printers() {
  const { t } = useTranslation();
  const fetchPrinters = useCallback(() => getPrinters('all'), []);
  const { data: printers, loading, error, refetch } = useApi(fetchPrinters);
  const [editing, setEditing] = useState(null);
  const [modalOpen, setModalOpen] = useState(false);
  const [costPrinter, setCostPrinter] = useState(null);
  const mutate = async (action, success) => {
    try { await action(); toast.success(success); await refetch(); }
    catch (err) { toast.error(err?.message || t('printers.updateError')); }
  };
  const saved = async () => { setModalOpen(false); setEditing(null); await refetch(); };
  return <div>
    {modalOpen && <PrinterModal printer={editing} onClose={() => { setModalOpen(false); setEditing(null); }} onSaved={saved} />}
    <PageHeader title={t('printers.title')} description={t('printers.description')}>
      <button onClick={() => { setEditing(null); setModalOpen(true); }} className="page-action"><Plus className="h-4 w-4" />{t('printers.add')}</button>
    </PageHeader>
    {error && <InlineError title={t('printers.loadError')} message={error} onRetry={refetch} />}
    {loading && !printers ? <div className="page-panel"><TableSkeleton columns={4} label={t('printers.loading')} /></div> : printers?.length === 0 ?
      <div className="page-panel"><EmptyState icon={Printer} title={t('printers.empty')} description={t('printers.emptyHelp')} action={<button onClick={() => setModalOpen(true)} className="page-action">{t('printers.addFirst')}</button>} /></div> :
      <div className="grid gap-4 lg:grid-cols-2">
        {(printers ?? []).map((printer) => <article key={printer.id} className={`page-panel p-5 ${printer.is_archived ? 'opacity-70' : ''}`}>
          <div className="flex items-start justify-between gap-3">
            <div><div className="flex flex-wrap items-center gap-2"><h2 className="text-lg font-semibold ui-text-primary">{printer.name}</h2>
              {printer.is_default && <span className="rounded-full ui-tone-primary px-2 py-0.5 text-xs font-medium ui-text-primary-action">{t('printers.default')}</span>}
              {printer.is_archived && <span className="rounded-full ui-surface-input px-2 py-0.5 text-xs ui-text-muted">{t('printers.archived')}</span>}</div>
              <p className="mt-1 text-sm ui-text-secondary">{printer.manufacturer} · {printer.model}</p>
              {printer.notes && <p className="mt-3 text-sm ui-text-muted whitespace-pre-wrap">{printer.notes}</p>}</div>
            <Printer className="h-5 w-5 ui-text-primary-action" aria-hidden="true" />
          </div>
          <div className="mt-5 flex flex-wrap gap-2">
            {!printer.is_archived && <button onClick={() => setCostPrinter(printer)} className="page-action page-action-secondary">{t('printers.editCosts')}</button>}
            <button onClick={() => { setEditing(printer); setModalOpen(true); }} className="page-action page-action-secondary"><Pencil className="h-4 w-4" />{t('common.edit')}</button>
            {!printer.is_archived && !printer.is_default && <button onClick={() => mutate(() => setDefaultPrinter(printer.id), t('printers.defaultChanged'))} className="page-action page-action-secondary"><Star className="h-4 w-4" />{t('printers.setDefault')}</button>}
            {printer.is_archived ? <button onClick={() => mutate(() => restorePrinter(printer.id), t('printers.restored'))} className="page-action page-action-secondary"><RotateCcw className="h-4 w-4" />{t('printers.restore')}</button> :
              <button onClick={() => mutate(() => archivePrinter(printer.id), t('printers.archivedMessage'))} className="page-action page-action-secondary"><Archive className="h-4 w-4" />{t('printers.archive')}</button>}
          </div>
        </article>)}
      </div>}
    {costPrinter && <CostEditor key={costPrinter.id + costPrinter.updated_at} printer={(printers ?? []).find((p) => p.id === costPrinter.id) ?? costPrinter} onClose={() => setCostPrinter(null)} onSaved={async () => { await refetch(); setCostPrinter(null); }} />}
  </div>;
}
