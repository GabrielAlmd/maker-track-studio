import { useState } from 'react';
import { useForm } from 'react-hook-form';
import { yupResolver } from '@hookform/resolvers/yup';
import * as yup from 'yup';
import { Loader2 } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import Modal from '../../../components/ui/Modal';
import { createSalesChannel } from '../services/salesService';

const channelSchema = (t) => yup.object({
  name: yup.string().trim().required(t('sales.validation.channelNameRequired')).max(100, t('sales.validation.channelNameLength')),
  notes: yup.string().trim().max(2000, t('sales.validation.notesLength')),
});

export default function SalesChannelModal({ onClose, onSaved }) {
  const { t } = useTranslation();
  const [submitError, setSubmitError] = useState(null);
  const { register, handleSubmit, formState: { errors, isSubmitting } } = useForm({
    resolver: yupResolver(channelSchema(t)),
    defaultValues: { name: '', notes: '' },
  });

  const onSubmit = async (data) => {
    setSubmitError(null);
    try {
      const channel = await createSalesChannel({ name: data.name.trim(), notes: data.notes.trim() });
      onSaved(channel);
    } catch (error) {
      const message = error?.response ? null : error?.message;
      setSubmitError(typeof message === 'string' && message ? message : t('sales.channelSaveError'));
    }
  };

  return (
    <Modal title={t('sales.addChannel')} onClose={onClose}>
      <form onSubmit={handleSubmit(onSubmit)} className="space-y-4">
        {submitError && <p className="page-form-error" role="alert">{submitError}</p>}
        <label className="block text-sm ui-text-secondary">
          {t('sales.channelName')}
          <input autoFocus maxLength="100" {...register('name')} aria-invalid={Boolean(errors.name)} aria-describedby={errors.name ? 'sales-channel-name-error' : undefined} className="mt-1 w-full rounded-md border ui-border-default px-3 py-2" />
          {errors.name && <p id="sales-channel-name-error" className="mt-1 text-xs ui-text-danger" role="alert">{errors.name.message}</p>}
        </label>
        <label className="block text-sm ui-text-secondary">
          {t('sales.channelNotes')}
          <textarea rows="3" maxLength="2000" {...register('notes')} aria-invalid={Boolean(errors.notes)} aria-describedby={errors.notes ? 'sales-channel-notes-error' : undefined} className="mt-1 w-full rounded-md border ui-border-default px-3 py-2" />
          {errors.notes && <p id="sales-channel-notes-error" className="mt-1 text-xs ui-text-danger" role="alert">{errors.notes.message}</p>}
        </label>
        <div className="flex justify-end gap-3">
          <button type="button" onClick={onClose} className="page-action page-action-secondary">{t('common.cancel')}</button>
          <button type="submit" disabled={isSubmitting} aria-busy={isSubmitting} className="page-action">
            {isSubmitting && <Loader2 className="h-4 w-4 animate-spin" aria-hidden="true" />}
            {t(isSubmitting ? 'common.saving' : 'sales.createChannel')}
          </button>
        </div>
      </form>
    </Modal>
  );
}
