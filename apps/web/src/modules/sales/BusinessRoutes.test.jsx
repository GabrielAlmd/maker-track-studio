import { render, screen, within } from '@testing-library/react';
import { MemoryRouter, Route, Routes, useLocation } from 'react-router-dom';
import { FeatureContext } from '../../contexts/featureState';
import { routeContributions } from '..';
import { listRecords } from './services/businessService';

vi.mock('./services/businessService', () => ({ listRecords: vi.fn() }));
vi.mock('./pages/Sales', () => ({ default: () => <h1>Order list fixture</h1> }));
vi.mock('./pages/SalesOrder', () => ({ default: () => <h1>Order detail fixture</h1> }));

function Location() {
  const { pathname, search } = useLocation();
  return <output aria-label="Location">{pathname}{search}</output>;
}

function visit(path, enabled = true) {
  return render(<MemoryRouter initialEntries={[path]}>
    <FeatureContext.Provider value={{ features: { sales_orders: enabled } }}>
      <Location />
      <Routes>
        {routeContributions().map((contribution) => <Route key={contribution.path} path={contribution.path} element={<contribution.Component />} />)}
        <Route path="/dashboard" element={<a href="/inventory">Inventory</a>} />
      </Routes>
    </FeatureContext.Provider>
  </MemoryRouter>);
}

it.each([
  ['/sales', '/business/orders', 'Order list fixture'],
  ['/sales/new', '/business/orders/new', 'Order detail fixture'],
  ['/sales/legacy-id?section=financials', '/business/orders/legacy-id?section=financials', 'Order detail fixture'],
])('resolves the legacy bookmark %s', async (legacy, destination, heading) => {
  visit(legacy);
  expect(await screen.findByRole('heading', { name: heading })).toBeInTheDocument();
  expect(screen.getByLabelText('Location')).toHaveTextContent(destination);
});

it.each(['/business/proposals', '/sales/legacy-id'])('hides optional routes without hiding Inventory at %s', async (route) => {
  listRecords.mockClear();
  visit(route, false);
  expect(await screen.findByRole('link', { name: 'Inventory' })).toBeVisible();
  expect(screen.getByLabelText('Location')).toHaveTextContent('/dashboard');
  expect(listRecords).not.toHaveBeenCalled();
});

it('shows proposal navigation and a usable no-data state', async () => {
  listRecords.mockResolvedValue({ items: [], has_more: false });
  visit('/business/proposals');
  expect(await screen.findByRole('heading', { name: 'No records on this page' })).toBeVisible();
  const navigation = screen.getByRole('navigation', { name: 'Business Operations' });
  expect(within(navigation).getByRole('link', { name: 'Proposals' })).toHaveAttribute('aria-current', 'page');
  expect(screen.getByRole('link', { name: 'Choose a budget to quote' })).toHaveAttribute('href', '/business/proposals/new');
});
