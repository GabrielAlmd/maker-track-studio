import FormSelect from '../components/ui/FormSelect';
import SearchableSelect from '../components/ui/SearchableSelect';
import { StockWarningList } from '../components/StockWarnings';
import MaterialEstimateDetails from '../components/MaterialEstimateDetails';
import { useEstimateStock } from '../hooks/useEstimateStock';
import { combineMappedEstimates, estimateRunMaterials } from '../utils/productionEstimates';
import ProductionModal from '../components/ProductionModal';
import PageHeader from '../components/ui/PageHeader';
import { useCallback, useEffect, useRef, useState } from 'react';
import { useForm } from 'react-hook-form';
import { yupResolver } from '@hookform/resolvers/yup';
import * as yup from 'yup';
import toast from 'react-hot-toast';
import { Loader2, Plus, Printer, Trash2 } from 'lucide-react';
import { format } from 'date-fns';
import Modal from '../components/ui/Modal';
import PrintJobCard from '../components/PrintJobCard';
import ModelModal from '../components/ModelModal';
import { EmptyState, InlineError, TableSkeleton } from '../components/ui/PageState';
import { filamentName, modelName } from '../components/printJobDisplay';
import { useApi } from '../hooks/useApi';
import { usePaginatedList } from '../hooks/usePaginatedList';
import ListToolbar from '../components/ui/ListToolbar';
import BulkActions, { RowSelectionCheckbox, SelectAllCheckbox } from '../components/ui/BulkActions';
import { useBulkSelection } from '../hooks/useBulkSelection';
import {
  getPrintJobs,
  createPrintJob,
  updatePrintJob,
  deletePrintJob,
} from '../services/printJobService';
import { getModels } from '../services/modelService';
import { getFilaments } from '../services/filamentService';
import { getPrinters } from '../services/printerService';
import { useTranslation } from 'react-i18next';
import FilamentSelect from '../components/FilamentSelect';
import ModelSelect from '../components/ModelSelect';

const printJobSchema = (t) => yup.object({
  printer_id: yup.string().required(t('validation.printerRequired')),
  model_id: yup.string().nullable().transform((v) => v || null),
  print_date: yup.string().required(t('validation.printDateRequired')),
  units_printed: yup
    .number()
    .typeError(t('validation.unitsNumber'))
    .integer(t('validation.wholeNumber'))
    .positive(t('validation.atLeastOne'))
    .required(t('validation.unitsRequired')),
  labor_hours: yup
    .number()
    .typeError(t('validation.laborNumber'))
    .test('finite', t('validation.laborNumber'), (value) => value == null || Number.isFinite(value))
    .min(0, t('validation.nonNegative'))
    .required(t('validation.laborRequired')),
  print_time_minutes: yup
    .number()
    .typeError(t('validation.printTimeNumber'))
    .integer(t('validation.wholeNumber'))
    .positive(t('validation.positive'))
    .nullable()
    .transform((v, orig) => (orig === '' ? null : v)),
});

