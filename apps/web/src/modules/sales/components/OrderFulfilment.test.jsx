import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import OrderFulfilment from './OrderFulfilment';
import { getRecord, saveRecord } from '../services/businessService';

vi.mock('../services/businessService', () => ({ getRecord: vi.fn(), saveRecord: vi.fn() }));

it('ships against the selected hold and reuses the operation ID after an uncertain response', async () => {
  getRecord.mockResolvedValue({
    status: 'unfulfilled', can_cancel: true, lines: [{
      allocation_id: 'allocation', name: 'Test model', run_item_id: 'run-item', revision: 4,
      reservable: 0, reserved: 2, shipped: 0, returned: 0,
      events: [{ kind: 'reserve', operation_id: 'hold', units: 2, remaining: 2, reason: 'Sample' }],
    }],
  });
  saveRecord.mockRejectedValueOnce(new Error('connection closed')).mockResolvedValueOnce({ sequence: 5 });
  render(<OrderFulfilment id="order" />);
  const user = userEvent.setup();
  await user.type(await screen.findByLabelText('Reason or shipment reference'), 'Shipment 101');
  const submit = () => user.click(screen.getByRole('button', { name: 'Record event' }));
  await submit();
  expect(await screen.findByRole('alert')).toHaveTextContent('Could not save');
  await submit();
  await waitFor(() => expect(getRecord).toHaveBeenCalledTimes(2));
  expect(saveRecord.mock.calls[0]).toEqual(['orders/order/fulfilment', {
    operation_id: expect.any(String), units: 1, reason: 'Shipment 101',
    allocation_id: 'allocation', reference: 'hold', expected_revision: 4, kind: 'dispatch',
  }, undefined]);
  expect(saveRecord.mock.calls[1]).toEqual(saveRecord.mock.calls[0]);
});
