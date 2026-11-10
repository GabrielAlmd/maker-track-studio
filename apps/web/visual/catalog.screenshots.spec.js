import { expect, test } from '@playwright/test';
import { installApiFixtures } from './catalog-fixtures';

async function expandScrollableAppForScreenshot(page) {
  await page.addStyleTag({
    content: `
      html, body, #root { height: auto !important; min-height: 100vh; }
      .app-shell { height: auto !important; min-height: 100vh; }
      .app-shell > .min-w-0 { overflow: visible !important; }
      .app-shell main { flex: none !important; overflow: visible !important; }
    `,
  });
}

async function waitForBundledFonts(page) {
  await page.evaluate(async () => {
    await document.fonts?.ready;
    if (document.fonts?.status !== 'loaded') throw new Error('Interface fonts did not finish loading.');
    await new Promise((resolve) => requestAnimationFrame(() => requestAnimationFrame(resolve)));
  });
}

const ready = {
  dashboard: async (page) => {
    await expect(page.getByText('Modular Enclosure')).toBeVisible();
    await expect(page.getByRole('heading', { name: 'Business' })).toBeVisible();
  },
  models: (page) => expect(page.getByText('Workshop Bracket')).toBeVisible(),
  filaments: (page) => expect(page.getByText('Galaxy Silver')).toBeVisible(),
  printJobs: async (page) => {
    await expect(page.getByRole('article', { name: /Modular Enclosure print job/i })).toBeVisible();
    await expect(page.getByText('Loading allocation capacity…')).toHaveCount(0);
    await expect(page.getByText('Units allocated').first()).toBeVisible();
  },
  printers: (page) => expect(page.getByText('A1 – Workshop')).toBeVisible(),
  purchases: (page) => expect(page.getByText('Galaxy Silver')).toBeVisible(),
  sales: (page) => expect(page.getByRole('link', { name: 'WEB-1042' })).toBeVisible(),
};

