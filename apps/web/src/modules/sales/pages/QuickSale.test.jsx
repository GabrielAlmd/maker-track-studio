import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import QuickSale from './QuickSale';
import { getModel, getModels } from '../../../services/modelService';
import { getFilaments } from '../../../services/filamentService';
import { getMaterials } from '../../../services/inventoryService';
import { getPrinters } from '../../../services/printerService';
import { allCustomers, listQuickSaleStock, saveRecord } from '../services/businessService';

vi.mock('../../../services/modelService', () => ({ getModel: vi.fn(), getModels: vi.fn() }));
vi.mock('../../../services/filamentService', () => ({ getFilaments: vi.fn() }));
vi.mock('../../../services/inventoryService', () => ({ getMaterials: vi.fn() }));
vi.mock('../../../services/printerService', () => ({ getPrinters: vi.fn() }));
vi.mock('../services/businessService', () => ({ allCustomers: vi.fn(), listQuickSaleStock: vi.fn(), saveRecord: vi.fn() }));

const model = '11111111-1111-4111-8111-111111111111';
const lot = '22222222-2222-4222-8222-222222222222';
const material = '33333333-3333-4333-8333-333333333333';
const filament = '44444444-4444-4444-8444-444444444444';

beforeEach(() => {
  vi.resetAllMocks();
  allCustomers.mockResolvedValue([]);
  getModels.mockResolvedValue({ items: [{ id: model, name: 'Synthetic bracket' }], page_size: 25, total: 1 });
  getModel.mockResolvedValue({
    id: model, material_requirements: [{ id: material, material_type: 'PLA', color: 'Blue', part_weight_g: 10 }],
  });
  getFilaments.mockResolvedValue([{ id: filament, type: 'PLA', color: 'Blue' }]);
  getMaterials.mockResolvedValue([]);
  getPrinters.mockResolvedValue([]);
  listQuickSaleStock.mockResolvedValue({ items: [{ print_run_item_id: lot, print_date: '2026-01-02', units_available: 5 }] });
  saveRecord.mockImplementation(async (path, request) => path.endsWith('/estimate')
    ? { token: 'estimate-token', unit_cost_cents: 500, total_cost_cents: 500 * request.quantity,
      suggested_unit_price_cents: request.cost.target.mode === 'margin' ? 833 : 700 }
    : { id: 'confirmed' });
});

async function start() {
  const user = userEvent.setup();
  render(<MemoryRouter initialEntries={['/business/orders/quick-sale']}>
    <Routes>
      <Route path="/business/orders/quick-sale" element={<QuickSale />} />
      <Route path="/business/orders/:id" element={<h1>Confirmed order</h1>} />
    </Routes>
  </MemoryRouter>);
  await user.type(await screen.findByLabelText('Sale reference'), 'Counter sale');
  await user.click(await screen.findByRole('combobox', { name: 'Model', exact: true }));
  await user.click(await screen.findByRole('option', { name: 'Synthetic bracket' }));
  return user;
}

it('previews shared model assumptions, applies the target price, and confirms edited cents', async () => {
  const user = await start();
  await user.click(screen.getByRole('checkbox', { name: 'Estimate cost and price (optional)' }));
  await user.type(screen.getByLabelText('Print minutes per unit'), '15');
  await user.type(screen.getByLabelText('Target (%)'), '100');
  const preview = await screen.findByRole('button', { name: 'Calculate estimate' });
  await user.click(preview);
  expect(await screen.findByLabelText('Target (%)')).toHaveAttribute('aria-invalid', 'true');
  expect(saveRecord).not.toHaveBeenCalled();
  await user.clear(screen.getByLabelText('Target (%)'));
  await user.type(screen.getByLabelText('Target (%)'), '40,00');
  await user.click(preview);
  const apply = await screen.findByRole('button', { name: 'Apply suggested price' });
  expect(saveRecord).toHaveBeenCalledWith('quick-sales/estimate', expect.objectContaining({
    quantity: 1,
    cost: expect.objectContaining({
      source: 'model', minutes_per_unit: 15,
      filament_mapping: { [material]: filament },
      target: { mode: 'margin', value: 4000 },
    }),
  }));
  await user.click(apply);
  const price = screen.getByLabelText('Unit price (EUR)');
  expect(price).toHaveValue('8.33');
  await user.clear(price);
  await user.type(price, '9,10');
  expect(screen.getByText(/Estimated profit at final price €4.10/)).toBeVisible();
  await user.click(screen.getByRole('button', { name: 'Confirm sale' }));
  expect(await screen.findByRole('heading', { name: 'Confirmed order' })).toBeVisible();
  expect(saveRecord).toHaveBeenLastCalledWith('quick-sales', expect.objectContaining({
    unit_price_cents: 910, estimate_token: 'estimate-token', cost: expect.objectContaining({ source: 'model' }),
  }), undefined);
});

it('uses a manual fallback for stock and requires a new lot and estimate after a quantity change', async () => {
  const user = await start();
  await user.selectOptions(screen.getByLabelText('Fulfilment'), 'stock');
  async function chooseLot() {
    await user.click(await screen.findByRole('combobox', { name: 'Finished stock lot' }));
    await user.click(await screen.findByRole('option', { name: /5 available/ }));
  }
  await chooseLot();
  await user.click(screen.getByRole('checkbox', { name: 'Estimate cost and price (optional)' }));
  expect(screen.getByText(/Historical cost is unavailable/)).toBeVisible();
  await user.type(screen.getByLabelText('Cost per unit (EUR)'), '5.00');
  await user.selectOptions(screen.getByLabelText('Pricing method'), 'markup');
  await user.type(screen.getByLabelText('Target (%)'), '40');
  await user.click(screen.getByRole('button', { name: 'Calculate estimate' }));
  await user.click(await screen.findByRole('button', { name: 'Apply suggested price' }));
  const confirm = screen.getByRole('button', { name: 'Confirm and dispatch' });
  expect(confirm).toBeEnabled();
  const quantity = screen.getByLabelText('Quantity', { exact: true });
  await user.clear(quantity);
  await user.type(quantity, '2');
  await chooseLot();
  expect(confirm).toBeDisabled();
  await user.click(screen.getByRole('checkbox', { name: 'Estimate cost and price (optional)' }));
  await user.click(confirm);
  await waitFor(() => expect(saveRecord).toHaveBeenLastCalledWith('quick-sales', expect.objectContaining({
    quantity: 2, stock_run_item_id: lot, unit_price_cents: 700, cost: undefined, estimate_token: undefined,
  }), undefined));
});
