import { useId, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useApi } from '../hooks/useApi';
import { getFilaments } from '../services/filamentService';
import { blankRequirement, maxModelGrams, maxRequirements, summarizeModelMaterials } from '../utils/modelMaterials';
import { formatNumber } from '../utils/formatters';
import { readSlicerFiles } from '../utils/slicerImport';
import FormSelect from './ui/FormSelect';
import Modal from './ui/Modal';

const fieldClass = 'w-full border ui-border-default rounded-md px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ui-focus-ring';
const secondaryAction = 'page-action page-action-secondary';
const filamentLabel = filament => [filament.brand, filament.type, filament.color].filter(Boolean).join(' · ');

function grams(value, t) {
  return value == null ? t('common.unknown') : t('modelMaterials.grams', { grams: formatNumber(value, { maximumFractionDigits: 6 }) });
}

function MaterialSummary({ summary }) {
  const { t } = useTranslation();
  return <dl className="grid grid-cols-2 gap-2 text-sm ui-text-secondary">
    <div><dt>{t('modelMaterials.partTotal')}</dt><dd>{grams(summary.part_weight_g, t)}</dd></div>
    <div><dt>{t(summary.waste_complete ? 'modelMaterials.wasteTotal' : 'modelMaterials.knownWaste')}</dt><dd>{grams(summary.known_waste_g, t)}</dd></div>
    <div><dt>{t('modelMaterials.colorStatus')}</dt><dd>{t(`modelMaterials.${summary.color_status}`)}</dd></div>
    <div><dt>{t('modelMaterials.materialStatus')}</dt><dd>{t(`modelMaterials.${summary.material_status}`)}</dd></div>
  </dl>;
}

function ModelSlicerReview({ append, disabled }) {
  const { t } = useTranslation();
  const [review, setReview] = useState(null);
  const [reading, setReading] = useState(false);
  const [error, setError] = useState(false);

  async function inspect(event) {
    const files = Array.from(event.currentTarget.files);
    event.currentTarget.value = '';
    if (!files.length) return;
    setReview(null);
    setReading(true);
    setError(false);
    try {
      setReview(await readSlicerFiles(files));
    } catch {
      setError(true);
    } finally {
      setReading(false);
    }
  }

  const tools = review?.length === 1 ? review[0].materials.filter(row => row.source_tool != null) : [];
  return <div className="space-y-2">
    <label className="block text-sm">{t('modelMaterials.inspect')}
      <input type="file" multiple accept=".gcode,.3mf" onChange={inspect} disabled={reading || disabled} className={fieldClass} />
    </label>
    {reading && <p role="status">{t('modelMaterials.reading')}</p>}
    {error && <p role="alert" className="page-form-error">{t('modelMaterials.invalidFile')}</p>}
    {review && <div aria-label={t('modelMaterials.review')} className="rounded border ui-border-default p-3 text-sm space-y-2">
      <p className="font-semibold">{t('modelMaterials.review')}</p>
      <p>{t(review.length > 1 ? 'modelMaterials.multiplePlates' : tools.length ? 'modelMaterials.reviewTools' : 'modelMaterials.noToolWeights')}</p>
      <ul className="list-disc pl-5 space-y-1">
        {review.map((plate, index) => <li key={index}>
          <span className="break-all">{plate.source_label}</span>
          {plate.items?.length > 0 && <ul className="list-disc pl-5">
            {plate.items.map((item, itemIndex) => <li key={itemIndex}>{item.source_model}</li>)}
          </ul>}
        </li>)}
        {tools.map(tool => <li key={tool.source_tool}>
          {t('modelMaterials.tool', { number: tool.source_tool })} · {grams(tool.estimated_weight_g, t)}
        </li>)}
      </ul>
      {tools.length > 0 && <button type="button" disabled={disabled} className={secondaryAction} onClick={() => {
        append(tools.map(tool => ({ ...blankRequirement, source_tool: tool.source_tool, review_grams: tool.estimated_weight_g })));
        setReview(null);
      }}>{t('modelMaterials.addTools')}</button>}
      <button type="button" className={secondaryAction} onClick={() => setReview(null)}>{t('common.close')}</button>
    </div>}
  </div>;
}

