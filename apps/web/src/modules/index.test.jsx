import { dashboardContributions, featureSettingContributions, navigationContributions, printJobCardContributions, routeContributions } from './index';
import { translationsFor } from './translations';

const t = (key) => key;

it('composes Sales routes and settings while controlling visible contributions by feature state', () => {
  expect(routeContributions().map(({ path }) => path)).toEqual(['sales', 'sales/new', 'sales/:id']);
  expect(featureSettingContributions()).toHaveLength(1);
  expect(navigationContributions({ sales_orders: false }, t)).toEqual([]);
  expect(dashboardContributions({ sales_orders: false })).toEqual([]);
  expect(printJobCardContributions({ sales_orders: false })).toEqual([]);
  expect(navigationContributions({ sales_orders: true }, t)[0].href).toBe('/sales');
  expect(dashboardContributions({ sales_orders: true })).toHaveLength(1);
  expect(printJobCardContributions({ sales_orders: true })).toHaveLength(1);
});

it('supports a production-only frontend composition', () => {
  expect(routeContributions([])).toEqual([]);
  expect(featureSettingContributions([])).toEqual([]);
  expect(navigationContributions({ sales_orders: true }, t, [])).toEqual([]);
  expect(dashboardContributions({ sales_orders: true }, [])).toEqual([]);
  expect(printJobCardContributions({ sales_orders: true }, [])).toEqual([]);
  expect(translationsFor('en', { nav: { dashboard: 'Dashboard' } }, [])).toEqual({ nav: { dashboard: 'Dashboard' } });
  expect(translationsFor('en', { nav: { dashboard: 'Dashboard' } }).sales.title).toBe('Sales & Orders');
});
