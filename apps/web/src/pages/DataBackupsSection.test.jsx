import { act, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import toast from 'react-hot-toast';
import DataBackupsSection from './DataBackupsSection';
import { importDesktopData } from '../services/appConfigService';
import { deferred } from '../test/apiFixtures';

vi.mock('../services/appConfigService', () => ({ exportDesktopData: vi.fn(), importDesktopData: vi.fn() }));
vi.mock('react-hot-toast', () => ({ default: { success: vi.fn(), error: vi.fn() } }));

describe('Backup restore UI (D01)', () => {
  beforeEach(() => vi.resetAllMocks());

  it('requires a file and submits that exact file only once while restoring', async () => {
    const pending = deferred();
    importDesktopData.mockReturnValue(pending.promise);
    const user = userEvent.setup();
    render(<DataBackupsSection />);
    const button = screen.getByRole('button', { name: 'Import Backup' });
    expect(button).toBeDisabled();
    const file = new File(['synthetic archive'], 'fixture.zip', { type: 'application/zip' });
    await user.upload(screen.getByLabelText('Desktop backup file'), file);
    await user.dblClick(button);
    expect(importDesktopData).toHaveBeenCalledExactlyOnceWith(file);
    expect(screen.getByRole('button', { name: 'Importing…' })).toBeDisabled();
    await act(async () => pending.resolve());
    expect(screen.getByRole('button', { name: 'Import Backup' })).toBeDisabled();
    expect(toast.success).toHaveBeenCalledOnce();
  });

  it('shows a failed restore and allows retry with the same file', async () => {
    importDesktopData.mockRejectedValueOnce(new Error('Invalid archive')).mockResolvedValueOnce();
    const user = userEvent.setup();
    render(<DataBackupsSection />);
    const file = new File(['synthetic archive'], 'fixture.zip', { type: 'application/zip' });
    await user.upload(screen.getByLabelText('Desktop backup file'), file);
    await user.click(screen.getByRole('button', { name: 'Import Backup' }));
    expect(await screen.findByRole('alert')).toHaveTextContent('Invalid archive');
    await user.click(screen.getByRole('button', { name: 'Import Backup' }));
    await waitFor(() => expect(importDesktopData).toHaveBeenCalledTimes(2));
    expect(importDesktopData.mock.calls).toEqual([[file], [file]]);
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
  });

  it('clears the visible file input after a successful restore so the same file can be selected again', async () => {
    importDesktopData.mockResolvedValue();
    const user = userEvent.setup();
    render(<DataBackupsSection />);
    const file = new File(['synthetic archive'], 'fixture.zip', { type: 'application/zip' });
    const selector = screen.getByLabelText('Desktop backup file');
    await user.upload(selector, file);
    await user.click(screen.getByRole('button', { name: 'Import Backup' }));
    await waitFor(() => expect(screen.getByRole('button', { name: 'Import Backup' })).toBeDisabled());
    expect(selector.files).toHaveLength(0);
    await user.upload(selector, file);
    expect(screen.getByRole('button', { name: 'Import Backup' })).toBeEnabled();
  });
});
