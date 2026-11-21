import { useForm } from 'react-hook-form';
import { yupResolver } from '@hookform/resolvers/yup';
import * as yup from 'yup';
import { useTranslation } from 'react-i18next';
import { textField } from '../utils/businessValidation';
import { Field } from './BusinessFields';
import useBusinessCommand from './useBusinessCommand';

const schema = yup.object({
  reason: textField(500, true),
  purchase_order: textField(160),
  delivery_terms: textField(2000),
});

export default function OrderAmendment({ revision, onSaved }) {
  const { t } = useTranslation();
  const command = useBusinessCommand();
  const { register, handleSubmit, formState: { errors } } = useForm({
    resolver: yupResolver(schema),
    defaultValues: { reason: '', purchase_order: revision.purchase_order, delivery_terms: revision.delivery_terms },
  });

  return <details className="page-panel p-5 sm:p-6">
    <summary className="cursor-pointer font-semibold">{t('business.amend')}</summary>
    <form noValidate className="mt-4 space-y-4" onSubmit={handleSubmit(async (values) => {
      const saved = await command.run(`orders/${revision.id}/amendments`, { ...values, expected_revision: revision.revision });
      if (saved) await onSaved();
    })}>
      <fieldset disabled={command.busy} className="space-y-4">
        <Field label="po" maxLength={160} {...register('purchase_order')} error={errors.purchase_order} />
        <Field label="deliveryTerms" multiline maxLength={2000} {...register('delivery_terms')} error={errors.delivery_terms} />
        <Field label="reason" required maxLength={500} {...register('reason')} error={errors.reason} />
        {command.error && <p className="page-form-error" role="alert">{command.error}</p>}
        <button className="page-action" type="submit">{t('common.saveChanges')}</button>
      </fieldset>
    </form>
  </details>;
}
