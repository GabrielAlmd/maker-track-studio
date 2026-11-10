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
import {
  createPrintJobFilament,
  deletePrintJobFilament,
} from '../services/printJobFilamentService';
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
        filament_id: pjf.filament_id,
        weight_used: pjf.weight_used,
      }))
      : []
  );

  const addFilamentRow = () =>
    setFilamentRows((prev) => [...prev, { filament_id: '', weight_used: '' }]);

  const removeFilamentRow = (index) =>
    setFilamentRows((prev) => prev.filter((_, i) => i !== index));

  const updateFilamentRow = (index, field, value) =>
    setFilamentRows((prev) =>
      prev.map((row, i) => (i === index ? { ...row, [field]: value, ...(field === 'weight_used' ? { _automaticWeight: false } : {}) } : row))
    );

  const {
    register,
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

  const applyModelDefaults = (modelId, unitsValue, modelChanged = false, selectedModel) => {
    const model = selectedModel ?? availableModels.find((item) => item.id === modelId);
    const units = Number(unitsValue);
    if (!Number.isSafeInteger(units) || units < 1) return;

    if (modelChanged || automaticTime.current) {
      const time = Number(model?.print_time) * units;
      setValue('print_time_minutes', Number.isSafeInteger(time) && time > 0 ? time : '', {
        shouldDirty: true,
        shouldValidate: true,
      });
      automaticTime.current = !!model;
    }

    const materialWeight = Number(model?.material_weight);
    const wasteMaterial = Number(model?.waste_material ?? 0);
    const weight = (materialWeight + wasteMaterial) * units;
    const weightUsed = Number.isFinite(weight) && weight > 0 ? Number(weight.toFixed(2)) : '';
    setFilamentRows((rows) => {
      if (modelChanged) {
        return model && weightUsed > 0
          ? [{ filament_id: rows.length === 1 ? rows[0].filament_id : '', weight_used: weightUsed, _automaticWeight: true }]
          : rows.filter((row) => !row._automaticWeight);
      }
      return rows.map((row) => row._automaticWeight ? { ...row, weight_used: weightUsed } : row);
    });
  };

  const handleModelCreated = (createdModel) => {
    setAvailableModels((current) => [...current.filter((model) => model.id !== createdModel.id), createdModel]);
    setValue('model_id', createdModel.id, { shouldDirty: true, shouldValidate: true });
    applyModelDefaults(createdModel.id, getValues('units_printed'), true, createdModel);
    setCreatingModel(false);
    onModelCreated();
  };

  const syncFilaments = async (jobId, existingFilaments = []) => {
    // Delete all existing filament rows then re-insert (simple diff strategy)
    await Promise.all(
      existingFilaments
        .filter((pjf) => pjf.id)
        .map((pjf) => deletePrintJobFilament(pjf.id))
    );
    const validRows = filamentRows.filter(
      (r) => r.filament_id && Number(r.weight_used) > 0
    );
    await Promise.all(
      validRows.map((r) =>
        createPrintJobFilament({
          print_job_id: jobId,
          filament_id: r.filament_id,
          weight_used: Number(r.weight_used),
        })
      )
    );
  };

  const onSubmit = async (data) => {
    setSubmitError(null);
    try {
      await yup.array().of(yup.object({
        filament_id: yup.string().required(t('validation.selectFilamentRows')),
        weight_used: yup.number().typeError(t('validation.filamentWeightNumber'))
          .positive(t('validation.filamentWeightPositive')).required(t('validation.filamentWeightRequired'))
          .test('finite', t('validation.filamentWeightFinite'), Number.isFinite),
      })).validate(filamentRows);
      const payload = {
        printer_id: data.printer_id,
        model_id: data.model_id || null,
        print_date: new Date(data.print_date).toISOString(),
        units_printed: Number(data.units_printed),
        labor_hours: Number(data.labor_hours),
        print_time_minutes: data.print_time_minutes ? Number(data.print_time_minutes) : null,
      };
      if (isEdit) {
        await updatePrintJob(printJob.id, payload);
        await syncFilaments(printJob.id, printJob.filaments ?? []);
        toast.success(t('printJobs.updated'));
      } else {
        const created = await createPrintJob(payload);
        await syncFilaments(created.id);
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
          <select id="print-job-printer" aria-required="true" {...register('printer_id')}
            className="w-full border ui-border-default rounded-md px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ui-focus-ring">
            <option value="">— {t('printJobs.selectPrinter')} —</option>
            {printJob?.printer_id && !printers.some((printer) => printer.id === printJob.printer_id) &&
              <option value={printJob.printer_id}>{printJob.printer_cost?.printer_name ?? t('printJobs.archivedPrinter')} ({t('printers.archived').toLowerCase()})</option>}
            {printers.map((printer) => <option key={printer.id} value={printer.id}>{printer.name}{printer.is_default ? ` (${t('printers.default').toLowerCase()})` : ''}</option>)}
          </select>
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
          <div className="space-y-2">
            {filamentRows.map((row, index) => (
              <div key={index} className="flex gap-2 items-center">
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
                <button
                  type="button"
                  onClick={() => removeFilamentRow(index)}
                  className="ui-text-danger hover:ui-text-danger"
                  aria-label={t('printJobs.removeFilament')}
                >
                  <Trash2 className="h-4 w-4" />
                </button>
              </div>
            ))}
          </div>
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
          <label className="block">{t('printJobs.printer')}<select role="listbox" value={list.filters.printer_id || ''} onChange={(event) => list.changeFilters({ ...list.filters, printer_id: event.target.value })} className="mt-1 w-full rounded border p-2"><option value="">{t('printJobs.allPrinters')}</option>{printers?.map((printer) => <option key={printer.id} value={printer.id}>{printer.name}{printer.is_archived ? ` (${t('printers.archived').toLowerCase()})` : ''}</option>)}</select></label>
          <div><span className="block mb-1">{t('dashboard.model')}</span><ModelSelect aria-label={t('dashboard.model')} value={list.filters.model_id || ''} onChange={(modelId) => list.changeFilters({ ...list.filters, model_id: modelId })} models={models ?? []} emptyLabel={t('printJobs.allModels')} /></div>
          <label className="block">{t('dashboard.filament')}<select role="listbox" value={list.filters.filament_id || ''} onChange={(event) => list.changeFilters({ ...list.filters, filament_id: event.target.value })} className="mt-1 w-full rounded border p-2"><option value="">{t('printJobs.allFilaments')}</option>{filaments?.map((filament) => <option key={filament.id} value={filament.id}>{filament.brand} {filament.type} — {filament.color}</option>)}</select></label>
        </div>
      </ListToolbar></div>
      {visibleJobs.length > 0 && <label className="mb-4 inline-flex items-center gap-2 text-sm ui-text-secondary"><SelectAllCheckbox bulk={bulk} />{t('bulk.selectAllVisible')}</label>}
      <BulkActions bulk={bulk} />
      <div className="space-y-4">
        {error && <InlineError title={t('printJobs.loadError')} message={printJobs ? t('common.existingDataSafe') : error} onRetry={refetch} />}
        {loading && !printJobs ? <div className="page-panel"><TableSkeleton columns={4} label={t('printJobs.loading')} /></div> : !printJobs || printJobs.length === 0 ? (
          !error && <div className="page-panel"><EmptyState icon={Printer} title={t('dashboard.noJobs')} description={t('dashboard.noJobsHelp')} action={activePrinters.length ? <button onClick={handleAdd} className="page-action">{t('printJobs.addFirst')}</button> : <a href="#/printers" className="page-action">{t('printJobs.addPrinterFirst')}</a>} /></div>
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
