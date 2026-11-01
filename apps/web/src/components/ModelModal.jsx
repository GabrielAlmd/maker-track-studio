import { useState } from 'react';
import { useForm } from 'react-hook-form';
import { yupResolver } from '@hookform/resolvers/yup';
import * as yup from 'yup';
import toast from 'react-hot-toast';
import { Loader2 } from 'lucide-react';
import Modal from './ui/Modal';
import { createModel, updateModel } from '../services/modelService';
import { useTranslation } from 'react-i18next';

const modelSchema = (t) => yup.object({
  name: yup.string().required(t('validation.nameRequired')),
  image_url: yup.string().url(t('validation.validUrl')).nullable().transform((v) => v || null),
  material_weight: yup.number().typeError(t('validation.materialNumber')).positive(t('validation.positive')).required(t('validation.materialRequired')),
  print_time: yup.number().typeError(t('validation.printTimeNumber')).integer(t('validation.wholeNumber')).positive(t('validation.positive')).required(t('validation.printTimeRequired')),
  waste_material: yup.number().typeError(t('validation.wasteNumber')).min(0, t('validation.nonNegative')).required(t('validation.wasteRequired')),
  is_multicolor: yup.boolean(),
});

export default function ModelModal({ model, onClose, onSaved }) {
  const { t } = useTranslation();
  const isEdit = !!model;
  const [submitError, setSubmitError] = useState(null);
  const { register, handleSubmit, formState: { errors, isSubmitting } } = useForm({
    resolver: yupResolver(modelSchema(t)),
    defaultValues: model
      ? {
          name: model.name,
          image_url: model.image_url || '',
          material_weight: model.material_weight,
          print_time: model.print_time,
          waste_material: model.waste_material,
          is_multicolor: model.is_multicolor,
        }
      : { waste_material: 0, is_multicolor: false },
  });

  const onSubmit = async (data) => {
    setSubmitError(null);
    try {
      const payload = {
        ...data,
        material_weight: Number(data.material_weight),
        print_time: Number(data.print_time),
        waste_material: Number(data.waste_material),
        image_url: data.image_url || '',
      };
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
    <Modal title={t(isEdit ? 'models.editTitle' : 'models.add')} onClose={onClose}>
      <form onSubmit={handleSubmit(onSubmit)} className="space-y-4">
        {submitError && <p className="page-form-error" role="alert">{submitError}</p>}
        <div>
          <label className="block text-sm font-medium ui-text-secondary mb-1">{t('models.name')}</label>
          <input {...register('name')} className="w-full border ui-border-default rounded-md px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ui-focus-ring" placeholder="e.g. Benchy" />
          {errors.name && <p className="ui-text-danger text-xs mt-1">{errors.name.message}</p>}
        </div>
        <div>
          <label className="block text-sm font-medium ui-text-secondary mb-1">{t('models.imageUrl')}</label>
          <input {...register('image_url')} className="w-full border ui-border-default rounded-md px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ui-focus-ring" placeholder="https://..." />
          {errors.image_url && <p className="ui-text-danger text-xs mt-1">{errors.image_url.message}</p>}
        </div>
        <div className="grid grid-cols-2 gap-4">
          <div>
            <label className="block text-sm font-medium ui-text-secondary mb-1">{t('models.materialWeight')}</label>
            <input type="number" step="0.01" {...register('material_weight')} className="w-full border ui-border-default rounded-md px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ui-focus-ring" placeholder="e.g. 15.5" />
            {errors.material_weight && <p className="ui-text-danger text-xs mt-1">{errors.material_weight.message}</p>}
          </div>
          <div>
            <label className="block text-sm font-medium ui-text-secondary mb-1">{t('models.printTimeMinutes')}</label>
            <input type="number" step="1" {...register('print_time')} className="w-full border ui-border-default rounded-md px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ui-focus-ring" placeholder="e.g. 120" />
            {errors.print_time && <p className="ui-text-danger text-xs mt-1">{errors.print_time.message}</p>}
          </div>
        </div>
        <div>
          <label className="block text-sm font-medium ui-text-secondary mb-1">{t('models.wasteMaterial')}</label>
          <input type="number" step="0.01" {...register('waste_material')} className="w-full border ui-border-default rounded-md px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ui-focus-ring" placeholder="e.g. 2.0" />
          {errors.waste_material && <p className="ui-text-danger text-xs mt-1">{errors.waste_material.message}</p>}
        </div>
        <div className="flex items-center gap-2">
          <input type="checkbox" id="is_multicolor" {...register('is_multicolor')} className="h-4 w-4 ui-text-primary-action rounded ui-border-default focus:ui-focus-ring" />
          <label htmlFor="is_multicolor" className="text-sm font-medium ui-text-secondary">{t('models.multicolorPrint')}</label>
        </div>
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
