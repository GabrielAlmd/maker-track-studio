import { afterEach, expect, it, vi } from 'vitest';
import { cleanup, render, screen } from '@testing-library/react';
import DiagnosticErrorBoundary from './DiagnosticErrorBoundary';
import { reportFrontendFailure } from '../services/diagnostics';

vi.mock('../services/diagnostics', () => ({ reportFrontendFailure: vi.fn() }));
afterEach(() => { cleanup(); vi.clearAllMocks(); });

it('shows a recovery message and reports only a fixed event after a render error', () => {
  function BrokenScreen() { throw new Error('secret-password'); }
  render(<DiagnosticErrorBoundary><BrokenScreen /></DiagnosticErrorBoundary>, { onCaughtError: () => {} });
  expect(screen.getByRole('alert')).toHaveTextContent('Restart the application');
  expect(document.body).not.toHaveTextContent('secret-password');
  expect(reportFrontendFailure).toHaveBeenCalledExactlyOnceWith('render');
});
