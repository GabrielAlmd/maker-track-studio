import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import Reports from './Reports';
import { exportReport } from '../services/reportService';

vi.mock('../services/reportService', () => ({ exportReport: vi.fn() }));

beforeEach(() => vi.clearAllMocks());

it('exports all data and shows the saved file', async () => {
  exportReport.mockResolvedValue({ path: '/tmp/report.zip', cancelled: false });
  const user = userEvent.setup();
  render(<Reports />);
  await user.click(screen.getByRole('button', { name: 'Generate ZIP' }));
  await waitFor(() => expect(exportReport).toHaveBeenCalledWith({ mode: 'full' }));
  expect(await screen.findByText('Export saved to /tmp/report.zip')).toBeInTheDocument();
  expect(screen.getByText(/app_config.csv/)).toBeInTheDocument();
});

it('validates categories and sends a custom date report', async () => {
  exportReport.mockResolvedValue({ path: '/tmp/period.zip', cancelled: false });
  const user = userEvent.setup();
  render(<Reports />);
  await user.click(screen.getByRole('radio', { name: 'Period report' }));
  await user.selectOptions(screen.getByLabelText('Reporting period'), 'custom');
  await user.type(screen.getByLabelText('From'), '2026-09-01');
  await user.type(screen.getByLabelText('Through'), '2026-09-30');
  await user.click(screen.getByRole('button', { name: 'Generate ZIP' }));
  await waitFor(() => expect(exportReport).toHaveBeenCalledWith({ mode: 'report', period: 'custom', date: '2026-09-01', end: '2026-09-30', categories: 'print_jobs,materials,purchases,sales,printers' }));
});

it('rejects an inverted custom range before requesting an export', async () => {
  const user = userEvent.setup();
  render(<Reports />);
  await user.click(screen.getByRole('radio', { name: 'Period report' }));
  await user.selectOptions(screen.getByLabelText('Reporting period'), 'custom');
  await user.type(screen.getByLabelText('From'), '2026-09-30');
  await user.type(screen.getByLabelText('Through'), '2026-09-01');
  await user.click(screen.getByRole('button', { name: 'Generate ZIP' }));
  expect(await screen.findByRole('alert')).toHaveTextContent('Check the reporting dates');
  expect(exportReport).not.toHaveBeenCalled();
});

it('includes recent events only when selected', async () => {
  exportReport.mockResolvedValue({ path: 'report.zip', cancelled: false });
  const user = userEvent.setup();
  render(<Reports />);
  await user.click(screen.getByRole('radio', { name: 'Period report' }));
  expect(screen.getByRole('checkbox', { name: 'Recent app events' })).not.toBeChecked();
  await user.click(screen.getByRole('checkbox', { name: 'Recent app events' }));
  await user.click(screen.getByRole('button', { name: 'Generate ZIP' }));
  await waitFor(() => expect(exportReport).toHaveBeenCalledWith(expect.objectContaining({ categories: 'print_jobs,materials,purchases,sales,printers,events' })));
});

it('requests a localized PDF summary without changing CSV export', async () => {
  exportReport.mockResolvedValue({ path: 'report.pdf', cancelled: false });
  const user = userEvent.setup();
  render(<Reports />);
  await user.click(screen.getByRole('radio', { name: 'Period report' }));
  await user.click(screen.getByRole('radio', { name: 'PDF report' }));
  await user.click(screen.getByRole('button', { name: 'Generate PDF' }));
  await waitFor(() => expect(exportReport).toHaveBeenCalledWith(expect.objectContaining({ mode: 'report', format: 'pdf', language: 'en' })));
  expect(await screen.findByText('report.pdf')).toBeInTheDocument();
  expect(screen.getByText('Included files')).toBeInTheDocument();
});
