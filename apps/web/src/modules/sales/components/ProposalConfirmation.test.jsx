import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import ProposalConfirmation from './ProposalConfirmation';
import { saveRecord } from '../services/businessService';

vi.mock('../services/businessService', () => ({ saveRecord: vi.fn() }));

const document = { id: 'proposal-id', revision: 3, terms: { valid_until: '2099-01-01' } };

function renderConfirmation(record = document) {
  render(<MemoryRouter initialEntries={['/proposal']}>
    <Routes>
      <Route path="/proposal" element={<ProposalConfirmation document={record} />} />
      <Route path="/business/orders/:id" element={<h1>Confirmed order</h1>} />
    </Routes>
  </MemoryRouter>);
}

beforeEach(() => vi.resetAllMocks());

it('retries an uncertain confirmation with the same operation ID and navigates only after success', async () => {
  saveRecord.mockRejectedValueOnce(new Error('Connection closed')).mockResolvedValueOnce({ id: 'order-id' });
  renderConfirmation();
  const user = userEvent.setup();
  await user.type(screen.getByLabelText('Customer PO'), 'PO-123');
  const submit = () => user.click(screen.getByRole('button', { name: 'Confirm customer acceptance' }));
  await submit();
  expect(await screen.findByRole('alert')).toHaveTextContent('Could not save');
  expect(screen.getByLabelText('Customer PO')).toHaveValue('PO-123');
  expect(screen.queryByRole('heading', { name: 'Confirmed order' })).not.toBeInTheDocument();

  await submit();
  expect(await screen.findByRole('heading', { name: 'Confirmed order' })).toBeInTheDocument();
  const [path, payload] = saveRecord.mock.calls[0];
  expect(path).toBe('orders');
  expect(payload).toEqual({
    purchase_order: 'PO-123', proposal_id: 'proposal-id', proposal_revision: 3, expected_revision: 0,
    operation_id: expect.stringMatching(/^[a-f0-9-]{36}$/),
  });
  expect(saveRecord.mock.calls[1]).toEqual(saveRecord.mock.calls[0]);
});

it('keeps a conflicting acceptance on the document and shows a localized error', async () => {
  saveRecord.mockRejectedValue({ response: { status: 409, data: { error: { message: 'internal details' } } } });
  renderConfirmation();
  await userEvent.click(screen.getByRole('button', { name: 'Confirm customer acceptance' }));
  expect(await screen.findByRole('alert')).toHaveTextContent('this action is no longer available');
  expect(screen.queryByText('internal details')).not.toBeInTheDocument();
  expect(screen.queryByRole('heading', { name: 'Confirmed order' })).not.toBeInTheDocument();
});

it('does not offer confirmation for an expired proposal', async () => {
  renderConfirmation({ ...document, terms: { valid_until: '2000-01-01' } });
  expect(screen.getByText('This proposal has expired.')).toBeVisible();
  await waitFor(() => expect(saveRecord).not.toHaveBeenCalled());
});
