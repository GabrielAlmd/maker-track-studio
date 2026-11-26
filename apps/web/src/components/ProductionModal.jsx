import FormSelect from './ui/FormSelect';
import { useEffect, useRef, useState } from 'react';
import { useFieldArray, useForm } from 'react-hook-form';
import { yupResolver } from '@hookform/resolvers/yup';
import * as yup from 'yup';
import { useTranslation } from 'react-i18next';
import api from '../services/api';
import { readSlicerFiles } from '../utils/slicerImport';
import Modal from './ui/Modal';
import MaterialEstimateDetails from './MaterialEstimateDetails';
import { useEstimateStock } from '../hooks/useEstimateStock';
import { combineMappedEstimates, estimateRunMaterials } from '../utils/productionEstimates';

const count = yup.number().integer().min(0).max(1000000000).required();
const schema = t => yup.object({
  printer_id: yup.string().required('Select a printer'),
  print_date: yup.string().required(),
  runs: yup.array().min(1).max(100).of(yup.object({
    status: yup.string().oneOf(['planned', 'in_progress', 'completed', 'failed', 'cancelled']).required(),
    labor_hours: yup.number().min(0).max(1000000).required(),
    estimated_time_minutes: count.nullable().transform((v, raw) => raw === '' ? null : v),
    print_time_minutes: count.nullable().transform((v, raw) => raw === '' ? null : v),
    items: yup.array().min(1).max(100).of(yup.object({ planned: count.min(1), attempted: count, accepted: count, rejected: count })),
    materials: yup.array().max(100).of(yup.object({ filament_id: yup.string().required(t('validation.selectFilamentRows')), weight_used: yup.number().positive().max(1e9).required(), waste_weight_g: yup.number().min(0).max(1e9).nullable()
      .transform((v, raw) => raw === '' ? null : v)
      .test('within-total', t('validation.wasteWithinTotal'), function (value) {
        return value == null || value <= Number(this.parent.weight_used);
      }) })).test('duplicates', t('validation.duplicateFilament'), rows => {
        const mapped = combineMappedEstimates(rows ?? []).filter(row => row.filament_id);
        return new Set(mapped.map(row => row.filament_id)).size === mapped.length;
      }).test('total', t('materialEstimates.weightLimit'), rows =>
        combineMappedEstimates(rows ?? []).every(row => !(Number(row.weight_used) > 1e9))),
  })),
});
const isTerminal = status => ['completed', 'failed', 'cancelled'].includes(status);
const newItem = () => ({ planned: 1, attempted: 0, accepted: 0, rejected: 0, model_id: null });
const newRun = () => ({ status: 'planned', labor_hours: 0, print_time_minutes: null, estimated_time_minutes: null, items: [newItem()], materials: [] });
const fieldClass = 'mt-1 w-full rounded-lg border ui-border-subtle ui-surface-input ui-text-primary p-2';

