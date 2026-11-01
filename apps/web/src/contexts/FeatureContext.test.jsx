import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { FeatureProvider } from './FeatureContext';
import { useFeatures } from './featureState';
import { getFeatures, updateFeatures } from '../services/featureService';

vi.mock('../services/featureService', () => ({ getFeatures: vi.fn(), updateFeatures: vi.fn() }));

function Probe() {
  const { features, loading, updateFeature } = useFeatures();
  return <div>
    <span>{loading ? 'loading' : features?.sales_orders ? 'enabled' : 'disabled'}</span>
    <button type="button" onClick={() => updateFeature('sales_orders', true)}>Enable</button>
  </div>;
}

beforeEach(() => vi.clearAllMocks());

it('loads once and shares the persisted feature state after updates', async () => {
  getFeatures.mockResolvedValue({ sales_orders: false });
  updateFeatures.mockResolvedValue({ sales_orders: true });
  const user = userEvent.setup();
  render(<FeatureProvider><Probe /><Probe /></FeatureProvider>);

  await waitFor(() => expect(screen.getAllByText('disabled')).toHaveLength(2));
  expect(getFeatures).toHaveBeenCalledOnce();
  await user.click(screen.getAllByRole('button', { name: 'Enable' })[0]);
  await waitFor(() => expect(screen.getAllByText('enabled')).toHaveLength(2));
  expect(updateFeatures).toHaveBeenCalledWith({ sales_orders: true });
});
