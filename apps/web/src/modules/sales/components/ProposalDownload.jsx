import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Download } from 'lucide-react';
import toast from 'react-hot-toast';
import { downloadProposalPDF } from '../services/businessService';

export default function ProposalDownload({ id, revision }) {
  const { t, i18n } = useTranslation();
  const [busy, setBusy] = useState(false);

  async function download() {
    setBusy(true);
    try {
      await downloadProposalPDF(id, revision, i18n.resolvedLanguage === 'pt-PT' ? 'pt-PT' : 'en');
    } catch (error) {
      if (!error?.isAxiosError) toast.error(t('business.downloadError'));
    } finally {
      setBusy(false);
    }
  }

  return <button type="button" className="page-action page-action-secondary" disabled={busy} onClick={download}>
    <Download size={16} aria-hidden="true" />{t('business.downloadProposal')}
  </button>;
}
