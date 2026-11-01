import { useState } from 'react';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import BrandSelect from './BrandSelect';

function Harness({ initialValue = '' }) {
  const [value, setValue] = useState(initialValue);
  return <BrandSelect id="brand" value={value} onChange={setValue} brands={[' Prusament ', 'Bambu Lab', 'bambu lab']} />;
}

describe('BrandSelect', () => {
  it('deduplicates, filters, and selects an existing brand', async () => {
    const user = userEvent.setup();
    render(<Harness />);
    const input = screen.getByRole('combobox');

    await user.type(input, 'bambu');
    expect(screen.getAllByRole('option')).toHaveLength(2);
    await user.click(screen.getByRole('option', { name: 'Bambu Lab' }));

    expect(input).toHaveValue('Bambu Lab');
  });

  it('accepts a new trimmed brand without navigating away', async () => {
    const user = userEvent.setup();
    render(<Harness />);
    const input = screen.getByRole('combobox');

    await user.type(input, '  New Brand  ');
    await user.click(screen.getByRole('option', { name: 'Use “New Brand” as a new brand' }));

    expect(input).toHaveValue('New Brand');
  });

  it('shows an existing value when editing', () => {
    render(<Harness initialValue="Prusament" />);
    expect(screen.getByRole('combobox')).toHaveValue('Prusament');
  });
});
