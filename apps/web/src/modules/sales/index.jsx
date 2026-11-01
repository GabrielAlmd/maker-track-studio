import { ReceiptText } from 'lucide-react';
import Sales from './pages/Sales';
import SalesOrder from './pages/SalesOrder';
import BusinessDashboard from './BusinessDashboard';
import SalesFeatureSetting from './SalesFeatureSetting';
import AllocationCapacity from './components/AllocationCapacity';

const salesModule = {
  id: 'sales_orders',
  enabled: (features) => features?.sales_orders === true,
  routes: [
    { path: 'sales', Component: Sales },
    { path: 'sales/new', Component: SalesOrder },
    { path: 'sales/:id', Component: SalesOrder },
  ],
  navigation: (t) => ({ name: t('nav.sales'), href: '/sales', icon: ReceiptText }),
  FeatureSetting: SalesFeatureSetting,
  Dashboard: BusinessDashboard,
  PrintJobCardDetail: AllocationCapacity,
};

export default salesModule;
