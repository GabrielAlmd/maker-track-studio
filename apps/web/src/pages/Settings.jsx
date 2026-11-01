import { useCallback } from 'react';
import { useSearchParams } from 'react-router-dom';
import { Moon, Settings2, Sun } from 'lucide-react';
import PageHeader from '../components/ui/PageHeader';
import { useApi } from '../hooks/useApi';
import { getAppConfig } from '../services/appConfigService';
import CalculationSettings from './CalculationSettings';
import DataBackupsSection from './DataBackupsSection';
import DataMigrationSection from './DataMigrationSection';
import { useTheme } from '../contexts/ThemeContext';
import { InlineError, TableSkeleton } from '../components/ui/PageState';
import { useTranslation } from 'react-i18next';
import toast from 'react-hot-toast';
import { useLocale } from '../i18n';
import { useFeatures } from '../contexts/featureState';
import { featureSettingContributions } from '../modules';

function AppearanceSection() {
  const { theme, setTheme } = useTheme();
  const { t } = useTranslation();
  const locale = useLocale();
  const updateLanguage = async (event) => {
    if (await locale.setLocale(event.target.value)) toast.success(t('settings.languageSaved'));
  };
  return <div className="space-y-6"><section className="page-panel" aria-labelledby="appearance-title"><div className="page-panel-header"><h2 id="appearance-title" className="text-lg font-semibold ui-text-primary">{t('settings.appearance')}</h2><p className="mt-1 text-sm ui-text-muted">{t('settings.appearanceHelp')}</p></div><div className="grid gap-3 p-6 sm:grid-cols-2" role="radiogroup" aria-label={t('settings.theme')}><button type="button" role="radio" aria-checked={theme === 'dark'} data-selected={theme === 'dark'} className="theme-option text-left" onClick={() => setTheme('dark')}><span className="flex items-center gap-3 text-sm font-semibold ui-text-primary"><Moon className="h-5 w-5 ui-text-primary-action" />{t('settings.dark')}</span><span className="mt-2 block text-sm ui-text-muted">{t('settings.darkHelp')}</span></button><button type="button" role="radio" aria-checked={theme === 'light'} data-selected={theme === 'light'} className="theme-option text-left" onClick={() => setTheme('light')}><span className="flex items-center gap-3 text-sm font-semibold ui-text-primary"><Sun className="h-5 w-5 ui-text-primary-action" />{t('settings.light')}</span><span className="mt-2 block text-sm ui-text-muted">{t('settings.lightHelp')}</span></button></div></section>
    <section className="page-panel" aria-labelledby="language-title"><div className="page-panel-header"><h2 id="language-title" className="text-lg font-semibold ui-text-primary">{t('settings.language')}</h2><p className="mt-1 text-sm ui-text-muted">{t('locale.help')}</p></div><div className="p-6"><label htmlFor="application-language" className="mb-2 block text-sm font-medium ui-text-primary">{t('locale.label')}</label><select id="application-language" value={locale.locale} disabled={locale.saving} onChange={updateLanguage} className="w-full max-w-sm rounded-lg border ui-border-default ui-surface-card px-3 py-2 ui-text-primary"><option value="en">English</option><option value="pt-PT">Português (Portugal)</option></select>{locale.error && <p role="alert" className="mt-2 text-sm ui-text-danger">{t(locale.error)}</p>}</div></section></div>;
}

function CalculationsSection() {
  const { t } = useTranslation();
  const fetchConfig = useCallback(() => getAppConfig(), []);
  const { data, loading, error, refetch } = useApi(fetchConfig);
  if (loading && !data) return <div className="page-panel"><TableSkeleton columns={3} label={t('settings.loadingCalculations')} /></div>;
  if (error && !data) return <InlineError title={t('settings.loadCalculations')} message={error} onRetry={refetch} />;
  return <>{error && <div className="mb-6"><InlineError title={t('settings.refreshCalculations')} message={t('common.existingDataSafe')} onRetry={refetch} /></div>}<CalculationSettings configs={data ?? []} /></>;
}

function FeaturesSection() {
  const { t } = useTranslation();
  const { features, loading, error, refetch } = useFeatures();

  if (loading && !features) {
    return <div className="page-panel"><TableSkeleton columns={1} label={t('settings.loadingFeatures')} /></div>;
  }
  if (error && !features) {
    return <InlineError title={t('settings.featuresLoadError')} message={t('settings.featuresLoadError')} onRetry={refetch} />;
  }

  return <section className="page-panel" aria-labelledby="features-title">
    <div className="page-panel-header">
      <h2 id="features-title" className="text-lg font-semibold ui-text-primary">{t('settings.features')}</h2>
      <p className="mt-1 text-sm ui-text-muted">{t('settings.featuresHelp')}</p>
    </div>
    <div className="p-6">
      {error && <div className="mb-4"><InlineError title={t('settings.featuresSaveError')} message={t('common.existingDataSafe')} onRetry={refetch} /></div>}
      {featureSettingContributions().map((contribution) => <contribution.Component key={contribution.id} />)}
    </div>
  </section>;
}

export default function Settings() {
  const { t } = useTranslation();
  const sections = [['appearance', t('settings.appearance')], ['calculations', t('settings.calculations')], ['features', t('settings.features')], ['migration', t('settings.migration')], ['backups', t('settings.backups')]];
  const [searchParams, setSearchParams] = useSearchParams();
  const requested = searchParams.get('section');
  const section = sections.some(([key]) => key === requested) ? requested : 'appearance';
  const select = (key) => setSearchParams(key === 'appearance' ? {} : { section: key });
  return <div><PageHeader title={t('settings.title')} icon={Settings2} description={t('settings.description')} /><nav className="settings-tabs mb-6" aria-label={t('settings.sections')} role="tablist">{sections.map(([key, label]) => <button key={key} type="button" role="tab" aria-selected={section === key} className={section === key ? 'settings-tab settings-tab-active' : 'settings-tab'} onClick={() => select(key)}>{label}</button>)}</nav>{section === 'appearance' && <AppearanceSection />}{section === 'calculations' && <CalculationsSection />}{section === 'features' && <FeaturesSection />}{section === 'migration' && <DataMigrationSection />}{section === 'backups' && <DataBackupsSection />}</div>;
}
