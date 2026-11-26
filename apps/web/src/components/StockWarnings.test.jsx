import { render, screen } from '@testing-library/react';
import { afterEach, expect, it } from 'vitest';
import { StockWarningList } from './StockWarnings';
import i18n from '../i18n';

afterEach(() => i18n.changeLanguage('en'));

it('localizes availability and keeps insufficient-stock warnings nonblocking', async () => {
  await i18n.changeLanguage('pt-PT');
  render(<StockWarningList requirements={[{ filament_id: 'material', weight_used: 13 }]} inventory={{
    rows: [{ id: 'material', brand: 'Synthetic', type: 'PLA', color: 'Copper',
      threshold_mg: 1000, balance: { verified: true, available: 12500 } }],
    failed: false,
  }} />);
  expect(screen.getByRole('region', { name: 'Avisos de stock' })).toBeVisible();
  expect(screen.getByText('Material insuficiente')).toBeVisible();
  expect(screen.getByText('12,5 g disponíveis')).toBeVisible();
  expect(screen.getByText(/Os avisos não impedem o planeamento/)).toBeVisible();
});
