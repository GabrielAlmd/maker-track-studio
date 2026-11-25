import { useState } from 'react';
import { useFieldArray, useForm, useWatch } from 'react-hook-form';
import { yupResolver } from '@hookform/resolvers/yup';
import * as yup from 'yup';
import toast from 'react-hot-toast';
import { Loader2 } from 'lucide-react';
import Modal from './ui/Modal';
import { createModel, updateModel } from '../services/modelService';
import { useTranslation } from 'react-i18next';
import { ModelMaterialsEditor } from './ModelMaterials';
import { maxModelGrams, maxRequirements, summarizeModelMaterials } from '../utils/modelMaterials';

const optionalNumber = (message) => yup.number().nullable().transform((value, original) => original === '' ? null : value).typeError(message);
const modelSchema = (t) => {
  const grams = (message) => optionalNumber(message).min(0, t('validation.nonNegative')).max(maxModelGrams, t('modelMaterials.invalidTotal'));
  return yup.object({
    name: yup.string().required(t('validation.nameRequired')),
    image_url: yup.string().url(t('validation.validUrl')).nullable().transform((v) => v || null),
    material_weight: grams(t('validation.materialNumber')).when('material_requirements', {
      is: rows => !rows?.length,
      then: schema => schema.required(t('validation.materialNumber')),
      otherwise: schema => schema.strip(),
    }),
    print_time: yup.number().typeError(t('validation.printTimeNumber')).integer(t('validation.wholeNumber')).positive(t('validation.positive')).required(t('validation.printTimeRequired')),
    waste_material: grams(t('validation.wasteNumber')).when('material_requirements', {
      is: rows => !rows?.length,
      then: schema => schema.required(t('validation.wasteRequired')),
      otherwise: schema => schema.strip(),
    }),
    is_multicolor: yup.boolean(),
    material_requirements: yup.array().of(yup.object({
      id: yup.string().optional(),
      material_type: yup.string().trim().max(100).required(t('modelMaterials.typeRequired')),
      color: yup.string().trim().max(100),
      part_weight_g: grams(t('validation.materialNumber')).required(t('validation.materialRequired')),
      waste_weight_g: grams(t('validation.wasteNumber')),
      preferred_filament_id: yup.string().nullable().transform(value => value || null),
      source_tool: yup.number().integer().min(1).max(100).nullable(),
      review_grams: yup.mixed().strip(),
    })).max(maxRequirements).test('total-grams', t('modelMaterials.invalidTotal'), (rows) => {
      const summary = summarizeModelMaterials(rows ?? []);
      return (summary.part_weight_g ?? 0) + summary.known_waste_g <= maxModelGrams;
    }),
  });
};

