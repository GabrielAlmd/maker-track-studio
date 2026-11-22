import { lazy } from 'react';
import { ReceiptText } from 'lucide-react';
import { BusinessRoute, LegacySalesRoute, OrderRoute } from './pages/BusinessRoutes';
import Sales from './pages/Sales';
import Overview from './pages/Overview';
import BusinessDashboard from './BusinessDashboard';
import SalesFeatureSetting from './SalesFeatureSetting';
import AllocationCapacity from './components/AllocationCapacity';

const QuickSale = lazy(() => import('./pages/QuickSale'));
const CommercialRecords = lazy(() => import('./pages/CommercialRecords'));

const salesModule = {
  id: 'sales_orders',
  enabled: (features) => features?.sales_orders === true,
  routes: [
    ['business', Overview],
    ['business/orders', Sales],
    ['business/orders/new', OrderRoute],
    ['business/orders/quick-sale', QuickSale],
    ['business/orders/:id', OrderRoute],
    ['business/customers/:id?', () => <CommercialRecords kind="customers" />],
    ['business/budgets/:id?', () => <CommercialRecords kind="budgets" />],
    ['business/proposals/:id?', () => <CommercialRecords kind="proposals" />],
    ['sales', LegacySalesRoute],
    ['sales/new', () => <LegacySalesRoute create />],
    ['sales/:id', LegacySalesRoute],
  ].map(([path, view]) => {
    const Component = view;
    return { path, Component: () => <BusinessRoute><Component /></BusinessRoute> };
  }),
  navigation: (t) => ({ name: t('nav.sales'), href: '/business', icon: ReceiptText }),
  FeatureSetting: SalesFeatureSetting,
  Dashboard: BusinessDashboard,
  PrintJobCardDetail: AllocationCapacity,
};

export default salesModule;
