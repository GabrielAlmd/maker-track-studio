import { useForm } from 'react-hook-form';
import { yupResolver } from '@hookform/resolvers/yup';
import * as yup from 'yup';
import { useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { textField } from '../utils/businessValidation';
import { Field } from './BusinessFields';
import useBusinessCommand from './useBusinessCommand';

const schema = yup.object({ purchase_order: textField(160) });

export default function ProposalConfirmation({ document }) {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const command = useBusinessCommand();
  const { register, handleSubmit, formState: { errors } } = useForm({ resolver: yupResolver(schema), defaultValues: { purchase_order: '' } });
  const expired = document.terms.valid_until < new Date().toISOString().slice(0, 10);
  if (expired) return <p className="text-sm ui-text-muted">{t('business.expired')}</p>;

  return <form className="page-panel p-5 sm:p-6" noValidate onSubmit={handleSubmit(async (values) => {
    const order = await command.run('orders', {
      ...values, expected_revision: 0, proposal_id: document.id, proposal_revision: document.revision,
    });
    if (order) navigate(`/business/orders/${order.id}`);
  })}>
    <fieldset disabled={command.busy} className="space-y-3">
      <legend className="mb-3 text-lg font-semibold">{t('business.confirm')}</legend>
      <Field label="po" maxLength={160} {...register('purchase_order')} error={errors.purchase_order} />
      {command.error && <p className="page-form-error" role="alert">{command.error}</p>}
      <button className="page-action" type="submit">{t('business.confirm')}</button>
    </fieldset>
  </form>;
}