export default function ModelModal({ model, onClose, onSaved }) {
  const { t } = useTranslation();
  const isEdit = !!model;
  const [submitError, setSubmitError] = useState(null);
  const { control, register, handleSubmit, formState: { errors, isSubmitting } } = useForm({
    resolver: yupResolver(modelSchema(t)),
    defaultValues: model
      ? {
          name: model.name,
          image_url: model.image_url || '',
          material_weight: model.material_weight,
          print_time: model.print_time,
          waste_material: model.waste_material,
          is_multicolor: model.is_multicolor,
          material_requirements: model.material_requirements?.map(row => ({
            ...row, waste_weight_g: row.waste_weight_g ?? '', preferred_filament_id: row.preferred_filament_id ?? '',
          })) ?? [],
        }
      : { waste_material: 0, is_multicolor: false, material_requirements: [] },
  });

  const { fields, append, remove } = useFieldArray({ control, name: 'material_requirements', keyName: 'formKey' });
  const rows = useWatch({ control, name: 'material_requirements' });

  const onSubmit = async (data) => {
    setSubmitError(null);
    try {
      const { material_requirements, ...payload } = data;
      payload.image_url ||= '';
      if (material_requirements.length || model?.material_requirements?.length) {
        payload.material_requirements = material_requirements;
      }
      const savedModel = isEdit
        ? await updateModel(model.id, payload)
        : await createModel(payload);
      toast.success(t(isEdit ? 'models.updated' : 'models.created'));
      onSaved(savedModel);
    } catch (error) {
      setSubmitError(error?.message || t('models.saveError'));
    }
  };

  return (
    <Modal title={t(isEdit ? 'models.editTitle' : 'models.add')} onClose={onClose} size="wide">
      <form noValidate onSubmit={handleSubmit(onSubmit)} className="space-y-4">
        {submitError && <p className="page-form-error" role="alert">{submitError}</p>}
        <div>
          <label htmlFor="model-name" className="block text-sm font-medium ui-text-secondary mb-1">{t('models.name')}</label>
          <input id="model-name" {...register('name')} className="w-full border ui-border-default rounded-md px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ui-focus-ring" placeholder="e.g. Benchy" />
          {errors.name && <p className="ui-text-danger text-xs mt-1">{errors.name.message}</p>}
        </div>
        <div>
          <label htmlFor="model-image-url" className="block text-sm font-medium ui-text-secondary mb-1">{t('models.imageUrl')}</label>
          <input id="model-image-url" {...register('image_url')} className="w-full border ui-border-default rounded-md px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ui-focus-ring" placeholder="https://..." />
          {errors.image_url && <p className="ui-text-danger text-xs mt-1">{errors.image_url.message}</p>}
        </div>
        <div>
          <label className="block text-sm font-medium ui-text-secondary mb-1">
            {t('models.printTimeMinutes')}
            <input type="number" min="1" max="1000000000" step="1" {...register('print_time')} className="w-full border ui-border-default rounded-md px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ui-focus-ring" placeholder="e.g. 120" />
          </label>
          {errors.print_time && <p className="ui-text-danger text-xs mt-1">{errors.print_time.message}</p>}
        </div>
        {fields.length === 0 && <>
          <div className="grid grid-cols-2 gap-4">
            <div>
              <label className="block text-sm font-medium ui-text-secondary mb-1">
                {t('models.materialWeight')}
                <input type="number" min="0" max={maxModelGrams} step="any" {...register('material_weight')} className="w-full border ui-border-default rounded-md px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ui-focus-ring" placeholder="e.g. 15.5" />
              </label>
              {errors.material_weight && <p className="ui-text-danger text-xs mt-1">{errors.material_weight.message}</p>}
            </div>
            <div>
              <label className="block text-sm font-medium ui-text-secondary mb-1">
                {t('models.wasteMaterial')}
                <input type="number" min="0" max={maxModelGrams} step="any" {...register('waste_material')} className="w-full border ui-border-default rounded-md px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ui-focus-ring" placeholder="e.g. 2.0" />
              </label>
              {errors.waste_material && <p className="ui-text-danger text-xs mt-1">{errors.waste_material.message}</p>}
            </div>
          </div>
          <div className="flex items-center gap-2">
            <input type="checkbox" id="is_multicolor" {...register('is_multicolor')} className="h-4 w-4 ui-text-primary-action rounded ui-border-default focus:ui-focus-ring" />
            <label htmlFor="is_multicolor" className="text-sm font-medium ui-text-secondary">{t('models.multicolorPrint')}</label>
          </div>
        </>}
        <ModelMaterialsEditor {...{ fields, rows, append, remove, register, control }} errors={errors.material_requirements} disabled={isSubmitting} />
        <div className="flex justify-end gap-3 pt-2">
          <button type="button" onClick={onClose} className="px-4 py-2 text-sm ui-text-secondary border ui-border-default rounded-md hover:ui-surface-hover">{t('common.cancel')}</button>
          <button type="submit" disabled={isSubmitting} className="px-4 py-2 text-sm ui-text-inverse ui-bg-primary rounded-md hover:ui-bg-primary-hover disabled:opacity-50 flex items-center gap-2" aria-busy={isSubmitting}>
            {isSubmitting && <Loader2 className="h-4 w-4 animate-spin" />}
            {isSubmitting ? t('common.saving') : isEdit ? t('common.saveChanges') : t('models.create')}
          </button>
        </div>
      </form>
    </Modal>
  );
}
