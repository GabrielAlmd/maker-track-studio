import { LockKeyhole } from 'lucide-react';
import HelpTooltip from '../components/ui/HelpTooltip';
import { COST_SETTINGS, machineRates, normalizeCostValue, settingLockReason, translatedCostSetting, valueError } from './costSettings';
import { VALUE_FALLBACKS, formatCurrency, formatNumber } from '../utils/formatters';
import { useTranslation } from 'react-i18next';

const COST_GROUPS = [
  ['Hourly costs', 'Set your electricity and manual wear rates.', ['electricity_cost_kwh', 'printer_wear_cost_hour']],
  ['Machine investment', 'Tell us what your printer costs and how much you use it.', ['printer_purchase_cost', 'printer_annual_maintenance', 'printer_lifetime_years', 'printer_uptime_percent']],
  ['Power and buffer', 'Account for power consumption and unexpected expenses.', ['printer_power_watts', 'printer_buffer_factor']],
  ['Material and labor', 'Allow for filament waste and your time.', ['filament_waste_factor', 'labor_cost_hour']],
];

function CostPreview({ values, dirty, applyToExistingJobs }) {
  const { t } = useTranslation();
  const advanced = values.machine_cost_mode === '1';
  const entries = Object.entries(values).map(([key, value]) => ({ key, value: normalizeCostValue(value) }));
  const rates = machineRates(entries);
  const simpleValid = ['electricity_cost_kwh', 'printer_wear_cost_hour'].every(key => !valueError(key, normalizeCostValue(values[key])));
  const total = advanced ? rates?.total : simpleValid ? Number(normalizeCostValue(values.electricity_cost_kwh)) + Number(normalizeCostValue(values.printer_wear_cost_hour)) : null;
  return <aside className="rounded-xl border ui-border-primary ui-tone-primary p-5 sm:p-6" aria-label={t('costSettings.preview')}>
    <p className="text-sm font-medium text-[var(--mt-text-primary)]">{t('costSettings.estimated')}</p>
    <p className="mt-2 text-3xl font-semibold text-[var(--mt-text-primary)]">{formatCurrency(total, { fractionDigits: 4, fallback: VALUE_FALLBACKS.notCalculated })}<span className="text-sm font-normal"> {t('costSettings.perHour')}</span></p>
    <p className="mt-2 text-sm text-[var(--mt-text-secondary)]">{t(dirty ? 'costSettings.previewChanges' : 'costSettings.savedSettings')} {t(applyToExistingJobs ? 'costSettings.applyExisting' : 'costSettings.keepExisting')}</p>
    {advanced && rates && <dl className="mt-5 grid gap-4 border-t ui-border-primary pt-4 text-base text-[var(--mt-text-primary)] sm:grid-cols-2 lg:grid-cols-3">
      {[
        [t('costSettings.totalInvestment'), formatCurrency(rates.investment)],
        [t('costSettings.lifetimeCost'), formatCurrency(rates.lifetime)],
        [t('costSettings.annualHours'), `${formatNumber(rates.hours, { minimumFractionDigits: 1, maximumFractionDigits: 1 })} h`],
        [t('costSettings.capitalBefore'), `${formatCurrency(rates.capital, { fractionDigits: 4 })}/h`],
        [t('costSettings.electricityBefore'), `${formatCurrency(rates.electricity, { fractionDigits: 4 })}/h`],
      ].map(([label, value]) => <div key={label} className="space-y-1"><dt className="text-sm text-[var(--mt-text-secondary)]">{label}</dt><dd className="font-semibold">{value}</dd></div>)}
    </dl>}
    <p className="mt-5 text-sm leading-relaxed text-[var(--mt-text-secondary)]">{t(advanced ? 'costSettings.advancedFormula' : 'costSettings.simpleFormula')} {t('costSettings.jobExtras')}</p>
  </aside>;
}