function RunFields({ index, control, register, watch, getValues, setValue, errors, models, filaments, filamentsLoading, stock, initial, onRemove }) {
  const { t } = useTranslation();
  const base = `runs.${index}`;
  const items = useFieldArray({ control, name: `${base}.items`, keyName: 'formKey' });
  const materials = useFieldArray({ control, name: `${base}.materials`, keyName: 'formKey' });
  const status = watch(`${base}.status`);
  const sourceLabel = watch(`${base}.source_label`);
  const materialRows = watch(`${base}.materials`) ?? [];
  const overrides = useRef(Boolean(initial?.materials?.length || sourceLabel));
  const [conflict, setConflict] = useState(false);
  const canReplace = !initial?.materials?.length;
  const updateEstimates = (replace = false) => {
    if (status !== 'planned') return;
    if (overrides.current && !replace) {
      setConflict(true);
      return;
    }
    materials.replace(estimateRunMaterials(getValues(`${base}.items`), models, filaments, stock));
    overrides.current = false;
    setConflict(false);
  };
  const terminal = isTerminal(initial?.status);
  const statuses = terminal ? [initial.status] : initial?.status === 'in_progress' ? ['in_progress', 'completed', 'failed', 'cancelled'] : initial ? ['planned', 'in_progress', 'cancelled'] : ['planned'];
  return <fieldset className="rounded-xl border ui-border-subtle p-4 space-y-4" disabled={terminal}>
    <legend className="px-2 font-semibold">{t('production.run')} {index + 1}</legend>
    {sourceLabel && <p className="text-sm ui-text-muted">{t('slicer.imported')}: {sourceLabel}</p>}
    <label className="block">{t('slicer.minutes')}<input className={fieldClass} type="number" min="0" max="1000000000" step="1" {...register(`${base}.estimated_time_minutes`)} /></label>
    <label className="block">{t('production.status')}<select className={fieldClass} {...register(`${base}.status`)}>{statuses.map(value => <option key={value} value={value}>{t(`production.${value}`)}</option>)}</select></label>
    {terminal && <p className="text-sm ui-text-muted">{t('production.terminal')}</p>}
    {items.fields.map((item, n) => <div key={item.formKey} className="grid grid-cols-2 gap-3">
      {item.source_model && <p className="col-span-2 text-sm ui-text-muted">{t('slicer.imported')}: {item.source_model}</p>}
      <div className="col-span-2"><FormSelect label={t('production.model')} control={control} disabled={terminal || filamentsLoading} name={`${base}.items.${n}.model_id`}
        options={models} onSelect={() => updateEstimates()} clearLabel={t('production.uncatalogued')} /></div>
      {['planned', 'attempted', 'accepted', 'rejected'].map(name => <label key={name} className="capitalize">{t(`production.${name}`)}<input className={fieldClass} type="number" min={name === 'planned' ? 1 : 0} max={1000000000} step="1" {...register(`${base}.items.${n}.${name}`, name === 'planned' ? { onChange: () => updateEstimates() } : undefined)} /></label>)}
    </div>)}
    {(!initial || initial.status === 'planned') && <button type="button" className="page-action page-action-secondary" onClick={() => items.append(newItem())}>{t('production.addModel')}</button>}
    <p className="text-sm ui-text-muted">{t('production.quantities')}</p>
    {status !== 'planned' && <div className="grid grid-cols-2 gap-3">
      <label>{t('production.actualMinutes')}<input className={fieldClass} type="number" min="0" max="1000000000" step="1" {...register(`${base}.print_time_minutes`)} /></label>
      <label>{t('production.actualLabor')}<input className={fieldClass} type="number" min="0" max="1000000" step="0.01" {...register(`${base}.labor_hours`)} /></label>
    </div>}
    {conflict && <div className="text-sm ui-text-muted" role="status">
      <p>{t('materialEstimates.kept')}</p>
      {canReplace && <button type="button" className="page-action page-action-secondary" onClick={() => updateEstimates(true)}>
        {t('materialEstimates.replace')}
      </button>}
    </div>}
    {errors?.materials?.message && <p role="alert" className="ui-text-danger">{errors.materials.message}</p>}
    {materials.fields.map((material, n) => {
      const name = `${base}.materials.${n}`;
      const markOverride = () => { overrides.current = true; };
      return <div key={material.formKey} className="grid grid-cols-2 gap-3">
        {material.source_tool && <p className="col-span-2 text-sm ui-text-muted">{t('slicer.tool', { number: material.source_tool })}</p>}
        <div className="col-span-2"><FormSelect label={t('production.filament')} control={control} name={`${name}.filament_id`}
          options={filaments.filter(f => !f.archived || f.id === material.filament_id)} clearLabel={t('production.selectFilament')} required
          disabled={terminal} onSelect={markOverride} getLabel={f => [f.brand, f.type].join(' ') + ' — ' + f.color} /></div>
        <label>{t('production.grams')}<input className={fieldClass} type="number" min="0.001" max="1000000000" step="0.001"
          {...register(`${name}.weight_used`, { onChange: markOverride })} /></label>
        <label>{t('production.waste')}<input className={fieldClass} type="number" min="0" max="1000000000" step="0.001"
          {...register(`${name}.waste_weight_g`, { onChange: markOverride })} /></label>
        <label>{t('production.measurement')}<select className={fieldClass} {...register(`${name}.measurement_state`, {
          onChange: event => {
            markOverride();
            if (event.target.value === 'verified') {
              const estimate = Number(getValues(`${name}.weight_used`));
              if (Number.isFinite(estimate) && estimate > 0) setValue(`${name}.estimated_weight_g`, estimate);
              setValue(`${name}.weight_used`, '');
              setValue(`${name}.waste_weight_g`, '');
            }
          },
        })}><option value="estimated">{t('production.estimate')}</option>{status !== 'planned' && <option value="verified">{t('production.actual')}</option>}</select></label>
        {!terminal && !material.id && <button type="button" className="page-action page-action-secondary self-end" onClick={() => {
          markOverride();
          materials.remove(n);
        }}>{t('printJobs.removeFilament')}</button>}
        {errors?.materials?.[n]?.filament_id && <p role="alert" className="col-span-2 ui-text-danger">{errors.materials[n].filament_id.message}</p>}
        <MaterialEstimateDetails row={materialRows[n] ?? material} rows={materialRows} filaments={filaments} stock={stock} />
      </div>;
    })}
    <button type="button" className="page-action page-action-secondary" onClick={() => {
      overrides.current = true;
      materials.append({ filament_id: '', weight_used: 1, measurement_state: 'estimated' });
    }}>{t('production.addMaterial')}</button>
    <p className="text-sm ui-text-muted">{t('production.weights')}</p>
    {onRemove && <button type="button" className="page-action page-action-secondary" onClick={onRemove}>{t('slicer.remove')}</button>}
  </fieldset>;
}

