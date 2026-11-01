import { useState } from 'react';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import FilamentSelect from './FilamentSelect';

const filaments = [
  { id: 'f3', brand: 'Zeta', type: 'PLA', color: 'Red' },
  { id: 'f2', brand: 'Alpha', type: 'PETG', color: 'Ocean Blue' },
  { id: 'f1', brand: 'Alpha', type: 'PLA', color: 'Black' },
];

function Harness({ initialValue = '' }) {
  const [value, setValue] = useState(initialValue);
  return (
    <FilamentSelect
      aria-label="Filament"
      value={value}
      onChange={setValue}
      filaments={filaments}
    />
  );
}

describe('FilamentSelect', () => {
  it('sorts options, filters by descriptive text, and selects with the keyboard', async () => {
    const user = userEvent.setup();
    render(<Harness />);

    const selector = screen.getByRole('combobox', { name: 'Filament' });
    await user.click(selector);
    expect(screen.getAllByRole('option').map((option) => option.textContent)).toEqual([
      'Alpha PETG — Ocean Blue',
      'Alpha PLA — Black',
      'Zeta PLA — Red',
    ]);

    await user.type(selector, 'ocean');
    expect(screen.getAllByRole('option')).toHaveLength(1);
    await user.keyboard('{Enter}');

    expect(selector).toHaveValue('Alpha PETG — Ocean Blue');
    expect(selector).toHaveAttribute('aria-expanded', 'false');
  });

  it('keeps an existing filament reference when a search is dismissed', async () => {
    const user = userEvent.setup();
    render(<Harness initialValue="f3" />);

    const selector = screen.getByRole('combobox', { name: 'Filament' });
    expect(selector).toHaveValue('Zeta PLA — Red');
    await user.click(selector);
    await user.type(selector, 'missing');
    expect(screen.getByText('No matching filaments')).toBeInTheDocument();
    await user.keyboard('{Escape}');

    expect(selector).toHaveValue('Zeta PLA — Red');
  });
});
