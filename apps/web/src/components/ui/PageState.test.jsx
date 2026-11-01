import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { EmptyState, InlineError, TableSkeleton } from './PageState';

describe('shared page states', () => {
  it('keeps a loading table accessible while preserving table layout', () => {
    render(<TableSkeleton columns={3} rows={2} label="Loading models" />);
    expect(screen.getByRole('status', { name: 'Loading models' })).toBeInTheDocument();
    expect(document.querySelectorAll('tbody tr')).toHaveLength(2);
  });

  it('renders a persistent inline error and retries on request', async () => {
    const retry = vi.fn();
    render(<InlineError title="Could not load models" message="Your existing data has not been changed." onRetry={retry} />);
    expect(screen.getByRole('alert')).toHaveTextContent('Could not load models');
    await userEvent.setup().click(screen.getByRole('button', { name: /try again/i }));
    expect(retry).toHaveBeenCalledOnce();
  });

  it('distinguishes a filtered empty state', () => {
    render(<EmptyState filtered title="No results match your search" description="Try another search." />);
    expect(screen.getByRole('status')).toHaveTextContent('No results match your search');
    expect(screen.getByText('Search or filter returned no records.')).toBeInTheDocument();
  });
});
