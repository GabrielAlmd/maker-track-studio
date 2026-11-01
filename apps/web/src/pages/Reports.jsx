import { useState } from 'react';
import { Download, FileArchive } from 'lucide-react';
import { useForm } from 'react-hook-form';
import { yupResolver } from '@hookform/resolvers/yup';
import * as yup from 'yup';
import { useTranslation } from 'react-i18next';
import toast from 'react-hot-toast';
import PageHeader from '../components/ui/PageHeader';
import { exportReport } from '../services/reportService';

const categories = ['print_jobs', 'materials', 'purchases', 'sales', 'printers', 'events'];
const filesByCategory = {
  print_jobs: ['models.csv', 'print_jobs.csv'],
  materials: ['filaments.csv', 'print_job_filaments.csv'],
  purchases: ['filament_purchases.csv', 'equipment_expenses.csv'],
  sales: ['sales_channels.csv', 'sales_orders.csv', 'sales_order_items.csv', 'sales_order_allocations.csv'],
  printers: ['printers.csv'],
  events: ['application_events.csv'],
};
const fullFiles = ['printers.csv', 'filaments.csv', 'filament_purchases.csv', 'models.csv', 'print_jobs.csv', 'print_job_filaments.csv', 'sales_channels.csv', 'sales_orders.csv', 'sales_order_items.csv', 'sales_order_allocations.csv', 'app_config.csv', 'equipment_expenses.csv'];
const today = new Date();
const currentYear = today.getFullYear();
const currentMonth = `${currentYear}-${String(today.getMonth() + 1).padStart(2, '0')}`;

const schema = yup.object({
  mode: yup.string().oneOf(['full', 'report']).required(),
  format: yup.string().oneOf(['zip', 'pdf']).required(),
  period: yup.string().oneOf(['month', 'quarter', 'year', 'custom']).required(),
  month: yup.string().matches(/^\d{4}-(0[1-9]|1[0-2])$/).required(),
  year: yup.number().integer().min(1900).max(9999).required(),
  quarter: yup.number().integer().min(1).max(4).required(),
  start: yup.string().default(''),
  end: yup.string().default(''),
  categories: yup.array().of(yup.string().oneOf(categories)).when('mode', {
    is: 'report', then: (field) => field.min(1), otherwise: (field) => field,
  }),
}).test('custom-range', 'invalid', function validRange(values) {
  if (values?.mode !== 'report' || values.period !== 'custom') return true;
  const validDate = (value) => {
    if (!/^\d{4}-\d{2}-\d{2}$/.test(value)) return false;
    const parsed = new Date(`${value}T00:00:00Z`);
    return !Number.isNaN(parsed.getTime()) && parsed.toISOString().slice(0, 10) === value;
  };
  return (validDate(values.start) && validDate(values.end) && values.start <= values.end) || this.createError({ path: 'end' });
});

function reportParams(values, language) {
  if (values.mode === 'full') return { mode: 'full' };
  const date = values.period === 'month' ? `${values.month}-01`
    : values.period === 'quarter' ? `${values.year}-${String((Number(values.quarter) - 1) * 3 + 1).padStart(2, '0')}-01`
      : values.period === 'year' ? `${values.year}-01-01` : values.start;
  return { mode: 'report', period: values.period, date, ...(values.period === 'custom' ? { end: values.end } : {}), categories: values.categories.join(','), ...(values.format === 'pdf' ? { format: 'pdf', language } : {}) };
}

