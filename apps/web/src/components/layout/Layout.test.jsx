import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { afterEach } from 'vitest';
import Layout from './Layout';
import { FeatureContext } from '../../contexts/featureState';

const navigationItems = ['Dashboard', 'Print Jobs', 'Printers', 'Models', 'Filaments', 'Purchases', 'Sales & Orders', 'Settings'];

function renderLayout(initialEntry = '/dashboard', salesOrders = true) {
  const features = { features: { sales_orders: salesOrders }, loading: false };
  return render(<FeatureContext.Provider value={features}><MemoryRouter initialEntries={[initialEntry]}><Layout /></MemoryRouter></FeatureContext.Provider>);
}

describe('compact sidebar', () => {
  beforeEach(() => {
    delete window.go;
    document.documentElement.dataset.theme = 'dark';
  });

  afterEach(() => {
    delete window.go;
    delete document.documentElement.dataset.theme;
  });

  it('keeps every primary destination labelled', () => {
    renderLayout('/settings');

    expect(screen.getByRole('navigation', { name: 'Primary navigation' })).toBeInTheDocument();
    navigationItems.forEach((name) => expect(screen.getByRole('link', { name })).toBeInTheDocument());
  });

  it('hides Sales & Orders when the feature is disabled', () => {
    renderLayout('/dashboard', false);
    expect(screen.queryByRole('link', { name: 'Sales & Orders' })).not.toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Print Jobs' })).toBeInTheDocument();
  });

  it.each([
    ['/dashboard', 'Dashboard'],
    ['/print-jobs', 'Print Jobs'],
    ['/printers', 'Printers'],
    ['/models', 'Models'],
    ['/filaments', 'Filaments'],
    ['/purchases', 'Purchases'],
    ['/sales', 'Sales & Orders'],
    ['/settings', 'Settings'],
  ])('makes %s visibly active for %s', (path, name) => {
    renderLayout(path);

    const activeLink = screen.getByRole('link', { name });
    expect(activeLink).toHaveAttribute('aria-current', 'page');
    expect(document.querySelector('.app-nav-active')).toBe(activeLink);
  });

  it.each(['dark', 'light'])('keeps primary navigation accessible in %s mode', async (theme) => {
    document.documentElement.dataset.theme = theme;
    const user = userEvent.setup();
    renderLayout();

    await user.tab();
    expect(screen.getByRole('link', { name: 'Dashboard' })).toHaveFocus();
    expect(screen.getByRole('link', { name: 'Dashboard' })).toHaveClass('app-nav-item');
    expect(document.querySelector('.app-sidebar')).toHaveClass('app-sidebar-compact');
  });

  it('keeps a long application version available without changing navigation labels', async () => {
    window.go = { main: { App: { GetVersion: vi.fn().mockResolvedValue('2026.09.18-enterprise-preview-build') } } };
    renderLayout();

    await waitFor(() => expect(screen.getByText('v2026.09.18-enterprise-preview-build')).toBeInTheDocument());
    expect(screen.getByText('Track Every Layer')).toBeInTheDocument();
    expect(screen.getByAltText('Maker Track Studio')).toHaveClass('app-sidebar-logo');
  });
});
