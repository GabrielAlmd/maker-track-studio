import i18n from '../i18n';

export const COST_SETTINGS = {
  machine_cost_mode: ['Machine cost calculation', 'Choose Simple to preserve existing hourly costs, or Advanced to calculate the hourly machine rate from the advanced inputs below. Changes recalculate all recorded jobs.', 'Calculation'],
  printer_purchase_cost: ['Printer Purchase Cost (€)', 'Adv. Inputs C11. Total purchase price, including delivery and taxes.', 'Machine investment'],
  printer_upfront_cost: ['Additional Upfront Cost (€)', 'Adv. Inputs C12. Upgrades and other initial expenses. Added to the purchase price to calculate total investment.', 'Machine investment'],
  printer_annual_maintenance: ['Annual Maintenance & Repair (€)', 'Adv. Inputs C15. Estimated yearly maintenance. Multiplied by the lifetime in years and added to the investment. Enter your own estimate; it is not automatically 10% of the purchase price.', 'Machine investment'],
  printer_lifetime_years: ['Estimated Life (years)', 'Adv. Inputs C18. Years of service over which the investment and maintenance costs are spread. Must be at least 0.001.', 'Machine investment'],
  printer_uptime_percent: ['Estimated Uptime (%)', 'Adv. Inputs C19. Percentage of the 8,760 hours in a year spent printing. 50% means 4,380 printing hours per year. Must be between 0.001 and 100.', 'Machine investment'],
  printer_power_watts: ['Average Printer Power (W)', 'Adv. Inputs C22. Average electrical power while printing, including warm-up. Divided by 1,000 and multiplied by the electricity tariff to obtain €/hour.', 'Energy and buffer'],
  electricity_cost_kwh: ['Electricity Cost (per kWh)', 'Adv. Inputs C23. Price of one kWh in euros. Advanced mode multiplies this by power in kW. Simple mode preserves the old calculation, which treats this value as an hourly electricity charge.', 'Energy and buffer'],
  printer_buffer_factor: ['Printer Cost Buffer Factor', 'Adv. Inputs C28. Multiplies BOTH capital and electricity costs for unforeseen expenses. 1.3 adds 30%; 1 adds no buffer. Applies only in Advanced mode.', 'Energy and buffer'],
  printer_wear_cost_hour: ['Printer Wear Cost (per hour)', 'Manual hourly wear cost in euros, excluding electricity. Used only in Simple mode. Advanced mode replaces it with the calculated capital cost and buffer.', 'Simple rate'],
  labor_cost_hour: ['Labor Cost (per hour)', 'Adv. Inputs C6. Hourly labor rate in euros. Multiplied by the labor hours entered on each print job, not its machine printing time.', 'Material and labor'],
  filament_waste_factor: ['Filament Waste Factor', 'Adv. Inputs C4. Multiplies the filament cost to account for waste. 1.1 adds 10%; 1 adds no waste. A legacy value of 0 is treated as 1.', 'Material and labor'],
};

export function translatedCostSetting(key, t = i18n.t) {
  return [t(`costSettings.fields.${key}.label`, { defaultValue: COST_SETTINGS[key][0] }), t(`costSettings.fields.${key}.help`, { defaultValue: COST_SETTINGS[key][1] }), COST_SETTINGS[key][2]];
}

export const normalizeCostValue = value => String(value ?? '').trim().replace(',', '.');

export function valueError(key, raw) {
  const value = Number(raw);
  if (!/^[+-]?(?:\d+(?:\.\d*)?|\.\d+)(?:e[+-]?\d+)?$/i.test(String(raw).trim()) || !Number.isFinite(value)) return i18n.t('costSettings.validation.number');
  if (value < 0) return i18n.t('costSettings.validation.nonNegative');
  if (value > 1e9) return i18n.t('costSettings.validation.maximum');
  if (['printer_lifetime_years', 'printer_uptime_percent'].includes(key) && value < 0.001) return i18n.t('costSettings.validation.minimum');
  if (key === 'printer_uptime_percent' && value > 100) return i18n.t('costSettings.validation.uptime');
  if (key === 'printer_buffer_factor' && value < 1) return i18n.t('costSettings.validation.buffer');
  if (key === 'machine_cost_mode' && ![0, 1].includes(value)) return i18n.t('costSettings.validation.mode');
  return null;
}

export function machineRates(configs) {
  const raw = Object.fromEntries(configs.map(({ key, value }) => [key, value]));
  const cfg = Object.fromEntries(configs.map(({ key, value }) => [key, Number(value)]));
  if (!['printer_purchase_cost', 'printer_upfront_cost', 'printer_annual_maintenance', 'printer_lifetime_years', 'printer_uptime_percent', 'printer_power_watts', 'printer_buffer_factor', 'electricity_cost_kwh'].every(key => raw[key] !== undefined && !valueError(key, raw[key]))) return null;
  const investment = cfg.printer_purchase_cost + cfg.printer_upfront_cost;
  const lifetime = investment + cfg.printer_annual_maintenance * cfg.printer_lifetime_years;
  const hours = 8760 * cfg.printer_uptime_percent / 100;
  const capital = lifetime / (hours * cfg.printer_lifetime_years);
  const electricity = cfg.printer_power_watts / 1000 * cfg.electricity_cost_kwh;
  return { investment, lifetime, hours, capital, electricity, total: (capital + electricity) * cfg.printer_buffer_factor, active: cfg.machine_cost_mode === 1 };
}

// Only fields that contribute to the selected calculation can be edited.
export function settingLockReason(key, advanced) {
  const advancedOnly = [
    'printer_purchase_cost', 'printer_upfront_cost', 'printer_annual_maintenance',
    'printer_lifetime_years', 'printer_uptime_percent', 'printer_power_watts', 'printer_buffer_factor',
  ];
  if (!advanced && advancedOnly.includes(key)) return i18n.t('costSettings.availableAdvanced');
  if (advanced && key === 'printer_wear_cost_hour') return i18n.t('costSettings.calculatedAdvanced');
  return null;
}
