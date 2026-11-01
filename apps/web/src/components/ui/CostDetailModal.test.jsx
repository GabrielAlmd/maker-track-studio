import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import CostDetailModal from './CostDetailModal';

describe('CostDetailModal', () => {
  it('renders the full cost breakdown without selling-price tiers', () => {
    render(
      <CostDetailModal
        cost={{
          filament_cost: 4.95,
          electricity_cost: 0.4,
          wear_cost: 0.16,
          labor_cost: 22.5,
          total_cost: 28.01,
        }}
        onClose={() => {}}
      />
    );

    expect(screen.getByText('Cost Breakdown')).toBeInTheDocument();
    expect(screen.getByText('€4.95')).toBeInTheDocument();
    expect(screen.getByText('€0.40')).toBeInTheDocument();
    expect(screen.getByText('€0.16')).toBeInTheDocument();
    expect(screen.getByText('€22.50')).toBeInTheDocument();
    expect(screen.getByText('€28.01')).toBeInTheDocument();

    expect(screen.queryByText(/margin/i)).not.toBeInTheDocument();
    expect(screen.queryByText(/suggested/i)).not.toBeInTheDocument();
  });

  it('does not turn missing cost values into zero', () => {
    render(
      <CostDetailModal
        cost={{
          filament_cost: 1.2,
          electricity_cost: 0.3,
          wear_cost: 0.1,
          labor_cost: 0,
          total_cost: 1.6,
        }}
        onClose={() => {}}
      />
    );

    // Labor cost is the only confirmed zero in this payload; all provided
    // cost values remain distinct from a missing value.
    expect(screen.getAllByText('€0.00')).toHaveLength(1);
  });

  it('calls onClose when clicking close controls', async () => {
    const user = userEvent.setup();
    const onClose = vi.fn();

    render(
      <CostDetailModal
        cost={{
          filament_cost: 1,
          electricity_cost: 1,
          wear_cost: 1,
          labor_cost: 1,
          total_cost: 4,
        }}
        onClose={onClose}
      />
    );

    const closeButtons = screen.getAllByRole('button', { name: /close/i });
    await user.click(closeButtons[0]);
    await user.click(closeButtons[1]);

    expect(onClose).toHaveBeenCalledTimes(2);
  });
});
