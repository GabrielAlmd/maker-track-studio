import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import OrderFinance from './OrderFinance';
import { getRecord, saveRecord } from '../services/businessService';

vi.mock('../services/businessService', () => ({ getRecord: vi.fn(), saveRecord: vi.fn() }));

it('links a partial refund to its receipt and submits the exact cent amount', async () => {
  getRecord.mockResolvedValue({
    invoice_status: 'none', payment_status: 'paid', invoiced_cents: 0, estimated_profit_cents: 30,
    net_paid_cents: 1200, balance_cents: 0, entries: [
      { revision: 1, kind: 'payment', date: '2026-01-02', amount_cents: 1200, reference: 'Deposit' },
    ],
  });
  saveRecord.mockResolvedValue({ revision: 2 });
  const saved = vi.fn();
  render(<OrderFinance orderId="sample" onSaved={saved} />);
  const user = userEvent.setup();
  await user.selectOptions(await screen.findByLabelText('Entry type'), 'refund');
  await user.selectOptions(screen.getByLabelText('Original entry'), '1');
  await user.type(screen.getByLabelText('Amount (EUR)'), '1.01');
  await user.type(screen.getByLabelText('Reference or reason'), 'Correction');
  await user.click(screen.getByRole('button', { name: 'Record entry' }));
  await waitFor(() => expect(saved).toHaveBeenCalledOnce());
  expect(saveRecord).toHaveBeenCalledWith('orders/sample/finance', expect.objectContaining({
    amount_cents: 101, reference: 'Correction', kind: 'refund', original_revision: 1, expected_revision: 1, operation_id: expect.any(String),
  }), undefined);
});