export default function ProductionModal({ job, models = [], filaments = [], filamentsLoading = false, printers = [], onClose, onSaved }) {
  const { t } = useTranslation();
  const [importing, setImporting] = useState(false);
  const [importError, setImportError] = useState('');
  const [imported, setImported] = useState(false);
  const [loading, setLoading] = useState(Boolean(job));
  const [error, setError] = useState('');
  const [initial, setInitial] = useState([]);
  const retry = useRef(null);
  const { stock } = useEstimateStock();
  const { control, register, watch, getValues, setValue, reset, handleSubmit, formState: { isSubmitting, errors } } = useForm({
    resolver: yupResolver(schema(t)),
    defaultValues: { printer_id: job?.printer_id || printers.find(p => p.is_default)?.id || '', print_date: job?.print_date?.slice(0, 10) || new Date().toISOString().slice(0, 10), expected_revision: 0, runs: [newRun()] },
  });
  const runs = useFieldArray({ control, name: 'runs', keyName: 'formKey' });
  useEffect(() => {
    if (!job) return;
    let active = true;
    api.get(`/v2/production/jobs/${job.id}`).then(({ data }) => {
      if (!active) return;
      setInitial(data.runs);
      reset({ printer_id: job.printer_id || '', print_date: job.print_date.slice(0, 10), expected_revision: data.revision, runs: data.runs });
      setLoading(false);
    }).catch(() => { if (active) { setError('Unable to load production. Close and try again.'); } });
    return () => { active = false; };
  }, [job, reset]);
  const importFiles = async event => {
    const files = [...event.target.files];
    event.target.value = '';
    if (!files.length) return;
    setImporting(true);
    setImportError('');
    try {
      const parsed = await readSlicerFiles(files);
      if (watch('runs').length + parsed.length > 100) throw new Error();
      runs.append(parsed.map(plate => ({ ...newRun(), ...plate, items: plate.items?.map(item => ({ ...newItem(), ...item })) || [{ ...newItem(), planned: '' }] })));
      setImported(true);
    } catch { setImportError(t('slicer.invalid')); }
    finally { setImporting(false); }
  };
  const submit = async values => {
    const body = {
      ...values,
      print_date: new Date(values.print_date).toISOString(),
      runs: values.runs.map((run, index) => {
        const input = { ...run };
        delete input.source_label;
        return {
          ...input,
          items: run.items.map(row => {
            const item = { ...row, model_id: row.model_id || null };
            delete item.source_model;
            return item;
          }),
          materials: combineMappedEstimates(run.materials ?? []).map(row => {
            const material = { ...row };
            delete material.source_tool;
            delete material._sources;
            return {
              ...material,
              estimated_weight_g: material.measurement_state === 'estimated' && !isTerminal(initial[index]?.status)
                ? material.weight_used : material.estimated_weight_g ?? null,
            };
          }),
        };
      }),
    };
    const signature = JSON.stringify(body);
    if (retry.current?.signature !== signature) retry.current = { signature, operation_id: crypto.randomUUID() };
    try {
      setError('');
      await api.request({ method: job ? 'put' : 'post', url: job ? `/v2/production/jobs/${job.id}` : '/v2/production/jobs', data: { ...body, operation_id: retry.current.operation_id } });
      onSaved();
    } catch (failure) { setError(failure.response?.status === 409 ? 'This production changed or the requested transition is not allowed. Reopen the job to load its current state.' : (typeof failure.response?.data?.error === 'string' ? failure.response.data.error : '') || 'Unable to save production. Your changes are still here.'); }
  };
  return <Modal title={t(job ? 'production.runs' : 'production.plan')} onClose={onClose} size="wide">
    {error && <p role="alert" className="mb-4 text-red-600">{error}</p>}
    {loading ? <p role="status">{t('production.loading')}</p> : <form onSubmit={handleSubmit(submit)} className="space-y-5">
      <section className="rounded-xl border ui-border-subtle p-4 space-y-2">
        <label className="block font-semibold">{t('slicer.title')}<input className={fieldClass} type="file" accept=".gcode,.3mf" multiple disabled={importing || isSubmitting} onChange={importFiles} /></label>
        <p className="text-sm ui-text-muted">{t('slicer.help')}</p>
        {importing && <p role="status">{t('slicer.reading')}</p>}
        {importError && <p role="alert" className="text-red-600">{importError}</p>}
        {imported && <p role="status" className="text-sm ui-text-muted">{t('slicer.review')}</p>}
      </section>
      <div className="block"><FormSelect label={t('production.printer')} control={control} name={'printer_id'}
        options={printers} clearLabel={t('production.selectPrinter')} required /></div>
      <label className="block">{t('production.date')}<input className={fieldClass} type="date" {...register('print_date')} /></label>
      {filamentsLoading && <p role="status" className="text-sm ui-text-muted">{t('materialEstimates.loading')}</p>}
      {runs.fields.map((run, index) => <RunFields key={run.formKey} index={index} control={control} register={register} watch={watch} getValues={getValues} setValue={setValue} stock={stock} errors={errors.runs?.[index]} models={models} filaments={filaments} filamentsLoading={filamentsLoading} initial={initial[index]} onRemove={index >= initial.length && runs.fields.length > 1 ? () => runs.remove(index) : undefined} />)}
      <button type="button" className="page-action page-action-secondary" onClick={() => runs.append(newRun())}>{t('production.addRun')}</button>
      {Object.keys(errors).length > 0 && <p role="alert" className="text-red-600">{t('production.validation')}</p>}
      <div className="flex justify-end gap-3"><button type="button" className="page-action page-action-secondary" onClick={onClose}>{t('common.cancel')}</button><button className="page-action" disabled={isSubmitting || importing}>{t(isSubmitting ? 'common.saving' : 'production.save')}</button></div>
    </form>}
  </Modal>;
}