const scenarios = [
  ...['empty', 'pending', 'error'].map((state) => ({
    slug: `purchases-first-run-${state}`, route: 'purchases',
    setup: async (page) => {
      await page.route('**/api/filament-purchases*', (route) => route.fulfill({ json: { items: [], total: 0, page: 1, page_size: 25 } }));
      await page.route('**/api/filaments', (route) => {
        if (state === 'pending') return;
        return route.fulfill({ status: state === 'error' ? 500 : 200, json: state === 'error' ? { error: 'Fixture lookup unavailable' } : [] });
      });
    },
    prepare: async (page) => {
      await page.getByRole('button', { name: 'Record first purchase' }).click();
      await expect(page.getByRole('dialog')).toBeVisible();
      if (state === 'pending') await expect(page.getByRole('dialog').getByRole('status')).toBeVisible();
      if (state === 'error') await expect(page.getByRole('button', { name: /try again/i })).toBeVisible();
    },
  })),
  { slug: 'dashboard', route: 'dashboard', fullPage: true, prepare: ready.dashboard },
  { slug: 'dashboard-narrow', route: 'dashboard', viewport: { width: 390, height: 844 }, fullPage: true, prepare: ready.dashboard },
  {
    slug: 'dashboard-first-run', route: 'dashboard', fullPage: true,
    setup: async (page) => {
      await page.route('http://127.0.0.1:4173/api/features', (route) => route.fulfill({ status: 200, json: { sales_orders: false } }));
      await page.route('http://127.0.0.1:4173/api/print-jobs/dashboard-summary', (route) => route.fulfill({ status: 200, json: {
        total_models: 0, total_filaments: 0, total_purchases: 0, total_print_jobs: 0,
        cost_stats: { total_units_printed: 0, total_filament_weight_g: 0, calculated_jobs: 0, total_filament_cost: 0, total_electricity_cost: 0, total_wear_cost: 0, total_labor_cost: 0, total_cost: 0 },
        expenses: { printer_cost: 0, filament_cost: 0, spare_parts_cost: 0, additional_cost: 0, other_cost: 0, total_expenses: 0 },
        recent_print_jobs: [],
      } }));
    },
    prepare: (page) => expect(page.getByRole('heading', { name: 'Already have spreadsheet records?' })).toBeVisible(),
  },
  { slug: 'models', route: 'models', prepare: ready.models },
  {
    slug: 'models-bulk-selection', route: 'models', prepare: async (page) => {
      await ready.models(page);
      await page.getByRole('checkbox', { name: 'Select Modular Enclosure' }).check();
      await page.getByRole('checkbox', { name: 'Select Workshop Bracket' }).check();
      await expect(page.getByText('2 records selected')).toBeVisible();
      await expect(page.getByRole('button', { name: 'Delete selected' })).toBeVisible();
    },
  },
  {
    slug: 'models-add', route: 'models', prepare: async (page) => {
      await ready.models(page);
      await page.getByRole('button', { name: 'Add New Model' }).click();
      await expect(page.getByRole('heading', { name: 'Add New Model' })).toBeVisible();
    },
  },
  {
    slug: 'models-filters', route: 'models', prepare: async (page) => {
      await ready.models(page);
      await page.locator('summary').filter({ hasText: 'Filters' }).click();
      await expect(page.getByLabel('Minimum print time')).toBeVisible();
    },
  },
  { slug: 'filaments', route: 'filaments', prepare: ready.filaments },
  {
    slug: 'filaments-add', route: 'filaments', prepare: async (page) => {
      await ready.filaments(page);
      await page.getByRole('button', { name: 'Add New Filament' }).click();
      await expect(page.getByRole('heading', { name: 'Add New Filament' })).toBeVisible();
    },
  },
  {
    slug: 'filament-types', route: 'filaments', prepare: async (page) => {
      await ready.filaments(page);
      await page.getByRole('button', { name: 'Manage types' }).first().click();
      await expect(page.getByRole('heading', { name: 'Manage filament types' })).toBeVisible();
    },
  },
  { slug: 'print-jobs', route: 'print-jobs', prepare: ready.printJobs },
  { slug: 'print-jobs-narrow', route: 'print-jobs', viewport: { width: 390, height: 844 }, fullPage: true, prepare: ready.printJobs },
  {
    slug: 'print-jobs-model-filter', route: 'print-jobs', prepare: async (page) => {
      await ready.printJobs(page);
      await page.locator('summary').filter({ hasText: 'Filters' }).click();
      await page.getByRole('combobox', { name: 'Model' }).fill('bracket');
      await expect(page.getByRole('option', { name: 'Workshop Bracket' })).toBeVisible();
    },
  },
  {
    slug: 'print-jobs-add', route: 'print-jobs', prepare: async (page) => {
      await ready.printJobs(page);
      await page.getByRole('button', { name: 'Add Print Job' }).click();
      const modal = page.getByRole('heading', { name: 'Add New Print Job' }).locator('../..');
      await modal.getByRole('combobox', { name: 'Model' }).fill('modular');
      await modal.getByRole('option', { name: 'Modular Enclosure' }).click();
      await modal.locator('input[name="print_date"]').fill('2026-09-18');
      await modal.locator('input[name="labor_hours"]').fill('0.25');
      await modal.getByLabel('Filament 1').fill('matte');
      await modal.getByRole('option', { name: 'Bambu Lab PLA — Matte Black' }).click();
      await expect(modal).toBeVisible();
    },
  },
  {
    slug: 'print-jobs-add-narrow', route: 'print-jobs', viewport: { width: 390, height: 844 }, prepare: async (page) => {
      await ready.printJobs(page);
      await page.getByRole('button', { name: 'Add Print Job' }).click();
      await expect(page.getByRole('heading', { name: 'Add New Print Job' })).toBeVisible();
    },
  },
  { slug: 'printers', route: 'printers', prepare: ready.printers },
  {
    slug: 'printers-add', route: 'printers', prepare: async (page) => {
      await ready.printers(page);
      await page.getByRole('button', { name: 'Add Printer' }).click();
      await expect(page.getByRole('heading', { name: 'Add Printer' })).toBeVisible();
    },
  },
  {
    slug: 'printers-add-preset', route: 'printers', prepare: async (page) => {
      await ready.printers(page);
      await page.getByRole('button', { name: 'Add Printer' }).click();
      await page.getByLabel('Choose a printer preset').selectOption('bambu-lab-a1-v1');
      await page.getByRole('button', { name: 'Apply preset values' }).click();
      await expect(page.getByLabel('Review and edit preset values')).toBeVisible();
    },
  },
  {
    slug: 'printers-costs', route: 'printers', fullPage: true, prepare: async (page) => {
      await ready.printers(page);
      await page.getByRole('button', { name: 'Edit costs' }).first().click();
      await expect(page.getByRole('heading', { name: 'Costs for A1 – Workshop' })).toBeVisible();
    },
  },
  {
    slug: 'print-job-costs', route: 'print-jobs', prepare: async (page) => {
      await ready.printJobs(page);
      await page.getByRole('button', { name: 'Cost breakdown' }).first().click();
      await expect(page.getByText('€18.40').first()).toBeVisible();
    },
  },
  {
    slug: 'print-jobs-production-only', route: 'print-jobs',
    setup: async (page) => {
      await page.route('http://127.0.0.1:4173/api/features', (route) => route.fulfill({ status: 200, json: { sales_orders: false } }));
    },
    prepare: async (page) => {
      await expect(page.getByRole('article', { name: /Modular Enclosure print job/i })).toBeVisible();
      await expect(page.getByText('Units allocated')).toHaveCount(0);
    },
  },
  { slug: 'purchases', route: 'purchases', prepare: ready.purchases },
  { slug: 'reports', route: 'reports', fullPage: true, prepare: async (page) => {
    await expect(page.getByRole('heading', { name: 'Reports & Data Export' })).toBeVisible();
    await page.getByRole('radio', { name: 'Period report' }).check();
    await expect(page.getByRole('checkbox', { name: 'Materials' })).toBeChecked();
  } },
  { slug: 'reports-pdf', route: 'reports', fullPage: true, prepare: async (page) => {
    await page.getByRole('radio', { name: 'Period report' }).check();
    await page.getByRole('radio', { name: 'PDF report' }).check();
    await expect(page.getByRole('button', { name: 'Generate PDF' })).toBeVisible();
  } },
  { slug: 'sales', route: 'sales', fullPage: true, prepare: ready.sales },
  { slug: 'sales-compact', route: 'sales', viewport: { width: 900, height: 900 }, fullPage: true, prepare: ready.sales },
  {
    slug: 'sales-filters', route: 'sales', fullPage: true, prepare: async (page) => {
      await ready.sales(page);
      await page.locator('summary').filter({ hasText: 'Filters' }).click();
      await page.getByRole('combobox', { name: 'Fulfilment status', exact: true }).selectOption('preparing');
      await expect(page.locator('summary').filter({ hasText: 'Filters (1)' })).toBeVisible();
    },
  },
  {
    slug: 'sales-new', route: 'sales/new', fullPage: true, prepare: async (page) => {
      await expect(page.getByRole('heading', { name: 'New order' })).toBeVisible();
      await expect(page.getByRole('heading', { name: 'Order details' })).toBeVisible();
      await page.getByLabel('Model').selectOption('m1');
      await page.getByRole('spinbutton', { name: 'Target profit (%)' }).fill('150');
      await expect(page.getByText('Suggested unit price: €11.50')).toBeVisible();
      await page.getByRole('button', { name: 'Help for Target profit (%)' }).hover();
      await expect(page.getByRole('tooltip')).toBeVisible();
    },
  },
  {
    slug: 'sales-new-compact', route: 'sales/new', viewport: { width: 900, height: 900 }, fullPage: true, prepare: async (page) => {
      await expect(page.getByRole('heading', { name: 'New order' })).toBeVisible();
      await expect(page.getByRole('heading', { name: 'Order details' })).toBeVisible();
    },
  },
  {
    slug: 'sales-new-narrow', route: 'sales/new', viewport: { width: 390, height: 844 }, fullPage: true, prepare: async (page) => {
      await expect(page.getByRole('heading', { name: 'New order' })).toBeVisible();
      await expect(page.getByRole('heading', { name: 'Order details' })).toBeVisible();
    },
  },
  {
    slug: 'sales-detail', route: 'sales/so1', fullPage: true, prepare: async (page) => {
      await expect(page.getByRole('heading', { name: 'WEB-1042' })).toBeVisible();
      await expect(page.getByRole('heading', { name: 'Production allocations' })).toBeVisible();
    },
  },
  {
    slug: 'sales-order-completed', route: 'sales/so2', fullPage: true, prepare: async (page) => {
      await expect(page.getByRole('heading', { name: 'DIRECT-031' })).toBeVisible();
      await expect(page.getByText('Historical financial values are frozen.')).toBeVisible();
      await expect(page.getByRole('button', { name: 'Save changes' })).toHaveCount(0);
    },
  },
  {
    slug: 'sales-order-production-allocation', route: 'sales/so1', fullPage: true, prepare: async (page) => {
      await expect(page.getByRole('heading', { name: 'WEB-1042' })).toBeVisible();
      await expect(page.getByRole('heading', { name: 'Production allocations' })).toBeVisible();
      await expect(page.getByLabel('Units assigned to Print Job j1')).toHaveValue('1');
    },
  },
  {
    slug: 'sales-channel-add', route: 'sales/new', fullPage: true, prepare: async (page) => {
      await expect(page.getByLabel('Sales channel')).toHaveValue('');
      await page.getByRole('button', { name: 'Add Sales Channel' }).click();
      await expect(page.getByRole('heading', { name: 'Add Sales Channel' })).toBeVisible();
      await expect(page.getByLabel('Channel name')).toBeVisible();
    },
  },
  {
    slug: 'sales-new-pt', route: 'sales/new', locale: 'pt-PT', fullPage: true, prepare: async (page) => {
      await expect(page.getByRole('heading', { name: 'Nova encomenda' })).toBeVisible();
      await expect(page.getByRole('heading', { name: 'Detalhes da encomenda' })).toBeVisible();
      await expect(page.getByLabel('Canal de venda')).toHaveValue('');
      await expect(page.getByLabel('Canal de venda')).toContainText('Sem canal de venda');
    },
  },
  {
    slug: 'purchases-add', route: 'purchases', prepare: async (page) => {
      await ready.purchases(page);
      await page.getByRole('button', { name: 'Record Purchase' }).click();
      await page.getByLabel('Purchase Date').fill('2026-09-18');
      await page.getByRole('button', { name: 'Create new filament' }).click();
      const brand = page.getByRole('combobox', { name: 'Brand', exact: true });
      await brand.fill('bambu');
      await page.getByRole('option', { name: 'Bambu Lab' }).click();
      await page.getByLabel('Type').selectOption('PLA');
      await page.getByLabel('Color').fill('Jade White');
      await page.getByLabel('Price (€)').fill('24.99');
      await page.getByLabel('Amount (kg)').fill('1');
      await brand.click();
      await expect(page.getByText('Total: €24.99')).toBeVisible();
    },
  },
  {
    slug: 'settings-appearance', route: 'settings', fullPage: true, prepare: (page) => expect(page.getByRole('radiogroup', { name: 'App theme' })).toBeVisible(),
  },
  {
    slug: 'settings-appearance-pt', route: 'settings', locale: 'pt-PT', fullPage: true, prepare: (page) => expect(page.getByRole('heading', { name: 'Definições' })).toBeVisible(),
  },
  {
    slug: 'settings-calculations', route: 'settings?section=calculations', fullPage: true, prepare: (page) => expect(page.getByRole('radio', { name: /Simple/ })).toBeVisible(),
  },
  {
    slug: 'settings-features', route: 'settings?section=features', fullPage: true, prepare: (page) => expect(page.getByText('Track orders, selling prices, production allocation, revenue and profit.')).toBeVisible(),
  },
  {
    slug: 'settings-calculations-advanced', route: 'settings?section=calculations', fullPage: true, prepare: async (page) => {
      await page.getByRole('radio', { name: /Advanced/ }).click();
      await expect(page.getByText('Total investment')).toBeVisible();
    },
  },
  {
    slug: 'settings-backups', route: 'settings?section=backups', prepare: async (page) => {
      await expect(page.getByText('Restore desktop backup')).toBeVisible();
      await page.getByLabel('Desktop backup file').setInputFiles({ name: 'makertrack-backup.zip', mimeType: 'application/zip', buffer: Buffer.from('fixture') });
      await expect(page.getByRole('button', { name: 'Import Backup' })).toBeEnabled();
    },
  },
  {
    slug: 'settings-migration', route: 'settings?section=migration', fullPage: true, prepare: async (page) => {
      await expect(page.getByRole('heading', { name: 'Import spreadsheet data' })).toBeVisible();
      await page.getByLabel('2. Choose a CSV or XLSX file').setInputFiles({ name: 'printers.csv', mimeType: 'text/csv', buffer: Buffer.from('Name,Manufacturer,Model,Notes\nWorkshop A1,Bambu Lab,A1,Primary printer\nEnclosed P1S,Bambu Lab,P1S,Engineering materials') });
      await page.getByRole('button', { name: 'Inspect file' }).click();
      await page.getByRole('button', { name: 'Validate and preview' }).click();
      await expect(page.getByRole('heading', { name: '4. Review preview' })).toBeVisible();
    },
  },
  {
    slug: 'settings-migration-purchases', route: 'settings?section=migration', fullPage: true, prepare: async (page) => {
      await expect(page.getByRole('heading', { name: 'Import spreadsheet data' })).toBeVisible();
      await page.getByRole('checkbox', { name: 'Printers' }).uncheck();
      await page.getByRole('checkbox', { name: 'Purchases' }).check();
      await expect(page.getByLabel('Before importing purchases')).toContainText('Brand | Type | Color');
    },
  },
  {
    slug: 'navigation-focus', route: 'dashboard', prepare: async (page) => {
      await ready.dashboard(page);
      await page.getByRole('link', { name: 'Print Jobs', exact: true }).focus();
      await expect(page.getByRole('link', { name: 'Print Jobs', exact: true })).toBeFocused();
    },
  },
  {
    slug: 'navigation-hover', route: 'dashboard', prepare: async (page) => {
      await ready.dashboard(page);
      await page.getByRole('link', { name: 'Models', exact: true }).hover();
    },
  },
];

