import { useState } from 'react';
import { useFieldArray, useForm } from 'react-hook-form';
import { yupResolver } from '@hookform/resolvers/yup';
import * as yup from 'yup';
import { Check, Loader2, Plus, Trash2 } from 'lucide-react';
import toast from 'react-hot-toast';
import HelpTooltip from '../components/ui/HelpTooltip';
import { updateAppConfig } from '../services/appConfigService';
import { COST_SETTINGS, normalizeCostValue, valueError } from './costSettings';
import { VALUE_FALLBACKS, formatCurrency } from '../utils/formatters';
import CostConfigFields from './CostConfigFields';
import { useTranslation } from 'react-i18next';

export default function CalculationSettings({ configs }) {
  const { t } = useTranslation();
  const editable = configs.filter(config => COST_SETTINGS[config.key] && config.key !== 'printer_upfront_cost');
  const initialExpenses = () => {
    const list = configs.find(row => row.key === 'printer_equipment_costs');
    if (list) return JSON.parse(list.value).map(row => ({ ...row, category: row.category || 'additional', amount: String(row.amount) }));
    const old = configs.find(row => row.key === 'printer_upfront_cost')?.value ?? '0';
    return Number(old) > 0 ? [{ description: t('calculation.previousCosts'), date: '', amount: old, category: 'additional' }] : [];
  };
  const [saved, setSaved] = useState(() => ({ ...Object.fromEntries(editable.map(({ key, value }) => [key, value])), expenses: initialExpenses() }));
  const schema = yup.object(Object.fromEntries(editable.map(({ key }) => [key,
    yup.string().test('cost', function (value) {
      const message = valueError(key, normalizeCostValue(value));
      return message ? this.createError({ message }) : true;
    }),
  ]))).shape({ expenses: yup.array().max(500, t('calculation.maxExpenses')).of(yup.object({
    description: yup.string().trim().required(t('calculation.descriptionRequired')).max(200, t('calculation.maxCharacters')),
    category: yup.string().oneOf(['spare_parts', 'additional', 'other']).required(),
    date: yup.string().test('date', t('calculation.validDate'), value => !value || (/^\d{4}-\d{2}-\d{2}$/.test(value) && !Number.isNaN(Date.parse(value)) && new Date(value).toISOString().slice(0,10) === value)),
    amount: yup.string().test('amount', function(value) {
      const message = valueError('printer_upfront_cost', normalizeCostValue(value));
      return message ? this.createError({ message }) : true;
    }),
  })).test('total', t('calculation.totalMaximum'), rows => rows.reduce((total, row) => total + Number(normalizeCostValue(row.amount)), 0) <= 1e9) });
  const { register, handleSubmit, watch, reset, control, formState: { errors, isSubmitting } } = useForm({
    defaultValues: saved, resolver: yupResolver(schema),
  });
  const { fields, append, remove } = useFieldArray({ control, name: 'expenses' });
  const values = watch();
  const expenseTotal = (values.expenses ?? []).reduce((total, row) => total + (normalizeCostValue(row.amount) === '' ? NaN : Number(normalizeCostValue(row.amount))), 0);
  const advanced = values.machine_cost_mode === '1';
  const dirty = editable.some(({ key }) => values[key] !== saved[key]) || JSON.stringify(values.expenses) !== JSON.stringify(saved.expenses);
  const [saveError, setSaveError] = useState('');

  const save = async data => {
    setSaveError('');
    // Existing API saves one key at a time. Save the mode last and retain a
    // baseline after each success so a failure can be retried without data loss.
    const changed = editable.filter(({ key }) => normalizeCostValue(data[key]) !== normalizeCostValue(saved[key]))
      .sort((a, b) => Number(a.key === 'machine_cost_mode') - Number(b.key === 'machine_cost_mode'));
    const persisted = { ...saved };
    try {
      if (JSON.stringify(data.expenses) !== JSON.stringify(saved.expenses)) {
        const expenses = data.expenses.map(row => ({ description: row.description.trim(), date: row.date || '', amount: Number(normalizeCostValue(row.amount)), category: row.category }));
        await updateAppConfig('printer_equipment_costs', JSON.stringify(expenses), 'json');
        persisted.expenses = expenses.map(row => ({ ...row, amount: String(row.amount) }));
        setSaved({ ...persisted });
      }
      for (const { key, type } of changed) {
        await updateAppConfig(key, normalizeCostValue(data[key]), type);
        persisted[key] = normalizeCostValue(data[key]);
        setSaved({ ...persisted });
      }
      reset(persisted);
      toast.success(t('calculation.saved'));
    } catch {
      setSaveError(t('calculation.saveError'));
    }
  };

  if (!editable.length) return <div className="page-panel p-6 ui-text-muted">{t('calculation.empty')}</div>;

  return <section className="page-panel" aria-labelledby="calculation-settings-title">
    <div className="page-panel-header">
      <h2 id="calculation-settings-title" className="text-lg font-semibold">{t('calculation.title')}</h2>
      <p className="mt-1 text-sm ui-text-secondary">{t('calculation.help')}</p>
    </div>
    <form onSubmit={handleSubmit(save)} noValidate>
      <fieldset disabled={isSubmitting} className="min-w-0 p-4 sm:p-6">
        <CostConfigFields values={{ ...values, printer_upfront_cost: Number.isFinite(expenseTotal) ? String(expenseTotal) : '' }} availableKeys={editable.map(({ key }) => key)} register={register} errors={errors} dirty={dirty} idPrefix="setting" machineInvestmentExtra={<div className="mt-6 border-t ui-border-subtle pt-4">
                  <div className="flex items-center gap-2">
                    <h4 className="text-base font-semibold ui-text-primary">{t('calculation.equipment')}</h4>
                    <HelpTooltip label={t('calculation.equipment')}>{t('calculation.equipmentHelp')}</HelpTooltip>
                  </div>
                  {!advanced && <p className="mt-1 text-sm ui-text-secondary">{t('costSettings.availableAdvanced')}</p>}
                  {fields.length === 0 && <p className="mt-3 text-sm ui-text-secondary">{t('calculation.noExpenses')}</p>}
                  <div className="mt-3 space-y-3">
                    {fields.map((field, index) => <div key={field.id} className="grid items-start gap-3 rounded-lg border ui-border-subtle p-3 sm:grid-cols-2 xl:grid-cols-[2fr_1.25fr_1fr_1fr_auto]">
                      <div>
                        <label className="mb-1 block text-sm font-medium ui-text-primary" htmlFor={`expense-${field.id}-category`}>{t('calculation.expenseCategory')}</label>
                        <select id={`expense-${field.id}-category`} aria-label={`${t('calculation.expenseCategory')} ${index + 1}`} {...register(`expenses.${index}.category`)} aria-disabled={!advanced} tabIndex={advanced ? 0 : -1}
                          onPointerDown={event => { if (!advanced) event.preventDefault(); }} onKeyDown={event => { if (!advanced) event.preventDefault(); }}
                          className={`w-full rounded-lg border ui-border-default ui-surface-card px-3 py-2 text-base ui-text-primary ${!advanced ? 'pointer-events-none cursor-not-allowed' : ''}`}>
                          <option value="spare_parts">{t('calculation.categories.spareParts')}</option>
                          <option value="additional">{t('calculation.categories.additional')}</option>
                          <option value="other">{t('calculation.categories.other')}</option>
                        </select>
                      </div>
                      {[['description', t('calculation.expenseDescription'), 'text'], ['date', t('calculation.dateOptional'), 'date'], ['amount', t('calculation.amount'), 'text']].map(([key, label, type]) => <div key={key}>
                        <label className="mb-1 block text-sm font-medium ui-text-primary" htmlFor={`expense-${field.id}-${key}`}>{label}</label>
                        <input id={`expense-${field.id}-${key}`} aria-label={`${label} ${index + 1}`} type={type} inputMode={key === 'amount' ? 'decimal' : undefined}
                          {...register(`expenses.${index}.${key}`)} readOnly={!advanced} aria-disabled={!advanced}
                          aria-invalid={!!errors.expenses?.[index]?.[key]} aria-describedby={errors.expenses?.[index]?.[key] ? `expense-error-${field.id}-${key}` : undefined}
                          className="w-full rounded-lg border ui-border-default ui-surface-card px-3 py-2 text-base ui-text-primary read-only:cursor-not-allowed" />
                        {errors.expenses?.[index]?.[key] && <p id={`expense-error-${field.id}-${key}`} role="alert" className="mt-1 text-sm ui-text-danger">{errors.expenses[index][key].message}</p>}
                      </div>)}
                      <button type="button" disabled={!advanced} onClick={() => remove(index)} aria-label={t('calculation.removeExpense', { number: index + 1 })} className="page-action page-action-secondary sm:mt-6"><Trash2 className="h-4 w-4" /></button>
                    </div>)}
                  </div>
                  {errors.expenses?.message && <p role="alert" className="mt-2 text-sm ui-text-danger">{errors.expenses.message}</p>}
                  {errors.expenses?.root?.message && <p role="alert" className="mt-2 text-sm ui-text-danger">{errors.expenses.root.message}</p>}
                  <div className="mt-4 flex flex-wrap items-center justify-between gap-3">
                    <button type="button" disabled={!advanced || fields.length >= 500} onClick={() => append({ description: '', date: '', amount: '', category: 'additional' })} className="page-action page-action-secondary"><Plus className="h-4 w-4" />{t('calculation.addExpense')}</button>
                    <p className="font-semibold ui-text-primary">{t('calculation.equipmentTotal')}: {formatCurrency(expenseTotal, { fallback: VALUE_FALLBACKS.notCalculated })}</p>
                  </div>
                </div>} />
      </fieldset>
      <div className="sticky bottom-0 flex flex-wrap items-center justify-between gap-3 border-t ui-border-subtle ui-surface-card p-4 sm:px-6">
        <div>
          <p className="text-sm ui-text-secondary" role="status">{t(isSubmitting ? 'calculation.saving' : dirty ? 'calculation.unsaved' : 'calculation.allSaved')}</p>
          {saveError && <p role="alert" className="mt-1 max-w-xl text-sm ui-text-danger">{saveError}</p>}
        </div>
        <div className="flex gap-2">
          <button type="button" disabled={!dirty || isSubmitting} onClick={() => { reset(saved); setSaveError(''); }} className="page-action page-action-secondary disabled:opacity-50">{t('calculation.discard')}</button>
          <button type="submit" disabled={!dirty || isSubmitting} className="page-action disabled:opacity-50">{isSubmitting ? <Loader2 className="h-4 w-4 animate-spin" /> : <Check className="h-4 w-4" />}{t('common.saveChanges')}</button>
        </div>
      </div>
    </form>
  </section>;
}