function MaterialRequirementField({ row, index, register, control, errors, remove, catalog }) {
  const { t } = useTranslation();
  const { data, loading, error } = catalog;
  const prefix = `material_requirements.${index}`;
  const fieldPrefix = useId();
  return <fieldset className="min-w-0 rounded border ui-border-default p-3 space-y-2 text-sm">
    <legend className="px-1">{row.source_tool ? t('modelMaterials.tool', { number: row.source_tool }) : t('modelMaterials.requirement', { number: index + 1 })}</legend>
    {row.review_grams != null && <p className="ui-text-muted">{t('modelMaterials.reportedGrams', { weight: grams(row.review_grams, t) })}</p>}
    <div className="grid grid-cols-1 sm:grid-cols-2 gap-2">
      {[['material_type', 'type'], ['color', 'color'], ['part_weight_g', 'part'], ['waste_weight_g', 'waste']].map(([field, label]) => {
        const numeric = field.endsWith('_g');
        const fieldError = errors?.[field];
        const id = `${fieldPrefix}-${field}`;
        return <div key={field}>
          <label htmlFor={id}>{t(`modelMaterials.${label}`)}</label>
          <input id={id} className={fieldClass} type={numeric ? 'number' : 'text'} aria-invalid={fieldError ? true : undefined}
            aria-describedby={fieldError ? `${id}-error` : undefined}
            {...(numeric ? { min: 0, max: maxModelGrams, step: 'any' } : { maxLength: 100 })}
            {...register(`${prefix}.${field}`)} />
          {fieldError && <span id={`${id}-error`} className="ui-text-danger block" role="alert">{fieldError.message}</span>}
        </div>;
      })}
    </div>
    <FormSelect name={`${prefix}.preferred_filament_id`} control={control} label={t('modelMaterials.preference')}
      options={data ?? []} getLabel={filamentLabel} loading={loading} error={error ? t('modelMaterials.catalogError') : undefined}
      clearLabel={t('modelMaterials.noPreference')} />
    <button type="button" className={secondaryAction} onClick={() => remove(index)}>{t('modelMaterials.remove', { number: index + 1 })}</button>
  </fieldset>;
}

function MaterialRequirementFields({ fields, register, control, remove, errors }) {
  const catalog = useApi(getFilaments);
  return fields.map((row, index) => <MaterialRequirementField key={row.formKey} {...{ row, index, register, control, remove, catalog }}
    errors={errors?.[index]} />);
}

export function ModelMaterialsEditor({ fields, rows, append, remove, register, control, errors, disabled }) {
  const { t } = useTranslation();
  return <section aria-label={t('modelMaterials.title')} className="space-y-3">
    <h3 className="font-semibold">{t('modelMaterials.title')}</h3>
    <p className="text-sm ui-text-muted">{t(fields.length ? 'modelMaterials.perUnit' : 'modelMaterials.legacy')}</p>
    <ModelSlicerReview append={append} disabled={disabled || fields.length > 0} />
    {fields.length > 0 && <MaterialRequirementFields {...{ fields, register, control, remove, errors }} />}
    {errors?.message && <p role="alert" className="ui-text-danger text-sm">{errors.message}</p>}
    {fields.length > 0 && <MaterialSummary summary={summarizeModelMaterials(rows)} />}
    <button type="button" disabled={disabled || fields.length >= maxRequirements} className={secondaryAction}
      onClick={() => append({ ...blankRequirement })}>{t('modelMaterials.add')}</button>
  </section>;
}

function RequirementDetails({ rows, filaments }) {
  const { t } = useTranslation();
  return <ul className="space-y-2">
    {rows.map(row => {
      const preferred = filaments.find(filament => filament.id === row.preferred_filament_id);
      return <li key={row.id} className="rounded border ui-border-default p-3 text-sm">
        <p className="font-semibold">{row.material_type} · {row.color || t('common.unknown')}</p>
        <p>{t('modelMaterials.partDetails', { weight: grams(row.part_weight_g, t) })} · {t('modelMaterials.wasteDetails', { weight: grams(row.waste_weight_g, t) })}</p>
        {row.preferred_filament_id && <p>{t('modelMaterials.preferenceDetails')} · {preferred ? filamentLabel(preferred) : t('common.unknown')}</p>}
        {row.source_tool && <p>{t('modelMaterials.tool', { number: row.source_tool })}</p>}
      </li>;
    })}
  </ul>;
}

function RequirementDetailsWithCatalog({ rows }) {
  const { data } = useApi(getFilaments);
  return <RequirementDetails rows={rows} filaments={data ?? []} />;
}

export function ModelDetails({ model, onClose }) {
  const { t } = useTranslation();
  const rows = model.material_requirements ?? [];
  return <Modal title={model.name} onClose={onClose} size="wide">
    <div className="space-y-3">
      <h3 className="font-semibold">{t('modelMaterials.title')}</h3>
      {rows.length > 0 ? <>
        <p className="text-sm ui-text-muted">{t('modelMaterials.perUnitDetails')}</p>
        <MaterialSummary summary={model.material_summary} />
        {rows.some(row => row.preferred_filament_id)
          ? <RequirementDetailsWithCatalog rows={rows} />
          : <RequirementDetails rows={rows} filaments={[]} />}
      </> : <>
        <p className="text-sm ui-text-muted">{t('modelMaterials.legacy')}</p>
        <dl className="text-sm">
          <dt>{t('models.materialWeight')}</dt><dd>{grams(model.material_weight, t)}</dd>
          <dt>{t('models.wasteMaterial')}</dt><dd>{grams(model.waste_material, t)}</dd>
          <dt>{t('models.multicolorPrint')}</dt><dd>{t(model.is_multicolor ? 'common.yes' : 'common.no')}</dd>
        </dl>
      </>}
    </div>
  </Modal>;
}
