import { fireEvent, render, screen } from '@testing-library/react';
import { expect, it, vi } from 'vitest';
import ReorderLink from './ReorderLink';
import { purchaseDetailsSchema } from '../utils/purchaseDetails';
import * as yup from 'yup';

it.each(['', '//example.test', 'javascript:alert(1)', 'https://@example.test', 'https://example.test:65536', 'https://example.test\\@other.test'])(
  'omits the reorder link for %s', (url) => {
    render(<ReorderLink url={url} />);
    expect(screen.queryByRole('link')).toBeNull();
  },
);

it('opens validated URLs through Wails and isolates browser links', () => {
  render(<ReorderLink url=" https://shop.example.test/part " />);
  const link = screen.getByRole('link', { name: 'Reorder' });
  expect(link).toHaveAttribute('href', 'https://shop.example.test/part');
  expect(link).toHaveAttribute('target', '_blank');
  expect(link).toHaveAttribute('rel', 'noopener noreferrer');

  const open = vi.fn();
  vi.stubGlobal('runtime', { BrowserOpenURL: open });
  try {
    expect(fireEvent.click(link)).toBe(false);
    expect(open).toHaveBeenCalledWith('https://shop.example.test/part');
  } finally {
    vi.unstubAllGlobals();
  }
});

it('trims optional purchase fields and rejects invalid details before submit', async () => {
  const schema = yup.object(purchaseDetailsSchema((key) => key));
  await expect(schema.validate({ supplier: ' Store ', supplier_reference: '', purchase_url: ' http://example.test/part ' }))
    .resolves.toEqual({ supplier: 'Store', supplier_reference: '', purchase_url: 'http://example.test/part' });
  await expect(schema.validate({ purchase_url: 'file:///tmp/part' })).rejects.toThrow('purchaseDetails.invalidURL');
  await expect(schema.validate({ supplier: 'X'.repeat(201) })).rejects.toThrow('purchaseDetails.invalidText');
});
