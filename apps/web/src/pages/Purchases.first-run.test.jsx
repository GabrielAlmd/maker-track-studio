import { act, render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import Purchases from './Purchases';
import DiagnosticErrorBoundary from '../components/DiagnosticErrorBoundary';
import { apiPage, deferred } from '../test/apiFixtures';
import { getFilaments } from '../services/filamentService';
import { getFilamentPurchases } from '../services/filamentPurchaseService';
import { getFilamentTypes } from '../services/filamentTypeService';
import { getFilamentBrands } from '../services/filamentService';

vi.mock('../services/filamentService', () => ({ getFilaments: vi.fn(), getFilamentBrands: vi.fn() }));
vi.mock('../services/filamentPurchaseService', () => ({
  getFilamentPurchases: vi.fn(), createFilamentPurchase: vi.fn(),
  updateFilamentPurchase: vi.fn(), deleteFilamentPurchase: vi.fn(), createFilamentPurchaseBatch: vi.fn(),
}));
vi.mock('../services/filamentTypeService', () => ({ getFilamentTypes: vi.fn(), updateFilamentTypes: vi.fn() }));
vi.mock('../services/diagnostics', () => ({ reportFrontendFailure: vi.fn() }));

function renderPage() {
  // Match the application's boundary: a render failure is observable as a
  // recovery screen, not as an unhandled test-runner exception.
  return render(<DiagnosticErrorBoundary><Purchases /></DiagnosticErrorBoundary>, { onCaughtError: () => {} });
}

describe('Purchases first-run regression (A01/A02)', () => {
  beforeEach(() => {
    vi.resetAllMocks();
    getFilamentPurchases.mockResolvedValue(apiPage());
    getFilamentTypes.mockResolvedValue(['PLA']);
    getFilamentBrands.mockResolvedValue([]);
  });

  it.each([
    { name: 'an empty array', response: [] },
    { name: 'the actual null response from an empty database', response: null },
  ])('opens the first purchase with $name', async ({ response }) => {
    getFilaments.mockResolvedValue(response);
    renderPage();
    await userEvent.click(await screen.findByRole('button', { name: /record first purchase/i }));
    expect(screen.getByRole('dialog', { name: 'Record New Purchase' })).toBeVisible();
    expect(screen.getByRole('combobox', { name: 'Filament' })).toBeEnabled();
    expect(screen.queryByText(/Restart the application/i)).not.toBeInTheDocument();
  });

  it('can open the form while the filament lookup is pending and keep it after resolution', async () => {
    const lookup = deferred();
    getFilaments.mockReturnValue(lookup.promise);
    renderPage();
    await userEvent.click(await screen.findByRole('button', { name: /record first purchase/i }));
    expect(screen.getByRole('dialog')).toBeVisible();
    await act(async () => lookup.resolve([]));
    expect(screen.getByRole('dialog')).toBeVisible();
  });

  it('keeps the first-purchase form usable when the filament lookup fails', async () => {
    getFilaments.mockRejectedValue(new Error('Could not load filaments'));
    renderPage();
    await userEvent.click(await screen.findByRole('button', { name: /record first purchase/i }));
    expect(screen.getByRole('dialog')).toBeVisible();
    expect(screen.getByRole('button', { name: /create new filament/i })).toBeEnabled();
    expect(screen.queryByText(/Restart the application/i)).not.toBeInTheDocument();
  });
  it('retries lookup without losing the purchase draft', async () => {
    getFilaments.mockRejectedValueOnce(new Error('Lookup unavailable')).mockResolvedValueOnce([]);
    renderPage();
    await userEvent.click(await screen.findByRole('button', { name: /record first purchase/i }));
    const date = screen.getByLabelText('Purchase Date');
    await userEvent.type(date, '2026-09-18');
    await userEvent.click(screen.getByRole('button', { name: /try again/i }));
    expect(date).toHaveValue('2026-09-18');
    expect(screen.getByRole('dialog')).toBeVisible();
    expect(getFilaments).toHaveBeenCalledTimes(2);
  });

});
