import { A1_REFERENCE_PROFILE, PRINTER_PRESETS } from './printerPresets';

describe('bundled printer presets', () => {
  it('ships versioned offline examples across several manufacturers', () => {
    expect(PRINTER_PRESETS).toHaveLength(10);
    expect(new Set(PRINTER_PRESETS.map(preset => preset.manufacturer)).size).toBeGreaterThan(1);
    expect(PRINTER_PRESETS.every(preset => preset.schemaVersion === 1 && preset.migrationStrategy === 'schemaVersion')).toBe(true);
    expect(PRINTER_PRESETS.every(preset => preset.currency === 'EUR' && preset.region === 'PT/EU' && preset.reviewedAt && preset.evidence.sourceUrl)).toBe(true);
  });

  it('preserves every value from the supplied A1 reference profile', () => {
    expect(A1_REFERENCE_PROFILE).toEqual(expect.objectContaining({
      machine_cost_mode: 1, printer_purchase_cost: 299, printer_upfront_cost: 35,
      printer_annual_maintenance: 30, printer_lifetime_years: 3, printer_uptime_percent: 50,
      printer_power_watts: 150, printer_buffer_factor: 1.3, printer_wear_cost_hour: 0.08,
      electricity_cost_kwh: 0.2, filament_waste_factor: 1.1, labor_cost_hour: 25,
      margin_tier_1: 80, margin_tier_2: 90, margin_tier_3: 99,
    }));
  });
});
