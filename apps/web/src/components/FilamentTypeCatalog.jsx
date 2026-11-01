import { useTranslation } from 'react-i18next';

export default function FilamentTypeCatalog({ loading, error, onRetry, onManage }) {
  const { t } = useTranslation();
  if (loading) return <p className="text-sm ui-text-muted">{t('filamentTypes.loading')}</p>;
  if (error) return <div role="alert" className="text-sm ui-text-danger">{t('filamentTypes.loadError')} <button type="button" onClick={onRetry}>{t('common.retry')}</button></div>;
  return <button type="button" onClick={onManage} className="text-sm ui-text-primary-action">{t('filamentTypes.manageShort')}</button>;
}
