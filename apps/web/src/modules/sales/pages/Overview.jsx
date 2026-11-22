import { Link } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import PageHeader from '../../../components/ui/PageHeader';
import BusinessDashboard from '../BusinessDashboard';
import BusinessNavigation from '../components/BusinessNavigation';

export default function Overview() {
  const { t } = useTranslation();
  return <>
    <PageHeader title={t('nav.sales')} description={t('dashboard.businessHelp')}>
      <div className="flex flex-wrap gap-2">
        <Link to="/business/orders/new" className="page-action page-action-secondary">{t('sales.newOrder')}</Link>
        <Link to="/business/orders/quick-sale" className="page-action">{t('business.quickSale')}</Link>
      </div>
    </PageHeader>
    <BusinessNavigation />
    <BusinessDashboard />
  </>;
}
