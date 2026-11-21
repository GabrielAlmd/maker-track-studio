import { Link } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import PageHeader from '../../../components/ui/PageHeader';
import BusinessDashboard from '../BusinessDashboard';
import BusinessNavigation from '../components/BusinessNavigation';

export default function Overview() {
  const { t } = useTranslation();
  return <>
    <PageHeader title={t('nav.sales')} description={t('dashboard.businessHelp')}>
      <Link to="/business/orders/new" className="page-action">{t('sales.newOrder')}</Link>
    </PageHeader>
    <BusinessNavigation />
    <BusinessDashboard />
  </>;
}
