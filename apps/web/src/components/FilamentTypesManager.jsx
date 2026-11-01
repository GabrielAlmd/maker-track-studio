import { useFieldArray, useForm } from 'react-hook-form';
import { yupResolver } from '@hookform/resolvers/yup';
import * as yup from 'yup';
import toast from 'react-hot-toast';
import Modal from './ui/Modal';
import { updateFilamentTypes } from '../services/filamentTypeService';
import { useTranslation } from 'react-i18next';

const typeSchema = (t) => yup.object({
  types: yup.array().of(yup.object({ name: yup.string().trim().required(t('validation.typeNameRequired')).max(200) }))
    .max(200).test('unique', t('validation.typeNamesUnique'), (items) => {
      const names = items.map(({ name }) => name.toLowerCase());
      return new Set(names).size === names.length;
    }),
});

export default function FilamentTypesManager({ types, onClose, onSaved }) {
  const { t } = useTranslation();
  const { register, control, handleSubmit, formState: { errors, isSubmitting } } = useForm({
    resolver: yupResolver(typeSchema(t)), defaultValues: { types: types.map((name) => ({ name })) },
  });
  const { fields, append, remove } = useFieldArray({ control, name: 'types' });
  const close = () => { if (!isSubmitting) onClose(); };
  const submit = async (data) => {
    try {
      await updateFilamentTypes(data.types.map(({ name }) => name));
      toast.success(t('filamentTypes.saved'));
      onSaved();
    } catch {
      // API errors are shown by the interceptor; retain the draft for retry.
    }
  };
  return (
    <Modal title={t('filamentTypes.manage')} onClose={close}>
      <form onSubmit={handleSubmit(submit)} className="space-y-4">
        <p className="text-sm ui-text-secondary">{t('filamentTypes.help')}</p>
        <fieldset disabled={isSubmitting} className="space-y-3">
          {fields.map((field, index) => (
            <div key={field.id}>
              <div className="flex gap-2">
                <input aria-label={`${t('filaments.type')} ${index + 1}`} {...register(`types.${index}.name`)} maxLength={200} className="w-full border rounded-md px-3 py-2 text-sm" />
                <button type="button" onClick={() => remove(index)} aria-label={`${t('common.remove')} ${t('filaments.type').toLowerCase()} ${index + 1}`} className="text-sm ui-text-danger">{t('common.remove')}</button>
              </div>
              {errors.types?.[index]?.name && <p role="alert" className="text-xs ui-text-danger">{errors.types[index].name.message}</p>}
            </div>
          ))}
          {!fields.length && <p className="text-sm ui-text-muted">{t('filamentTypes.empty')}</p>}
          {(errors.types?.message || errors.types?.root?.message) && <p role="alert" className="text-sm ui-text-danger">{errors.types.message || errors.types.root.message}</p>}
          <button type="button" onClick={() => append({ name: '' })} disabled={fields.length >= 200} className="text-sm ui-text-primary-action">{t('filamentTypes.add')}</button>
        </fieldset>
        <div className="flex justify-end gap-3">
          <button type="button" onClick={close} disabled={isSubmitting} className="px-4 py-2 text-sm border rounded-md">{t('common.cancel')}</button>
          <button type="submit" disabled={isSubmitting} className="px-4 py-2 text-sm ui-bg-primary ui-text-inverse rounded-md disabled:opacity-50">{isSubmitting ? t('common.saving') : t('filamentTypes.save')}</button>
        </div>
      </form>
    </Modal>
  );
}
