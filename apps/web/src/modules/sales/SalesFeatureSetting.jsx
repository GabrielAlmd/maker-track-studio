import { useTranslation } from 'react-i18next';
import toast from 'react-hot-toast';
import { useFeatures } from '../../contexts/featureState';

export default function SalesFeatureSetting() {
  const { t } = useTranslation();
  const { features, saving, updateFeature } = useFeatures();
  const enabled = features?.sales_orders ?? false;
  const toggle = async (event) => {
    try {
      await updateFeature('sales_orders', event.target.checked);
      toast.success(t('settings.featuresSaved'));
    } catch {
      toast.error(t('settings.featuresSaveError'));
    }
  };

  return <label className="flex max-w-2xl items-start justify-between gap-6 rounded-lg border ui-border-default ui-surface-card p-4">
    <span>
      <span className="block text-sm font-semibold ui-text-primary">{t('settings.salesFeature')}</span>
      <span className="mt-1 block text-sm ui-text-muted">{t('settings.salesFeatureHelp')}</span>
    </span>
    <span className="flex shrink-0 items-center gap-2 text-sm font-medium ui-text-primary">
      <input type="checkbox" checked={enabled} disabled={saving} onChange={toggle} />
      {t(enabled ? 'settings.enabled' : 'settings.disabled')}
    </span>
  </label>;
}
