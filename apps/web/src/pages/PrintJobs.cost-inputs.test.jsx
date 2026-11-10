import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import PrintJobs from './PrintJobs';
import { apiPage } from '../test/apiFixtures';
import * as jobs from '../services/printJobService';
import * as materials from '../services/printJobFilamentService';
import { getModels } from '../services/modelService';
import { getFilaments } from '../services/filamentService';
import { getPrinters } from '../services/printerService';

vi.mock('../services/printJobService', () => ({ getPrintJobs: vi.fn(), getPrintJobCost: vi.fn(), createPrintJob: vi.fn(), updatePrintJob: vi.fn(), deletePrintJob: vi.fn() }));
vi.mock('../services/printJobFilamentService', () => ({ createPrintJobFilament: vi.fn(), deletePrintJobFilament: vi.fn() }));
vi.mock('../services/modelService', () => ({ getModels: vi.fn(), createModel: vi.fn() }));
vi.mock('../services/filamentService', () => ({ getFilaments: vi.fn() }));
vi.mock('../services/printerService', () => ({ getPrinters: vi.fn() }));
vi.mock('react-hot-toast', () => ({ default: { success: vi.fn(), error: vi.fn() } }));

const filament = { id: 'f1', brand: 'Fixture', type: 'PLA', color: 'Blue' };
const model = { id: 'm1', name: 'Fixture model', print_time: 60, material_weight: 100, waste_material: 0 };
const savedJob = {
  id: 'j1', printer_id: 'p1', model_id: 'm1', model: { state: 'assigned', name: 'Fixture model' },
  print_date: '2026-09-19T12:00:00Z', units_printed: 1, labor_hours: 0.5, print_time_minutes: 60,
  filaments: [{ id: 'usage-1', filament_id: 'f1', weight_used: 100, filament: { state: 'available', name: 'Fixture PLA Blue' } }],
};

const input = (name) => document.querySelector(`input[name="${name}"]`);
const change = (name, value) => fireEvent.change(input(name), { target: { value: String(value) } });

describe('Print Jobs labor and quantity inputs (C11–C17)', () => {
  beforeEach(() => {
    vi.resetAllMocks();
    jobs.getPrintJobs.mockResolvedValue(apiPage());
    jobs.createPrintJob.mockResolvedValue({ ...savedJob, id: 'created' });
    jobs.updatePrintJob.mockResolvedValue(savedJob);
    getModels.mockResolvedValue([model]);
    getFilaments.mockResolvedValue([filament]);
    getPrinters.mockResolvedValue([{ id: 'p1', name: 'Fixture printer', is_default: true }]);
    materials.createPrintJobFilament.mockResolvedValue({ id: 'new-usage' });
    materials.deletePrintJobFilament.mockResolvedValue(undefined);
  });

  it.each([0, 0.25, 0.5, 1.5])('submits %s total labor hours unchanged when automatic quantities change', async (hours) => {
    const user = userEvent.setup();
    render(<PrintJobs />);
    await user.click(await screen.findByRole('button', { name: /add first print job/i }));
    await user.click(screen.getByRole('combobox', { name: 'Model' }));
    await user.click(screen.getByRole('option', { name: 'Fixture model' }));
    change('labor_hours', hours);
    for (const units of [3, 10, 1, 3]) {
      change('units_printed', units);
      expect(input('labor_hours')).toHaveValue(hours);
      expect(input('print_time_minutes')).toHaveValue(60 * units);
      expect(screen.getByPlaceholderText('g')).toHaveValue(100 * units);
    }
    await user.click(screen.getByLabelText('Filament 1'));
    await user.click(screen.getByRole('option', { name: 'Fixture PLA — Blue' }));
    change('print_date', '2026-09-19');
    await user.click(screen.getByRole('button', { name: /create print job/i }));
    await waitFor(() => expect(jobs.createPrintJob).toHaveBeenCalledExactlyOnceWith(expect.objectContaining({ units_printed: 3, labor_hours: hours, print_time_minutes: 180 })));
    expect(materials.createPrintJobFilament).toHaveBeenCalledExactlyOnceWith({ print_job_id: 'created', filament_id: 'f1', weight_used: 300 });
  });

  it.each([0, 0.25, 0.5, 1.5])('edits to %s labor hours and keeps manual totals when units change repeatedly', async (hours) => {
    jobs.getPrintJobs.mockResolvedValue(apiPage([savedJob]));
    const user = userEvent.setup();
    render(<PrintJobs />);
    await user.click(await screen.findByRole('button', { name: /edit/i }));
    change('labor_hours', hours);
    change('print_time_minutes', 90);
    fireEvent.change(screen.getByPlaceholderText('g'), { target: { value: '150' } });
    for (const units of [3, 10, 1, 3]) {
      change('units_printed', units);
      expect(input('labor_hours')).toHaveValue(hours);
      expect(input('print_time_minutes')).toHaveValue(90);
      expect(screen.getByPlaceholderText('g')).toHaveValue(150);
    }
    await user.click(screen.getByRole('button', { name: /save changes/i }));
    await waitFor(() => expect(jobs.updatePrintJob).toHaveBeenCalledExactlyOnceWith('j1', expect.objectContaining({ units_printed: 3, labor_hours: hours, print_time_minutes: 90 })));
    expect(materials.deletePrintJobFilament).toHaveBeenCalledExactlyOnceWith('usage-1');
    expect(materials.createPrintJobFilament).toHaveBeenCalledExactlyOnceWith({ print_job_id: 'j1', filament_id: 'f1', weight_used: 150 });
  });

  it('accepts fifteen minutes as 0.25 labor hours in the native input', async () => {
    const user = userEvent.setup();
    render(<PrintJobs />);
    await user.click(await screen.findByRole('button', { name: /add first print job/i }));
    change('labor_hours', 0.25);
    expect(input('labor_hours')).toHaveValue(0.25);
    // A decimal-hour value accepted by the schema/API must not be blocked by
    // native HTML validation before react-hook-form can submit it.
    expect(input('labor_hours').validity.stepMismatch).toBe(false);
    expect(input('labor_hours').checkValidity()).toBe(true);
  });
});