function PrintJobModal({ printJob, models, filaments, printers, onClose, onSaved, onModelCreated }) {
  const { t } = useTranslation();
  const requiredLabel = (label) => <>{label} <span aria-hidden="true" className="text-xs capitalize ui-text-muted">({t('required')})</span></>;
  const isEdit = !!printJob;
  const automaticTime = useRef(false);
  const materialOverrides = useRef(Boolean(printJob?.filaments?.length));
  const [estimateConflict, setEstimateConflict] = useState(false);
  const inventory = useEstimateStock();
  const [submitError, setSubmitError] = useState(null);
  const [creatingModel, setCreatingModel] = useState(false);
  const [availableModels, setAvailableModels] = useState(models ?? []);

  useEffect(() => {
    setAvailableModels((current) => {
      const merged = new Map(current.map((model) => [model.id, model]));
      (models ?? []).forEach((model) => merged.set(model.id, model));
      return Array.from(merged.values());
    });
  }, [models]);

  // Automatic weights track quantity changes until manually adjusted.
  const [filamentRows, setFilamentRows] = useState(
    printJob?.filaments?.length
      ? printJob.filaments.map((pjf) => ({
        id: pjf.id,
        filament_id: pjf.filament_id,
        weight_used: pjf.weight_used,
        estimated_weight_g: pjf.estimated_weight_g ?? '',
        waste_weight_g: pjf.waste_weight_g ?? '',
        measurement_state: pjf.measurement_state ?? 'legacy_unverified',
      }))
      : []
  );

  const addFilamentRow = () => {
    materialOverrides.current = true;
    setFilamentRows(prev => [...prev, { filament_id: '', weight_used: '', estimated_weight_g: '', waste_weight_g: '', measurement_state: 'verified' }]);
  };

  const removeFilamentRow = (index) => {
    materialOverrides.current = true;
    setFilamentRows(prev => prev.filter((_, i) => i !== index));
  };

  const updateFilamentRow = (index, field, value) => {
    const row = filamentRows[index];
    const legacyEstimate = row._sources?.length === 1 && !row._sources[0].type;
    materialOverrides.current = true;
    setFilamentRows(prev => prev.map((current, i) => {
      if (i !== index) return current;
      if (field === 'weight_used' && legacyEstimate && current.measurement_state === 'estimated') {
        return { ...current, weight_used: value, measurement_state: 'verified', waste_weight_g: '' };
      }
      if (field === 'measurement_state' && value === 'verified') {
        const estimate = Number(current.weight_used);
        return { ...current, measurement_state: value, estimated_weight_g: estimate > 0 ? estimate : current.estimated_weight_g,
          weight_used: '', waste_weight_g: '' };
      }
      if (field === 'weight_used' && !current._sources) return { ...current, weight_used: value, measurement_state: 'verified' };
      return { ...current, [field]: value };
    }));
  };

  const {
    control, register,
    handleSubmit,
    getValues,
    setValue,
    watch,
    formState: { errors, isSubmitting },
  } = useForm({
    resolver: yupResolver(printJobSchema(t)),
    defaultValues: printJob
      ? {
        printer_id: printJob.printer_id ?? '',
        model_id: printJob.model_id ?? '',
        print_date: format(new Date(printJob.print_date), 'yyyy-MM-dd'),
        units_printed: printJob.units_printed,
        labor_hours: printJob.labor_hours,
        print_time_minutes: printJob.print_time_minutes ?? '',
      }
      : { printer_id: printers.find((printer) => printer.is_default)?.id ?? '', units_printed: 1, labor_hours: 0, print_time_minutes: '' },
  });

  const applyModelDefaults = (modelId, unitsValue, modelChanged = false, selectedModel, replace = false) => {
    const model = selectedModel ?? availableModels.find((item) => item.id === modelId);
    const units = Number(unitsValue);
    if (!Number.isSafeInteger(units) || units < 1) return;

    if (modelChanged || automaticTime.current) {
      const time = Number(model?.print_time) * units;
      setValue('print_time_minutes', Number.isSafeInteger(time) && time > 0 ? time : '', {
        shouldDirty: true,
        shouldValidate: true,
      });
      automaticTime.current = Boolean(model);
    }

    if (materialOverrides.current && !replace) {
      setEstimateConflict(true);
      return;
    }

    const suggestions = estimateRunMaterials([{ model_id: modelId, planned: units }], model ? [model] : [], filaments, inventory.stock, 2);
    setFilamentRows(suggestions);
    if (replace) materialOverrides.current = false;
    setEstimateConflict(false);
  };

  const handleModelCreated = (createdModel) => {
    setAvailableModels((current) => [...current.filter((model) => model.id !== createdModel.id), createdModel]);
    setValue('model_id', createdModel.id, { shouldDirty: true, shouldValidate: true });
    applyModelDefaults(createdModel.id, getValues('units_printed'), true, createdModel);
    setCreatingModel(false);
    onModelCreated();
  };

  const onSubmit = async (data) => {
    setSubmitError(null);
    try {
      await yup.array().of(yup.object({
        filament_id: yup.string().required(t('validation.selectFilamentRows')),
        weight_used: yup.number().typeError(t('validation.filamentWeightNumber'))
          .positive(t('validation.filamentWeightPositive')).required(t('validation.filamentWeightRequired'))
          .test('finite', t('validation.filamentWeightFinite'), Number.isFinite),
        waste_weight_g: yup.number().transform((value, original) => original === '' ? null : value)
          .nullable().min(0, t('validation.nonNegative'))
          .test('within-total', t('validation.wasteWithinTotal'), function (value) { return value == null || value <= Number(this.parent.weight_used); }),
      })).max(100).validate(filamentRows);
      const materialRows = combineMappedEstimates(filamentRows, 2);
      await yup.array().of(yup.object({ weight_used: yup.number().max(1e9) })).validate(materialRows);
      const ids = materialRows.map((row) => row.filament_id);
      if (new Set(ids).size !== ids.length) throw new Error(t('validation.duplicateFilament'));
      const payload = {
        printer_id: data.printer_id,
        model_id: data.model_id || null,
        print_date: new Date(data.print_date).toISOString(),
        units_printed: Number(data.units_printed),
        labor_hours: Number(data.labor_hours),
        print_time_minutes: data.print_time_minutes ? Number(data.print_time_minutes) : null,
        filaments: materialRows.map((row) => ({
          ...(row.id ? { id: row.id } : {}),
          filament_id: row.filament_id,
          weight_used: Number(row.weight_used),
          measurement_state: row.measurement_state,
          ...(row.estimated_weight_g !== '' ? { estimated_weight_g: Number(row.measurement_state === 'estimated' ? row.weight_used : row.estimated_weight_g) } : {}),
          ...(row.waste_weight_g !== '' && row.waste_weight_g != null ? { waste_weight_g: Number(row.waste_weight_g) } : {}),
        })),
      };
      if (isEdit) {
        await updatePrintJob(printJob.id, payload);
        toast.success(t('printJobs.updated'));
      } else {
        await createPrintJob(payload);
        toast.success(t('printJobs.created'));
      }
      onSaved();
    } catch (error) {
      setSubmitError(error instanceof yup.ValidationError ? error.message : error?.message || t('printJobs.saveError'));
    }
  };

  if (creatingModel) {
    return <ModelModal onClose={() => setCreatingModel(false)} onSaved={handleModelCreated} />;
  }

  return (
    <Modal title={t(isEdit ? 'printJobs.editTitle' : 'printJobs.addTitle')} onClose={onClose} size="wide">
      <form onSubmit={handleSubmit(onSubmit)} className="space-y-4">
        {submitError && <p className="page-form-error" role="alert">{submitError}</p>}
        <div>
          <label htmlFor="print-job-printer" className="block text-sm font-medium ui-text-secondary mb-1">{requiredLabel(t('printJobs.printer'))}</label>
          <FormSelect id="print-job-printer" name="printer_id" control={control} required
            options={printJob?.printer_id && !printers.some(p => p.id === printJob.printer_id)
              ? [{ id: printJob.printer_id, name: printJob.printer_cost?.printer_name ?? t('printJobs.archivedPrinter'), is_archived: true }, ...printers]
              : printers}
            getLabel={p => p.name + (p.is_default ? ` (${t('printers.default').toLowerCase()})` : p.is_archived ? ` (${t('printers.archived').toLowerCase()})` : '')}
            clearLabel={t('printJobs.selectPrinter')} />
          {errors.printer_id && <p role="alert" className="ui-text-danger text-xs mt-1">{errors.printer_id.message}</p>}
          <p className="mt-1 text-xs ui-text-muted">{t('printJobs.printerCostHelp')}</p>
        </div>
        <div>
          <div className="mb-1 flex items-center justify-between gap-3">
            <label className="block text-sm font-medium ui-text-secondary">{t('printJobs.modelOptional')}</label>
            <button type="button" onClick={() => setCreatingModel(true)} className="flex items-center gap-1 text-xs ui-text-primary-action hover:underline">
              <Plus className="h-3 w-3" /> {t('printJobs.createModel')}
            </button>
          </div>
          <input type="hidden" {...register('model_id')} />
          <ModelSelect
            aria-label={t('dashboard.model')}
            value={watch('model_id') ?? ''}
            models={availableModels}
            onChange={(modelId) => {
              setValue('model_id', modelId, { shouldDirty: true, shouldValidate: true });
              applyModelDefaults(modelId, getValues('units_printed'), true);
            }}
          />
        </div>
        <div>
          <label htmlFor="print-job-date" className="block text-sm font-medium ui-text-secondary mb-1">{requiredLabel(t('printJobs.printDate'))}</label>
          <input
            id="print-job-date"
            type="date"
            aria-required="true"
            {...register('print_date')}
            className="w-full border ui-border-default rounded-md px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ui-focus-ring"
          />
          {errors.print_date && (
            <p className="ui-text-danger text-xs mt-1">{errors.print_date.message}</p>
          )}
        </div>
        <div className="grid grid-cols-2 gap-4">
          <div>
            <label htmlFor="print-job-units" className="block text-sm font-medium ui-text-secondary mb-1">{requiredLabel(t('dashboard.unitsPrinted'))}</label>
            <input
              id="print-job-units"
              type="number"
              step="1"
              min="1"
              aria-required="true"
              {...register('units_printed', {
                onChange: (event) => applyModelDefaults(getValues('model_id'), event.target.value),
              })}
              className="w-full border ui-border-default rounded-md px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ui-focus-ring"
            />
            {errors.units_printed && (
              <p className="ui-text-danger text-xs mt-1">{errors.units_printed.message}</p>
            )}
          </div>
          <div>
            <label htmlFor="print-job-labor" className="block text-sm font-medium ui-text-secondary mb-1">{requiredLabel(t('printJobs.laborHours'))}</label>
            <input
              id="print-job-labor"
              type="number"
              step="any"
              min="0"
              aria-required="true"
              {...register('labor_hours')}
              className="w-full border ui-border-default rounded-md px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ui-focus-ring"
            />
            {errors.labor_hours && (
              <p className="ui-text-danger text-xs mt-1">{errors.labor_hours.message}</p>
            )}
          </div>
        </div>
        <div>
          <label className="block text-sm font-medium ui-text-secondary mb-1">
            {t('printJobs.printTimeOptional')}
          </label>
          <input
            type="number"
            step="1"
            min="1"
            {...register('print_time_minutes', { onChange: () => { automaticTime.current = false; } })}
            className="w-full border ui-border-default rounded-md px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ui-focus-ring"
            placeholder="e.g. 90"
          />
          {errors.print_time_minutes && (
            <p className="ui-text-danger text-xs mt-1">{errors.print_time_minutes.message}</p>
          )}
        </div>
        <p className="text-xs ui-text-muted">
          {t('printJobs.estimatesHelp')}
        </p>
        {/* Filaments used */}
        <div>
          <div className="flex justify-between items-center mb-2">
            <label className="block text-sm font-medium ui-text-secondary">{t('printJobs.filamentsUsed')}</label>
            <button
              type="button"
              onClick={addFilamentRow}
              className="text-xs ui-text-primary-action hover:ui-text-primary-action flex items-center gap-1"
            >
              <Plus className="h-3 w-3" /> {t('printJobs.addFilament')}
            </button>
          </div>
          {filamentRows.length === 0 && (
            <p className="text-xs ui-text-muted italic">{t('printJobs.noFilamentsYet')}</p>
          )}
          {estimateConflict && <div className="mb-2 text-xs ui-text-muted" role="status">
            <p>{t('materialEstimates.kept')}</p>
            {!isEdit && watch('model_id') && <button type="button" className="page-action page-action-secondary"
              onClick={() => applyModelDefaults(getValues('model_id'), getValues('units_printed'), false, undefined, true)}>
              {t('materialEstimates.replace')}
            </button>}
          </div>}
          <div className="space-y-2">
            {filamentRows.map((row, index) => (
              <div key={row.id ?? index} className="flex flex-wrap gap-2 items-center">
                <FilamentSelect
                  aria-label={`${t('filaments.title').slice(0, -1)} ${index + 1}`}
                  value={row.filament_id}
                  onChange={(filamentId) => updateFilamentRow(index, 'filament_id', filamentId)}
                  filaments={filaments}
                  className="flex-1"
                />
                <input
                  type="number"
                  step="0.01"
                  min="0"
                  placeholder="g"
                  value={row.weight_used}
                  onChange={(e) => updateFilamentRow(index, 'weight_used', e.target.value)}
                  className="w-20 border ui-border-default rounded-md px-2 py-1.5 text-sm focus:outline-none focus:ring-2 focus:ui-focus-ring"
                />
                <input
                  type="number"
                  step="0.01"
                  min="0"
                  placeholder={t('printJobs.wasteGrams')}
                  aria-label={`${t('printJobs.wasteGrams')} ${index + 1}`}
                  value={row.waste_weight_g ?? ''}
                  onChange={(e) => updateFilamentRow(index, 'waste_weight_g', e.target.value)}
                  className="w-24 border ui-border-default rounded-md px-2 py-1.5 text-sm focus:outline-none focus:ring-2 focus:ui-focus-ring"
                />
                {row._sources?.some(source => source.type) ? <select
                  aria-label={`${t('production.measurement')} ${index + 1}`}
                  value={row.measurement_state} onChange={event => updateFilamentRow(index, 'measurement_state', event.target.value)}>
                  <option value="estimated">{t('production.estimate')}</option>
                  <option value="verified">{t('production.actual')}</option>
                </select> : <span className="text-xs ui-text-muted">{t(`printJobs.materialState.${row.measurement_state ?? 'legacy_unverified'}`)}</span>}
                {row.measurement_state === 'verified' && row.estimated_weight_g !== '' && row.estimated_weight_g != null && (
                  <span className="text-xs ui-text-muted">{t('printJobs.estimateLabel')}: {Number(row.estimated_weight_g).toFixed(2)} g</span>
                )}
                <button
                  type="button"
                  onClick={() => removeFilamentRow(index)}
                  className="ui-text-danger hover:ui-text-danger"
                  aria-label={t('printJobs.removeFilament')}
                >
                  <Trash2 className="h-4 w-4" />
                </button>
                <MaterialEstimateDetails row={row} rows={filamentRows} filaments={filaments} stock={inventory.stock} />
              </div>
            ))}
          </div>
          {filamentRows.length > 0 && <p className="mt-2 text-xs ui-text-secondary">
            {t('printJobs.totalMaterial')}: {filamentRows.reduce((total, row) => total + (Number(row.weight_used) || 0), 0).toFixed(2)} g · {t('printJobs.verifiedMaterial')}: {filamentRows.filter((row) => row.measurement_state === 'verified').reduce((total, row) => total + (Number(row.weight_used) || 0), 0).toFixed(2)} g
          </p>}
          <StockWarningList requirements={filamentRows} inventory={inventory} />
          <p className="mt-1 text-xs ui-text-muted">{t('printJobs.materialHelp')}</p>
        </div>

        <div className="flex justify-end gap-3 pt-2">
          <button
            type="button"
            onClick={onClose}
            className="px-4 py-2 text-sm ui-text-secondary border ui-border-default rounded-md hover:ui-surface-hover"
          >
            {t('common.cancel')}
          </button>
          <button
            type="submit"
            disabled={isSubmitting}
            className="px-4 py-2 text-sm ui-text-inverse ui-bg-primary rounded-md hover:ui-bg-primary-hover disabled:opacity-50 flex items-center gap-2"
            aria-busy={isSubmitting}
          >
            {isSubmitting && <Loader2 className="h-4 w-4 animate-spin" />}
            {isSubmitting ? t('common.saving') : isEdit ? t('common.saveChanges') : t('printJobs.create')}
          </button>
        </div>
      </form>
    </Modal>
  );
}

