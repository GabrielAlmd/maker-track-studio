import { useCallback, useId, useRef, useState } from 'react';
import { useForm } from 'react-hook-form';
import { yupResolver } from '@hookform/resolvers/yup';
import * as yup from 'yup';
import { Download, Paperclip } from 'lucide-react';
import { Link } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import toast from 'react-hot-toast';
import { useApi } from '../../../hooks/useApi';
import { InlineError, LoadingState } from '../../../components/ui/PageState';
import { activeIntlLocale, formatDate } from '../../../utils/formatters';
import { downloadOrderAttachment, getRecord, uploadOrderAttachment } from '../services/businessService';
import operationID from '../utils/operationID';
import ProposalDownload from './ProposalDownload';

const schema = yup.object({
  file: yup.mixed().test('file', 'attachmentInvalid', (files) =>
    files?.length === 1 && files[0].size > 0 && files[0].size <= 5 * 1024 * 1024 && /\.(pdf|png|jpe?g)$/i.test(files[0].name)),
});

export default function OrderDocuments({ orderId, acceptance }) {
  const { t } = useTranslation();
  const locale = activeIntlLocale();
  const bytes = new Intl.NumberFormat(locale, { style: 'unit', unit: 'byte' });
  const kilobytes = new Intl.NumberFormat(locale, { style: 'unit', unit: 'kilobyte', maximumFractionDigits: 1 });
  const fileInput = useId();
  const retry = useRef(null);
  const [downloading, setDownloading] = useState(false);
  const fetchAttachments = useCallback(() => getRecord(`orders/${orderId}/attachments`), [orderId]);
  const { data, loading, error, refetch } = useApi(fetchAttachments);
  const { register, handleSubmit, reset, formState: { errors, isSubmitting } } = useForm({ resolver: yupResolver(schema) });

  async function attach({ file }) {
    const selected = file[0];
    if (retry.current?.file !== selected) retry.current = { file: selected, id: operationID() };
    try {
      await uploadOrderAttachment(orderId, selected, retry.current.id);
      retry.current = null;
      reset();
      toast.success(t('business.attachmentAdded'));
      await refetch();
    } catch (error) {
      if (!error?.isAxiosError) toast.error(t('business.saveError'));
    }
  }

  async function download(attachment) {
    setDownloading(true);
    try {
      await downloadOrderAttachment(orderId, attachment);
    } catch (error) {
      if (!error?.isAxiosError) toast.error(t('business.downloadError'));
    } finally {
      setDownloading(false);
    }
  }

  return <section className="page-panel p-5 sm:p-6">
    <h2 className="text-lg font-semibold">{t('business.documents')}</h2>
    {acceptance && <div className="my-4 flex flex-wrap gap-2">
      <Link className="page-action page-action-secondary" to={`/business/proposals/${acceptance.proposal_id}?document=${acceptance.proposal_revision}`}>{t('business.viewAcceptedDocument')}</Link>
      <ProposalDownload id={acceptance.proposal_id} revision={acceptance.proposal_revision} />
    </div>}
    <h3 className="mt-5 font-semibold">{t('business.attachments')}</h3>
    {loading ? <LoadingState /> : error ? <InlineError title={t('business.loadError')} onRetry={refetch} /> :
      data?.items.length ? <ul className="my-3 divide-y ui-divide-subtle">
        {data.items.map((item) => <li className="flex flex-wrap items-center justify-between gap-3 py-3" key={item.id}>
          <div className="min-w-0">
            <div className="break-words font-medium">{item.name}</div>
            <div className="ui-text-secondary text-sm">
              {formatDate(item.created_at)} · {item.size < 1000 ? bytes.format(item.size) : kilobytes.format(item.size / 1000)}
            </div>
          </div>
          <button type="button" className="page-action page-action-secondary" disabled={downloading} onClick={() => download(item)}
            aria-label={t('business.downloadNamedAttachment', { name: item.name })}>
            <Download size={16} aria-hidden="true" />{t('business.downloadAttachment')}
          </button>
        </li>)}
      </ul> : <p className="my-3 ui-text-secondary">{t('business.noAttachments')}</p>}
    <form className="mt-4 flex flex-wrap items-end gap-3" onSubmit={handleSubmit(attach)}>
      <div className="min-w-0">
        <label className="block text-sm font-medium" htmlFor={fileInput}>{t('business.attachmentFile')}</label>
        <input className="mt-2 block max-w-full text-sm" type="file" id={fileInput} accept=".pdf,.png,.jpg,.jpeg"
          aria-invalid={Boolean(errors.file)} aria-describedby={`${fileInput}-help`} disabled={isSubmitting} {...register('file')} />
        <p className="mt-2 text-sm ui-text-secondary" id={`${fileInput}-help`}>{t('business.attachmentHelp')}</p>
        {errors.file && <p role="alert" className="mt-1 text-sm ui-text-danger">{t('business.attachmentInvalid')}</p>}
      </div>
      <button type="submit" className="page-action" disabled={isSubmitting}>
        <Paperclip size={16} aria-hidden="true" />{t('business.addAttachment')}
      </button>
    </form>
  </section>;
}
