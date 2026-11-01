import api from './api';
import {
  createSalesChannel,
  createSalesOrder,
  deleteSalesOrder,
  getCompatiblePrintJobs,
  getPrintJobAllocations,
  getSalesChannels,
  getSalesOrder,
  getSalesOrderAllocations,
  getSalesOrders,
  getSalesSummary,
  replaceSalesOrderAllocations,
  updateSalesOrder,
  updateSalesOrderStatus,
} from '../modules/sales/services/salesService';

vi.mock('./api', () => ({ default: { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() } }));

beforeEach(() => {
  vi.clearAllMocks();
  api.get.mockResolvedValue({ data: { result: true } });
  api.post.mockResolvedValue({ data: { result: true } });
  api.put.mockResolvedValue({ data: { result: true } });
  api.delete.mockResolvedValue({});
});

it('wraps Sales Order reads and mutations', async () => {
  await getSalesSummary();
  await getSalesOrders({ page: 2 });
  await getSalesOrder('order-1');
  await createSalesOrder({ reference: 'A' });
  await updateSalesOrder('order-1', { reference: 'B' });
  await updateSalesOrderStatus('order-1', { fulfilment_status: 'confirmed' });
  await deleteSalesOrder('order-1');

  expect(api.get).toHaveBeenCalledWith('/sales/summary');
  expect(api.get).toHaveBeenCalledWith('/sales-orders', { params: { page: 2 } });
  expect(api.get).toHaveBeenCalledWith('/sales-orders/order-1');
  expect(api.post).toHaveBeenCalledWith('/sales-orders', { reference: 'A' });
  expect(api.put).toHaveBeenCalledWith('/sales-orders/order-1', { reference: 'B' });
  expect(api.put).toHaveBeenCalledWith('/sales-orders/order-1/status', { fulfilment_status: 'confirmed' });
  expect(api.delete).toHaveBeenCalledWith('/sales-orders/order-1');
});

it('wraps channel and allocation contracts without duplicating request logic', async () => {
  await getSalesChannels('all');
  await createSalesChannel({ name: 'Direct', notes: '' });
  await getSalesOrderAllocations('order-1');
  await replaceSalesOrderAllocations('order-1', [{ sales_order_item_id: 'item-1', print_job_id: 'job-1', units_allocated: 2 }]);
  await getCompatiblePrintJobs('model-1', { page: 2 });
  await getPrintJobAllocations('job-1');

  expect(api.get).toHaveBeenCalledWith('/sales-channels', { params: { state: 'all' } });
  expect(api.post).toHaveBeenCalledWith('/sales-channels', { name: 'Direct', notes: '' });
  expect(api.get).toHaveBeenCalledWith('/sales-orders/order-1/allocations');
  expect(api.put).toHaveBeenCalledWith('/sales-orders/order-1/allocations', { allocations: [{ sales_order_item_id: 'item-1', print_job_id: 'job-1', units_allocated: 2 }] });
  expect(api.get).toHaveBeenCalledWith('/print-jobs', { params: { page: 2, pageSize: 100, sort: 'date', direction: 'desc', model_id: 'model-1' } });
  expect(api.get).toHaveBeenCalledWith('/print-jobs/job-1/allocations');
});