function PrintJobs() {
  const { t } = useTranslation();
  const [productionOpen, setProductionOpen] = useState(false);
  const [modalOpen, setModalOpen] = useState(false);
  const [editingJob, setEditingJob] = useState(null);
  const [deletingId, setDeletingId] = useState(null);

  const fetchJobs = useCallback((params) => getPrintJobs(params), []);
  const fetchModels = useCallback(() => getModels(), []);
  const fetchFilaments = useCallback(() => getFilaments(), []);
  const fetchPrinters = useCallback(() => getPrinters('all'), []);

  const list = usePaginatedList(fetchJobs, 'date', 'desc');
  const { data: jobPage, loading, error, refetch } = list;
  const printJobs = jobPage?.items;
  const { data: models, refetch: refetchModels } = useApi(fetchModels);
  const { data: filaments } = useApi(fetchFilaments);
  const { data: printers, loading: printersLoading, error: printersError, refetch: refetchPrinters } = useApi(fetchPrinters);
  const activePrinters = (printers ?? []).filter((printer) => !printer.is_archived);
  const visibleJobs = jobPage?.legacy
    ? (printJobs ?? []).filter((job) => [modelName(job), ...(job.filaments ?? []).map(filamentName)].some((value) => value.toLowerCase().includes(list.searchInput.trim().toLowerCase()))).sort((a, b) => list.direction === 'asc' ? new Date(a.print_date) - new Date(b.print_date) : new Date(b.print_date) - new Date(a.print_date))
    : (printJobs ?? []);
  const bulk = useBulkSelection({
    items: visibleJobs,
    deleteItem: deletePrintJob,
    getLabel: modelName,
    onDeleted: refetch,
    failureFallback: t('bulk.failureFallback'),
  });

  const handleEdit = (job) => {
    setEditingJob(job);
    if (job.workflow_version === 2) { setProductionOpen(true); return; }
    setModalOpen(true);
  };

  const handleAdd = () => {
    if (!activePrinters.length) {
      toast.error(t('printJobs.needPrinter'));
      return;
    }
    setEditingJob(null);
    setModalOpen(true);
  };

  const handleModalClose = () => {
    setModalOpen(false);
    setProductionOpen(false);
    setEditingJob(null);
  };

  const handleSaved = () => {
    handleModalClose();
    refetch();
  };

  const handleDelete = async (id) => {
    if (!window.confirm(t('printJobs.confirmDelete'))) return;
    setDeletingId(id);
    try {
      await deletePrintJob(id);
      toast.success(t('printJobs.deleted'));
      refetch();
    } catch {
      // toasted by interceptor
    } finally {
      setDeletingId(null);
    }
  };

  return (
    <div>
      {productionOpen && <ProductionModal job={editingJob} models={models ?? []} filaments={filaments ?? []} printers={editingJob ? (printers ?? []) : activePrinters} onClose={handleModalClose} onSaved={handleSaved} />}
      {modalOpen && (
        <PrintJobModal
          printJob={editingJob}
          models={models}
          filaments={filaments}
          printers={activePrinters}
          onClose={handleModalClose}
          onSaved={handleSaved}
          onModelCreated={refetchModels}
        />
      )}

      <PageHeader title={t('printJobs.title')} description={t('printJobs.description')}>
        <button className="page-action page-action-secondary" disabled={!activePrinters.length} onClick={() => { setEditingJob(null); setProductionOpen(true); }}>{t('production.plan')}</button>
        <button
          onClick={handleAdd}
          disabled={printersLoading || !activePrinters.length}
          className="page-action disabled:cursor-not-allowed disabled:opacity-50"
        >
          <Plus className="h-4 w-4" />
          {t('printJobs.add')}
        </button>
      </PageHeader>

      {printersError && <div className="mb-5"><InlineError title={t('printers.loadError')} message={printersError} onRetry={refetchPrinters} /></div>}
      {!printersLoading && !printersError && activePrinters.length === 0 && <div className="mb-5 rounded-xl border ui-border-primary ui-tone-primary p-4 text-sm ui-text-secondary">
        {t('printJobs.addActivePrefix')} <a href="#/printers" className="font-semibold ui-text-primary-action underline">{t('printJobs.printersPage')}</a> {t('printJobs.addActiveSuffix')}
      </div>}

      <div className="list-toolbar-panel mb-5"><ListToolbar list={list} sortLabel={t('printJobs.sort')} nativeSort={!modalOpen} sortValue={list.direction === 'asc' ? 'oldest' : 'newest'} onSortChange={(value) => { list.setSort('date'); list.setDirection(value === 'newest' ? 'desc' : 'asc'); list.setPage(1); }} sorts={[{ value: 'newest', label: t('printJobs.newest') }, { value: 'oldest', label: t('printJobs.oldest') }]}>
        <div className="space-y-3 text-sm">
          <label className="block">{t('purchases.from')}<input type="date" value={list.filters.date_from || ''} onChange={(event) => list.changeFilters({ ...list.filters, date_from: event.target.value })} className="mt-1 w-full rounded border p-2" /></label>
          <label className="block">{t('purchases.to')}<input type="date" value={list.filters.date_to || ''} onChange={(event) => list.changeFilters({ ...list.filters, date_to: event.target.value })} className="mt-1 w-full rounded border p-2" /></label>
          <div className="block"><SearchableSelect label={t('printJobs.printer')} options={printers ?? []} getLabel={p => p.name + (p.is_archived ? ` (${t('printers.archived').toLowerCase()})` : '')}
            value={list.filters.printer_id || ''} clearLabel={t('printJobs.allPrinters')}
            onChange={id => list.changeFilters({ ...list.filters, printer_id: id })} /></div>
          <div><span className="block mb-1">{t('dashboard.model')}</span><ModelSelect aria-label={t('dashboard.model')} value={list.filters.model_id || ''} onChange={(modelId) => list.changeFilters({ ...list.filters, model_id: modelId })} models={models ?? []} emptyLabel={t('printJobs.allModels')} /></div>
          <div className="block"><FilamentSelect includeArchived label={t('dashboard.filament')} filaments={filaments}
            value={list.filters.filament_id || ''} clearLabel={t('printJobs.allFilaments')}
            onChange={id => list.changeFilters({ ...list.filters, filament_id: id })} /></div>
        </div>
      </ListToolbar></div>
      {visibleJobs.length > 0 && <label className="mb-4 inline-flex items-center gap-2 text-sm ui-text-secondary"><SelectAllCheckbox bulk={bulk} />{t('bulk.selectAllVisible')}</label>}
      <BulkActions bulk={bulk} />
      <div className="space-y-4">
        {error && <InlineError title={t('printJobs.loadError')} message={printJobs ? t('common.existingDataSafe') : error} onRetry={refetch} />}
        {loading && !printJobs ? <div className="page-panel"><TableSkeleton columns={4} label={t('printJobs.loading')} /></div> : !printJobs || printJobs.length === 0 ? (
          !error && (list.hasFilters ? <div className="page-panel"><EmptyState filtered title={t('common.noResults')} description={t('printJobs.differentFilter')} action={<button onClick={list.clear} className="page-action page-action-secondary">{t('printJobs.clearSearch')}</button>} /></div> : <div className="page-panel"><EmptyState icon={Printer} title={t('dashboard.noJobs')} description={t('dashboard.noJobsHelp')} action={activePrinters.length ? <button onClick={handleAdd} className="page-action">{t('printJobs.addFirst')}</button> : <a href="#/printers" className="page-action">{t('printJobs.addPrinterFirst')}</a>} /></div>)
        ) : (
          <div className="space-y-4">
            {visibleJobs.length ? visibleJobs.map((job) => (
              <PrintJobCard key={job.id} job={job} onEdit={handleEdit} onDelete={handleDelete} deleting={deletingId === job.id}
                selection={<RowSelectionCheckbox bulk={bulk} id={job.id} label={modelName(job)} />} />
            )) : (
              <div className="page-panel"><EmptyState filtered title={t('common.noResults')} description={t('printJobs.differentFilter')} action={<button onClick={list.clear} className="page-action page-action-secondary">{t('printJobs.clearSearch')}</button>} /></div>
            )}
          </div>
        )}
      </div>
    </div>
  );
}

export default PrintJobs;
