import { useState } from 'react';
import { fireEvent, render, screen, waitFor, act } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import SearchableSelect from './SearchableSelect';

it('commits IDs, cancels unmatched input, and clears a required selection explicitly', async () => {
  function Form() {
    const [value, setValue] = useState('');
    return <><SearchableSelect aria-label="Material" required options={[{ id: 'a', name: 'Ocean blue' }]}
      value={value} onChange={setValue} /><output>{value}</output></>;
  }
  const user = userEvent.setup();
  render(<Form />);
  const input = screen.getByRole('combobox', { name: 'Material' });
  expect(input).toHaveAttribute('aria-required', 'true');
  await user.type(input, 'blue{Enter}');
  expect(screen.getByText('a')).toBeInTheDocument();
  await user.type(input, 'missing{Escape}');
  expect(input).toHaveValue('Ocean blue');
  await user.click(input);
  await user.click(screen.getByRole('option', { name: 'Clear selection' }));
  expect(input).toHaveValue('');
});

it('ignores stale responses, paginates the current search and retains a selected remote label', async () => {
  const requests = [];
  const loadOptions = vi.fn(({ query, cursor }) => new Promise(resolve => requests.push({ query, cursor, resolve })));
  function Form() {
    const [value, setValue] = useState('');
    return <SearchableSelect aria-label="Part" value={value} onChange={setValue} loadOptions={loadOptions} debounceMs={1} />;
  }
  render(<Form />);
  const input = screen.getByRole('combobox');
  fireEvent.click(input);
  await waitFor(() => expect(requests).toHaveLength(1));
  fireEvent.change(input, { target: { value: 'new' } });
  await waitFor(() => expect(requests).toHaveLength(2));
  await act(() => requests[1].resolve({ items: [{ id: 'b', name: 'New bracket' }], nextCursor: 'p2' }));
  await act(() => requests[0].resolve({ items: [{ id: 'a', name: 'Old bracket' }] }));
  expect(screen.queryByText('Old bracket')).toBeNull();
  fireEvent.click(screen.getByRole('button', { name: 'Load more' }));
  await waitFor(() => expect(requests[2]).toMatchObject({ query: 'new', cursor: 'p2' }));
  await act(() => requests[2].resolve({ items: [{ id: 'c', name: 'New clamp' }] }));
  fireEvent.click(screen.getByRole('option', { name: 'New clamp' }));
  expect(input).toHaveValue('New clamp');
});

it('labels custom options, retries a failed search, and respects disabled and blur', async () => {
  const load = vi.fn().mockRejectedValueOnce(new Error('offline'))
    .mockResolvedValue({ items: [{ code: 'petg', title: 'PETG' }] });
  const blur = vi.fn();
  const props = { 'aria-label': 'Type', value: '', onChange: vi.fn(), onBlur: blur, loadOptions: load,
    getKey: option => option.code, getLabel: option => option.title,
    renderOption: option => <strong>{option.title}</strong>, debounceMs: 0 };
  const user = userEvent.setup();
  const { rerender } = render(<SearchableSelect {...props} disabled />);
  const input = screen.getByRole('combobox');
  await user.click(input);
  expect(load).not.toHaveBeenCalled();
  rerender(<SearchableSelect {...props} />);
  await user.click(input);
  expect(await screen.findByText('Could not load options')).toBeVisible();
  await user.click(screen.getByRole('button', { name: 'Try again' }));
  expect(await screen.findByRole('option', { name: 'PETG' })).toBeVisible();
  await user.keyboard('{ArrowDown}');
  expect(document.getElementById(input.getAttribute('aria-activedescendant'))).toHaveTextContent('PETG');
  await user.tab();
  expect(blur).toHaveBeenCalled();
  expect(input).toHaveAttribute('aria-expanded', 'false');
});
