import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import toast from 'react-hot-toast';
import DataMigrationSection from './DataMigrationSection';
import * as migrationService from '../services/migrationService';

vi.mock('../services/migrationService', () => ({
  getMigrationSchema: vi.fn(),
  inspectMigration: vi.fn(),
  previewMigration: vi.fn(),
  importMigration: vi.fn(),
  downloadMigrationTemplate: vi.fn(),
  downloadMigrationWorkbookTemplate: vi.fn(),
}));

vi.mock('react-hot-toast', () => ({ default: { success: vi.fn(), error: vi.fn() } }));

const schema = {
  sections: [
    { key: 'printers', label: 'Printers', fields: [
      { key: 'name', label: 'Name', required: true, aliases: ['printer_name'], example: 'Workshop A1' },
      { key: 'manufacturer', label: 'Manufacturer', required: true, aliases: ['maker'], example: 'Bambu Lab' },
      { key: 'model', label: 'Model', required: true, aliases: [], example: 'A1' },
      { key: 'notes', label: 'Notes', required: false, aliases: [], example: '' },
    ] },
    { key: 'filaments', label: 'Filaments', fields: [
      { key: 'brand', label: 'Brand', required: true, aliases: [], example: 'Prusa' },
    ] },
    { key: 'purchases', label: 'Purchases', fields: [
      { key: 'filament', label: 'Filament', required: true, aliases: [], example: 'Bambu Lab | PLA | Black' },
    ] },
  ],
};

describe('Data migration wizard', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    migrationService.getMigrationSchema.mockResolvedValue(schema);
    migrationService.inspectMigration.mockResolvedValue({
      format: 'csv',
      sources: [{ name: 'printers', columns: ['Printer Name', 'Maker', 'Model'], row_count: 1, sample_rows: [['Workshop', 'Bambu Lab', 'A1']] }],
    });
  });

  it('maps, previews, confirms, and imports without writing before confirmation', async () => {
    migrationService.previewMigration.mockResolvedValue({
      can_import: true,
      sections: [{ section: 'printers', summary: { ready: 0, warnings: 1, invalid: 0, duplicates: 1, skipped: 0 }, rows: [{ row: 2, status: 'warning', action: 'update', values: {}, issues: [{ code: 'duplicate_update', message: 'An existing record will be updated' }] }] }],
    });
    migrationService.importMigration.mockResolvedValue({ imported: { printers: 1 } });
    const user = userEvent.setup();
    render(<DataMigrationSection />);

    await screen.findByRole('heading', { name: 'Import spreadsheet data' });
    const file = new File(['Printer Name,Maker,Model\nWorkshop,Bambu Lab,A1'], 'printers.csv', { type: 'text/csv' });
    await user.upload(screen.getByLabelText('2. Choose a CSV or XLSX file'), file);
    await user.click(screen.getByRole('button', { name: 'Inspect file' }));

    expect(await screen.findByRole('heading', { name: '3. Map columns' })).toBeInTheDocument();
    expect(screen.getByLabelText('Name Source column')).toHaveValue('Printer Name');
    expect(screen.getByLabelText('Manufacturer Source column')).toHaveValue('Maker');
    await user.selectOptions(screen.getByLabelText('Printers Duplicate handling'), 'update');
    await user.type(screen.getByLabelText('Notes Default value'), 'Imported');
    await user.click(screen.getByRole('button', { name: 'Validate and preview' }));

    expect(await screen.findByText('An existing record will be updated')).toBeInTheDocument();
    expect(migrationService.importMigration).not.toHaveBeenCalled();
    const importButton = screen.getByRole('button', { name: 'Import data' });
    expect(importButton).toBeDisabled();
    await user.click(screen.getByRole('checkbox', { name: /I reviewed the preview/i }));
    await user.click(importButton);
    await waitFor(() => expect(migrationService.importMigration).toHaveBeenCalledOnce());
    expect(migrationService.previewMigration.mock.calls[0][0]).toBe(file);
    expect(migrationService.importMigration).toHaveBeenCalledWith(file, migrationService.previewMigration.mock.calls[0][1]);
    expect(toast.success).toHaveBeenCalledWith('Spreadsheet data imported successfully');
    expect(migrationService.previewMigration.mock.calls[0][1].sections[0]).toMatchObject({
      section: 'printers', source: 'printers', duplicate_strategy: 'update', defaults: { notes: 'Imported' },
    });
  });

  it('requires a single section for CSV sources', async () => {
    const user = userEvent.setup();
    render(<DataMigrationSection />);
    await screen.findByRole('heading', { name: 'Import spreadsheet data' });
    await user.click(screen.getByRole('checkbox', { name: 'Filaments' }));
    await user.upload(screen.getByLabelText('2. Choose a CSV or XLSX file'), new File(['data'], 'records.csv', { type: 'text/csv' }));
    await user.click(screen.getByRole('button', { name: 'Inspect file' }));
    expect(await screen.findByRole('alert')).toHaveTextContent('Choose one section for a CSV file');
    expect(migrationService.previewMigration).not.toHaveBeenCalled();
  });

  it('explains purchase references when Purchases is selected', async () => {
    const user = userEvent.setup();
    render(<DataMigrationSection />);
    await screen.findByRole('heading', { name: 'Import spreadsheet data' });
    expect(screen.queryByLabelText('Before importing purchases')).not.toBeInTheDocument();
    await user.click(screen.getByRole('checkbox', { name: 'Purchases' }));
    expect(screen.getByLabelText('Before importing purchases')).toHaveTextContent('Import filaments first');
    expect(screen.getByLabelText('Before importing purchases')).toHaveTextContent('Brand | Type | Color');
  });

  it('offers a complete XLSX template', async () => {
    const user = userEvent.setup();
    migrationService.downloadMigrationWorkbookTemplate.mockResolvedValue();
    render(<DataMigrationSection />);
    await screen.findByRole('heading', { name: 'Import spreadsheet data' });
    await user.click(screen.getByRole('button', { name: 'Complete XLSX template' }));
    expect(migrationService.downloadMigrationWorkbookTemplate).toHaveBeenCalledOnce();
    expect(screen.getByText(/replace the examples with your records/i)).toBeInTheDocument();
  });
});