// The same inputs and validation rules are deliberately rendered for the
// new-printer template and for a physical printer profile. Only their owner
// and save action differ.
export default function CostConfigFields({ values, availableKeys, register, errors, dirty, idPrefix, applyToExistingJobs = false, machineInvestmentExtra }) {
  const { t } = useTranslation();
  const advanced = values.machine_cost_mode === '1';
  const available = new Set(availableKeys);
  return <>
    {available.has('machine_cost_mode') && <fieldset className="mb-6">
      <legend className="mb-3 text-base font-semibold ui-text-primary">{t('costSettings.mode')} <HelpTooltip label={t('costSettings.machineCalculation')}>{translatedCostSetting('machine_cost_mode', t)[1]}</HelpTooltip></legend>
      <div className="grid gap-3 sm:grid-cols-2">
        {[
          ['0', t('costSettings.simple'), t('costSettings.simpleHelp')],
          ['1', t('costSettings.advanced'), t('costSettings.advancedHelp')],
        ].map(([value, label, description]) => <label key={value} className={`flex cursor-pointer items-start gap-3 rounded-xl border p-4 ${values.machine_cost_mode === value ? 'ui-border-primary ui-tone-primary ring-1 ui-focus-ring' : 'ui-border-subtle hover:ui-border-primary'}`}>
          <input {...register('machine_cost_mode')} type="radio" value={value} className="mt-1 accent-blue-600" />
          <span><span className="block font-semibold ui-text-primary">{label}</span><span className="mt-1 block text-sm ui-text-secondary">{description}</span></span>
        </label>)}
      </div>
    </fieldset>}
    <div className="mb-6"><CostPreview values={values} dirty={dirty} applyToExistingJobs={applyToExistingJobs} /></div>
    <div className="space-y-6">
      {COST_GROUPS.map(([, , keys], groupIndex) => {
        const title = t(`costSettings.groups.${groupIndex}.title`);
        const description = t(`costSettings.groups.${groupIndex}.description`);
        const groupKeys = keys.filter(key => available.has(key));
        if (!groupKeys.length) return null;
        return <section key={title} className="rounded-xl border ui-border-subtle p-4">
          <h3 className="text-lg font-semibold ui-text-primary">{title}</h3>
          <p className="mt-1 text-sm leading-relaxed ui-text-secondary">{description}</p>
          <div className="mt-4 grid gap-4 sm:grid-cols-2">
            {groupKeys.map(key => {
              const locked = settingLockReason(key, advanced);
              const setting = translatedCostSetting(key, t);
              const label = key === 'electricity_cost_kwh' && !advanced ? t('costSettings.electricityHour') : setting[0];
              const inputID = `${idPrefix}-${key}`;
              return <div key={key}>
                <div className="mb-1 flex items-center"><label htmlFor={inputID} className="text-base font-medium ui-text-primary">{label}</label><HelpTooltip label={label}>{setting[1]}</HelpTooltip></div>
                <input id={inputID} {...register(key)} type="text" inputMode="decimal" readOnly={!!locked} aria-disabled={!!locked} aria-invalid={!!errors[key]}
                  aria-describedby={errors[key] ? `${inputID}-error` : locked ? `${inputID}-lock` : undefined}
                  className={`w-full rounded-lg border px-3 py-2.5 text-base focus:outline-none focus:ring-2 focus:ui-focus-ring ${locked ? 'cursor-not-allowed ui-border-subtle ui-surface-input ui-text-secondary' : errors[key] ? 'ui-border-danger ui-surface-card' : 'ui-border-default ui-surface-card ui-text-primary'}`} />
                {locked && <p id={`${inputID}-lock`} className="mt-1 flex items-center gap-1 text-sm ui-text-secondary"><LockKeyhole className="h-3 w-3" />{locked}</p>}
                {errors[key] && <p id={`${inputID}-error`} role="alert" className="mt-1 text-xs ui-text-danger">{errors[key].message}{locked ? ` ${t('costSettings.switchModes')}` : ''}</p>}
              </div>;
            })}
          </div>
          {title === 'Machine investment' && machineInvestmentExtra}
        </section>;
      })}
    </div>
  </>;
}
