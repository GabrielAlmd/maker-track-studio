import api from './api';
import { getFeatures, updateFeatures } from './featureService';

vi.mock('./api', () => ({ default: { get: vi.fn(), put: vi.fn() } }));

beforeEach(() => vi.clearAllMocks());

it('loads the dedicated feature contract', async () => {
  api.get.mockResolvedValue({ data: { sales_orders: false } });
  await expect(getFeatures()).resolves.toEqual({ sales_orders: false });
  expect(api.get).toHaveBeenCalledWith('/features');
});

it('persists the complete feature contract', async () => {
  api.put.mockResolvedValue({ data: { sales_orders: true } });
  await expect(updateFeatures({ sales_orders: true })).resolves.toEqual({ sales_orders: true });
  expect(api.put).toHaveBeenCalledWith('/features', { sales_orders: true });
});