test.beforeEach(async ({ page }) => {
  await installApiFixtures(page);
});

for (const theme of ['light', 'dark']) {
  for (const scenario of scenarios) {
    test(`${scenario.slug} documentation screenshot in ${theme} mode`, async ({ page }) => {
      await page.addInitScript(({ selectedTheme, selectedLocale }) => {
        window.localStorage.setItem('maker-track-studio-theme', selectedTheme);
        window.localStorage.setItem('maker-track-studio-locale', selectedLocale);
        // Set the fixture theme before React mounts, so native controls never
        // paint in the default dark theme during a light-theme capture.
        const applyTheme = () => {
          if (!document.documentElement) return false;
          document.documentElement.dataset.theme = selectedTheme;
          return true;
        };
        if (!applyTheme()) {
          const observer = new MutationObserver(() => {
            if (applyTheme()) observer.disconnect();
          });
          observer.observe(document, { childList: true });
        }
      }, { selectedTheme: theme, selectedLocale: scenario.locale ?? 'en' });

      // Report defaults read the current month at module load. Keep the
      // catalogue independent of the date on the developer or CI machine.
      if (['reports', 'reports-pdf'].includes(scenario.slug)) {
        await page.clock.setFixedTime(new Date('2026-10-15T12:00:00Z'));
      }
      if (scenario.viewport) await page.setViewportSize(scenario.viewport);
      if (scenario.setup) await scenario.setup(page);
      await page.goto(`/#/${scenario.route}`);
      await scenario.prepare(page);
      await expect(page.locator('html')).toHaveAttribute('data-theme', theme);
      if (scenario.fullPage) await expandScrollableAppForScreenshot(page);
      await waitForBundledFonts(page);

      const screenshotOptions = {
        fullPage: Boolean(scenario.fullPage),
      };
      // Small type and native controls on this screen vary slightly between
      // Chromium rasterizers; CI observed 602 differing pixels with otherwise
      // visually identical output. Keep the allowance local to this capture.
      if (scenario.slug === 'purchases' && theme === 'dark') {
        screenshotOptions.maxDiffPixels = 650;
      }
      await expect(page).toHaveScreenshot(`${scenario.slug}-${theme}.png`, screenshotOptions);
    });
  }
}