export default function Reports() {
  const { t, i18n } = useTranslation();
  const [result, setResult] = useState(null);
  const [failure, setFailure] = useState('');
  const { register, handleSubmit, watch, formState: { errors, isSubmitting } } = useForm({
    resolver: yupResolver(schema),
    defaultValues: { mode: 'full', format: 'zip', period: 'month', month: currentMonth, year: currentYear, quarter: Math.floor(today.getMonth() / 3) + 1, start: '', end: '', categories: categories.filter((category) => category !== 'events') },
  });
  const mode = watch('mode');
  const period = watch('period');
  const format = watch('format');
  const selected = watch('categories') || [];
  const submit = async (values) => {
    setFailure('');
    setResult(null);
    try {
      const outcome = await exportReport(reportParams(values, i18n.resolvedLanguage === 'pt-PT' ? 'pt-PT' : 'en'));
      const range = values.period === 'month' ? values.month : values.period === 'quarter' ? `Q${values.quarter} ${values.year}` : values.period === 'year' ? String(values.year) : `${values.start} – ${values.end}`;
      if (!outcome.cancelled) setResult({ path: outcome.path, at: new Date().toLocaleString(), mode: values.mode, period: values.period, range, categories: values.categories,
        files: values.format === 'pdf' && values.mode === 'report' ? ['report.pdf'] : values.mode === 'full' ? fullFiles : [...new Set(values.categories.flatMap((category) => filesByCategory[category])), 'summary.csv', 'trends.csv'] });
    } catch {
      setFailure(t('reports.failed'));
      toast.error(t('reports.failed'));
    }
  };

  return <div className="space-y-6">
    <PageHeader title={t('reports.title')} icon={FileArchive} description={t('reports.description')} />
    <form onSubmit={handleSubmit(submit)} className="page-panel p-6 space-y-6">
      <fieldset><legend className="font-semibold ui-text-primary">{t('reports.exportType')}</legend>
        <div className="mt-3 flex flex-wrap gap-6"><label className="flex items-center gap-2"><input type="radio" value="full" {...register('mode')} />{t('reports.full')}</label><label className="flex items-center gap-2"><input type="radio" value="report" {...register('mode')} />{t('reports.periodReport')}</label></div>
        <p className="mt-2 text-sm ui-text-muted">{t(mode === 'full' ? 'reports.fullHelp' : 'reports.reportHelp')}</p>
      </fieldset>
      {mode === 'report' && <>
        <fieldset><legend className="font-semibold ui-text-primary">{t('reports.format')}</legend><div className="mt-3 flex flex-wrap gap-6"><label className="flex items-center gap-2"><input type="radio" value="zip" {...register('format')} />{t('reports.csvZip')}</label><label className="flex items-center gap-2"><input type="radio" value="pdf" {...register('format')} />{t('reports.pdf')}</label></div><p className="mt-2 text-sm ui-text-muted">{t(format === 'pdf' ? 'reports.pdfHelp' : 'reports.zipHelp')}</p></fieldset>
        <div><label className="block font-medium ui-text-primary" htmlFor="report-period">{t('reports.period')}</label><select id="report-period" {...register('period')} className="mt-2 rounded border ui-border-default ui-surface-card p-2"><option value="month">{t('reports.month')}</option><option value="quarter">{t('reports.quarter')}</option><option value="year">{t('reports.year')}</option><option value="custom">{t('reports.custom')}</option></select></div>
        <div className="flex flex-wrap gap-4">
          {period === 'month' && <label className="block">{t('reports.month')}<input type="month" {...register('month')} className="mt-2 block rounded border ui-border-default ui-surface-card p-2" /></label>}
          {(period === 'quarter' || period === 'year') && <label className="block">{t('reports.year')}<input type="number" min="1900" max="9999" step="1" {...register('year')} className="mt-2 block w-28 rounded border ui-border-default ui-surface-card p-2" /></label>}
          {period === 'quarter' && <label className="block">{t('reports.quarter')}<select {...register('quarter')} className="mt-2 block rounded border ui-border-default ui-surface-card p-2">{[1, 2, 3, 4].map((value) => <option key={value} value={value}>Q{value}</option>)}</select></label>}
          {period === 'custom' && <><label className="block">{t('reports.from')}<input type="date" {...register('start')} className="mt-2 block rounded border ui-border-default ui-surface-card p-2" /></label><label className="block">{t('reports.to')}<input type="date" {...register('end')} className="mt-2 block rounded border ui-border-default ui-surface-card p-2" /></label></>}
        </div>
        <fieldset><legend className="font-semibold ui-text-primary">{t('reports.categories')}</legend><div className="mt-3 grid gap-3 sm:grid-cols-2 lg:grid-cols-3">{categories.map((category) => <label key={category} className="flex items-center gap-2"><input type="checkbox" value={category} {...register('categories')} />{t(`reports.category.${category}`)}</label>)}</div>{!selected.length && <p className="mt-2 text-sm ui-text-danger">{t('reports.selectCategory')}</p>}</fieldset>
        <p className="text-sm ui-text-muted">{t('reports.dataLimits')}</p>
      </>}
      {(errors.root || errors.start || errors.end || errors.month || errors.year || errors.categories) && <p role="alert" className="page-form-error">{t('reports.invalid')}</p>}
      {failure && <p role="alert" className="page-form-error">{failure}</p>}
      <button type="submit" disabled={isSubmitting} className="page-action"><Download className="h-4 w-4" />{t(isSubmitting ? 'reports.generating' : format === 'pdf' && mode === 'report' ? 'reports.generatePDF' : 'reports.generate')}</button>
      {result && <div role="status" className="ui-tone-success rounded-lg p-4 text-sm"><p>{t('reports.saved', { path: result.path })}</p><p>{t('reports.generatedAt', { date: result.at })}</p><p>{result.mode === 'full' ? t('reports.full') : `${t('reports.periodReport')} · ${t(`reports.${result.period}`)} ${result.range} · ${result.categories.map((item) => t(`reports.category.${item}`)).join(', ')}`}</p><p className="mt-2 font-medium">{t('reports.includedFiles')}</p><p className="break-all">{result.files.join(' · ')}</p></div>}
    </form>
  </div>;
}
