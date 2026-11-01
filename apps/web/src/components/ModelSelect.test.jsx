import { useState } from 'react';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import ModelSelect from './ModelSelect';

const models = [
  { id: 'm3', name: 'Zeta bracket' },
  { id: 'm2', name: 'Alpha bracket 10' },
  { id: 'm1', name: 'Alpha bracket 2' },
];

function Harness({ initialValue = '' }) {
  const [value, setValue] = useState(initialValue);
  return <ModelSelect aria-label="Model" value={value} onChange={setValue} models={models} />;
}

describe('ModelSelect', () => {
  it('sorts, searches, and selects a model with the keyboard', async () => {
    const user = userEvent.setup();
    render(<Harness />);
    const selector = screen.getByRole('combobox', { name: 'Model' });
    await user.click(selector);
    expect(screen.getAllByRole('option').map((option) => option.textContent)).toEqual([
      'No model', 'Alpha bracket 2', 'Alpha bracket 10', 'Zeta bracket',
    ]);
    await user.type(selector, 'BRACKET 10');
    expect(screen.getAllByRole('option')).toHaveLength(1);
    await user.keyboard('{Enter}');
    expect(selector).toHaveValue('Alpha bracket 10');
  });

  it('keeps the selected model when search is dismissed and can clear it', async () => {
    const user = userEvent.setup();
    render(<Harness initialValue="m3" />);
    const selector = screen.getByRole('combobox', { name: 'Model' });
    await user.click(selector);
    await user.type(selector, 'missing');
    expect(screen.getByText('No matching models')).toBeInTheDocument();
    await user.keyboard('{Escape}');
    expect(selector).toHaveValue('Zeta bracket');
    await user.click(selector);
    await user.click(screen.getByRole('option', { name: 'No model' }));
    expect(selector).toHaveValue('');
  });
});
