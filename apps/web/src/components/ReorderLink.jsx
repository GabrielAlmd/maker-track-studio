import { useTranslation } from 'react-i18next';
import { safePurchaseURL } from '../utils/purchaseDetails';

export default function ReorderLink({ url }) {
  const { t } = useTranslation();
  const href = safePurchaseURL(url);
  if (!href) return null;

  const open = (event) => {
    if (typeof window.runtime?.BrowserOpenURL === 'function') {
      event.preventDefault();
      window.runtime.BrowserOpenURL(href);
    }
  };

  return <a href={href} onClick={open} target="_blank" rel="noopener noreferrer"
    className="ui-text-primary-action underline">{t('purchaseDetails.reorder')}</a>;
}
