import { useEffect, useMemo, useRef, useState } from 'react';
import { AlertTriangle, CheckCircle2, Download, FileSpreadsheet, Loader2, Upload } from 'lucide-react';
import toast from 'react-hot-toast';
import { useTranslation } from 'react-i18next';
import {
  downloadMigrationTemplate,
  downloadMigrationWorkbookTemplate,
  getMigrationSchema,
  importMigration,
  inspectMigration,
  previewMigration,
} from '../services/migrationService';

function normalized(value) {
  return value.trim().toLocaleLowerCase().replace(/[^\p{L}\p{N}]+/gu, '_').replace(/^_+|_+$/g, '');
}

function automaticMapping(fields, columns) {
  const byName = new Map(columns.map((column) => [normalized(column), column]));
  return Object.fromEntries(fields.flatMap((field) => {
    const match = [field.key, field.label, ...(field.aliases ?? [])]
      .map((candidate) => byName.get(normalized(candidate)))
      .find(Boolean);
    return match ? [[field.key, match]] : [];
  }));
}

function initialPlans(schema, selected, inspection) {
  return selected.map((sectionKey, index) => {
    const definition = schema.sections.find((section) => section.key === sectionKey);
    const named = inspection.sources.find((source) => normalized(source.name) === normalized(sectionKey)
      || normalized(source.name) === normalized(definition.label));
    const source = named ?? inspection.sources[Math.min(index, inspection.sources.length - 1)];
    return {
      section: sectionKey,
      source: source.name,
      mapping: automaticMapping(definition.fields, source.columns),
      defaults: {},
      duplicate_strategy: 'skip',
    };
  });
}

function statusTone(status) {
  if (status === 'ready') return 'ui-text-success';
  if (status === 'invalid') return 'ui-text-danger';
  return 'ui-text-warning';
}

export default function DataMigrationSection() {
  const { t } = useTranslation();
  const [schema, setSchema] = useState(null);
  const [selected, setSelected] = useState(['printers']);
  const [file, setFile] = useState(null);
  const [inspection, setInspection] = useState(null);
  const [plans, setPlans] = useState([]);
  const [preview, setPreview] = useState(null);
  const [confirmed, setConfirmed] = useState(false);
  const [busy, setBusy] = useState('');
  const [error, setError] = useState('');
  const fileInput = useRef(null);

  useEffect(() => {
    let active = true;
    getMigrationSchema()
      .then((result) => { if (active) setSchema(result); })
      .catch((err) => { if (active) setError(err?.message || t('migration.schemaError')); });
    return () => { active = false; };
  }, [t]);

  const sectionByKey = useMemo(() => new Map((schema?.sections ?? []).map((section) => [section.key, section])), [schema]);
  const fieldName = (sectionKey, key) => {
    if (!key) return '';
    const fallback = sectionByKey.get(sectionKey)?.fields.find((field) => field.key === key)?.label ?? key;
    return t(`migration.fields.${key}`, fallback);
  };

  const resetAfterSource = () => {
    setInspection(null);
    setPlans([]);
    setPreview(null);
    setConfirmed(false);
    setError('');
  };

  const toggleSection = (key) => {
    setSelected((current) => current.includes(key) ? current.filter((item) => item !== key) : [...current, key]);
    resetAfterSource();
  };

  const inspect = async () => {
    if (!file || selected.length === 0) return;
    setBusy('inspect');
    setError('');
    try {
      const result = await inspectMigration(file);
      if (result.format === 'csv' && selected.length > 1) {
        setError(t('migration.csvSingleSection'));
        return;
      }
      setInspection(result);
      setPlans(initialPlans(schema, selected, result));
      setPreview(null);
    } catch (err) {
      setError(err?.response?.data?.error || err?.message || t('migration.inspectError'));
    } finally {
      setBusy('');
    }
  };

  const updatePlan = (section, updater) => {
    setPlans((current) => current.map((plan) => plan.section === section ? updater(plan) : plan));
    setPreview(null);
    setConfirmed(false);
  };

  const changeSource = (plan, sourceName) => {
    const definition = sectionByKey.get(plan.section);
    const source = inspection.sources.find((item) => item.name === sourceName);
    updatePlan(plan.section, (current) => ({ ...current, source: sourceName, mapping: automaticMapping(definition.fields, source.columns) }));
  };

  const validate = async () => {
    setBusy('preview');
    setError('');
    try {
      setPreview(await previewMigration(file, { sections: plans }));
      setConfirmed(false);
    } catch (err) {
      setError(err?.response?.data?.error || err?.message || t('migration.previewError'));
    } finally {
      setBusy('');
    }
  };

  const commit = async () => {
    setBusy('import');
    setError('');
    try {
      await importMigration(file, { sections: plans });
      toast.success(t('migration.imported'));
      setFile(null);
      if (fileInput.current) fileInput.current.value = '';
      setInspection(null);
      setPlans([]);
      setPreview(null);
      setConfirmed(false);
    } catch (err) {
      setError(err?.response?.data?.error || err?.message || t('migration.importError'));
    } finally {
      setBusy('');
    }
  };

  if (!schema) {
    return <div className="page-panel"><div className="flex items-center gap-2 p-6 ui-text-muted" role="status"><Loader2 className="h-4 w-4 animate-spin" />{t('migration.loading')}</div>{error && <p className="page-form-error mx-6 mb-6" role="alert">{error}</p>}</div>;
  }

  return <div className="space-y-6">
    <section className="page-panel" aria-labelledby="migration-source-title">
      <div className="page-panel-header"><h2 id="migration-source-title" className="text-lg font-semibold ui-text-primary">{t('migration.title')}</h2><p className="mt-1 text-sm ui-text-muted">{t('migration.help')}</p></div>
      <div className="space-y-6 p-6">
        {error && <p className="page-form-error" role="alert">{error}</p>}
        <fieldset>
          <legend className="text-sm font-semibold ui-text-primary">{t('migration.chooseSections')}</legend>
          <p className="mt-1 text-sm ui-text-muted">{t('migration.chooseSectionsHelp')}</p>
          <p className="mt-2 text-sm ui-text-secondary">{t('migration.importOrderTip')}</p>
          <div className="mt-3 grid gap-2 sm:grid-cols-2 lg:grid-cols-3">
            {schema.sections.map((section) => <label key={section.key} className="flex items-center gap-3 rounded-lg border ui-border-default ui-surface-card px-4 py-3 text-sm ui-text-primary">
              <input type="checkbox" checked={selected.includes(section.key)} onChange={() => toggleSection(section.key)} />
              {t(`migration.sections.${section.key}`, section.label)}
            </label>)}
          </div>
          {selected.includes('purchases') && <aside className="mt-4 rounded-lg border ui-border-default ui-surface-nested p-4 text-sm ui-text-secondary" aria-label={t('migration.purchaseTipTitle')}>
            <p className="font-semibold ui-text-primary">{t('migration.purchaseTipTitle')}</p>
            <p className="mt-1">{t('migration.purchaseTip')}</p>
            <p className="mt-2 font-mono text-xs ui-text-primary">{t('migration.purchaseTipExample')}</p>
          </aside>}
        </fieldset>
        <div>
          <h3 className="text-sm font-semibold ui-text-primary">{t('migration.templates')}</h3>
          <p className="mt-1 text-sm ui-text-muted">{t('migration.templatesHelp')}</p>
          <div className="mt-3 flex flex-wrap gap-2"><button type="button" className="page-action page-action-secondary" onClick={() => downloadMigrationWorkbookTemplate().catch(() => toast.error(t('migration.templateError')))}><Download className="h-4 w-4" />{t('migration.workbookTemplate')}</button>{schema.sections.map((section) => <button key={section.key} type="button" className="page-action page-action-secondary" onClick={() => downloadMigrationTemplate(section.key)}><Download className="h-4 w-4" />{t(`migration.sections.${section.key}`, section.label)}</button>)}</div>
        </div>
        <div>
          <label htmlFor="migration-file" className="mb-2 block text-sm font-semibold ui-text-primary">{t('migration.sourceFile')}</label>
          <input ref={fileInput} id="migration-file" type="file" accept=".csv,.xlsx,text/csv,application/vnd.openxmlformats-officedocument.spreadsheetml.sheet" onChange={(event) => { setFile(event.target.files?.[0] || null); resetAfterSource(); }} className="block w-full text-sm ui-text-secondary file:mr-4 file:rounded-md file:border-0 file:ui-file-inverse file:px-3 file:py-2 file:text-sm file:font-medium file:ui-file-text hover:file:ui-file-hover" />
          <p className="mt-2 text-xs ui-text-muted">{t('migration.fileHelp')}</p>
        </div>
        <button type="button" className="page-action" disabled={!file || selected.length === 0 || busy !== ''} onClick={inspect}>{busy === 'inspect' ? <Loader2 className="h-4 w-4 animate-spin" /> : <FileSpreadsheet className="h-4 w-4" />}{t(busy === 'inspect' ? 'migration.inspecting' : 'migration.inspect')}</button>
      </div>
    </section>

    {inspection && <section className="page-panel" aria-labelledby="migration-mapping-title">
      <div className="page-panel-header"><h2 id="migration-mapping-title" className="text-lg font-semibold ui-text-primary">{t('migration.mappingTitle')}</h2><p className="mt-1 text-sm ui-text-muted">{t('migration.mappingHelp')}</p></div>
      <div className="space-y-8 p-6">{plans.map((plan) => {
        const definition = sectionByKey.get(plan.section);
        const source = inspection.sources.find((item) => item.name === plan.source);
        return <fieldset key={plan.section} className="rounded-lg border ui-border-default p-4">
          <legend className="px-2 text-sm font-semibold ui-text-primary">{t(`migration.sections.${plan.section}`, definition.label)}</legend>
          <div className="grid gap-4 md:grid-cols-2">
            <label className="text-sm font-medium ui-text-primary">{t('migration.worksheet')}<select aria-label={`${t(`migration.sections.${plan.section}`, definition.label)} ${t('migration.worksheet')}`} value={plan.source} onChange={(event) => changeSource(plan, event.target.value)} className="mt-1 block w-full rounded-lg border ui-border-default ui-surface-card px-3 py-2">{inspection.sources.map((item) => <option key={item.name} value={item.name}>{item.name} ({t('migration.rows', { count: item.row_count })})</option>)}</select></label>
            <label className="text-sm font-medium ui-text-primary">{t('migration.duplicates')}<select aria-label={`${t(`migration.sections.${plan.section}`, definition.label)} ${t('migration.duplicates')}`} value={plan.duplicate_strategy} onChange={(event) => updatePlan(plan.section, (current) => ({ ...current, duplicate_strategy: event.target.value }))} className="mt-1 block w-full rounded-lg border ui-border-default ui-surface-card px-3 py-2"><option value="skip">{t('migration.skip')}</option><option value="update">{t('migration.update')}</option><option value="create">{t('migration.create')}</option></select></label>
          </div>
          <div className="mt-4 overflow-x-auto"><table className="min-w-full text-sm"><thead><tr className="text-left ui-text-muted"><th className="px-2 py-2">{t('migration.destinationField')}</th><th className="px-2 py-2">{t('migration.sourceColumn')}</th><th className="px-2 py-2">{t('migration.defaultValue')}</th></tr></thead><tbody>{definition.fields.map((field) => {
            const label = fieldName(plan.section, field.key);
            return <tr key={field.key} className="border-t ui-border-subtle"><th scope="row" className="px-2 py-2 text-left font-medium ui-text-primary">{label}{field.required && <span className="ui-text-danger" aria-label={t('migration.required')}> *</span>}</th><td className="px-2 py-2"><select aria-label={`${label} ${t('migration.sourceColumn')}`} value={plan.mapping[field.key] || ''} onChange={(event) => updatePlan(plan.section, (current) => ({ ...current, mapping: { ...current.mapping, [field.key]: event.target.value } }))} className="w-full min-w-44 rounded-lg border ui-border-default ui-surface-card px-2 py-1.5"><option value="">{t('migration.notMapped')}</option>{source.columns.map((column) => <option key={column} value={column}>{column}</option>)}</select></td><td className="px-2 py-2"><input aria-label={`${label} ${t('migration.defaultValue')}`} value={plan.defaults[field.key] || ''} onChange={(event) => updatePlan(plan.section, (current) => ({ ...current, defaults: { ...current.defaults, [field.key]: event.target.value } }))} placeholder={field.example} className="w-full min-w-44 rounded-lg border ui-border-default ui-surface-card px-2 py-1.5 ui-text-primary" /></td></tr>;
          })}</tbody></table></div>
        </fieldset>;
      })}
        <button type="button" className="page-action" disabled={busy !== ''} onClick={validate}>{busy === 'preview' ? <Loader2 className="h-4 w-4 animate-spin" /> : <CheckCircle2 className="h-4 w-4" />}{t(busy === 'preview' ? 'migration.validating' : 'migration.validate')}</button>
      </div>
    </section>}

    {preview && <section className="page-panel" aria-labelledby="migration-preview-title">
      <div className="page-panel-header"><h2 id="migration-preview-title" className="text-lg font-semibold ui-text-primary">{t('migration.previewTitle')}</h2><p className="mt-1 text-sm ui-text-muted">{t(preview.can_import ? 'migration.previewReady' : 'migration.previewBlocked')}</p></div>
      <div className="space-y-6 p-6">{preview.sections.map((section) => <div key={section.section}>
        <h3 className="font-semibold ui-text-primary">{t(`migration.sections.${section.section}`)}</h3>
        <p className="mt-1 text-sm ui-text-muted">{t('migration.summary', section.summary)}</p>
        <div className="mt-3 max-h-80 overflow-auto rounded-lg border ui-border-default"><table className="min-w-full text-sm"><thead className="sticky top-0 ui-surface-nested"><tr><th className="px-3 py-2 text-left">{t('migration.row')}</th><th className="px-3 py-2 text-left">{t('common.status')}</th><th className="px-3 py-2 text-left">{t('migration.result')}</th></tr></thead><tbody>{section.rows.map((row) => <tr key={row.row} className="border-t ui-border-subtle"><td className="px-3 py-2 ui-text-secondary">{row.row}</td><td className={`px-3 py-2 font-medium ${statusTone(row.status)}`}>{t(`migration.status.${row.status}`)}</td><td className="px-3 py-2 ui-text-secondary">{row.issues.length > 0 ? row.issues.map((issue) => t(`migration.errors.${issue.code}`, { field: fieldName(section.section, issue.field), defaultValue: issue.message })).join(' · ') : t(row.action === 'skip' ? 'migration.willSkip' : 'migration.ready')}</td></tr>)}</tbody></table></div>
      </div>)}
        {preview.can_import ? <div className="rounded-lg border ui-border-default ui-surface-nested p-4"><label className="flex items-start gap-3 text-sm ui-text-primary"><input type="checkbox" checked={confirmed} onChange={(event) => setConfirmed(event.target.checked)} className="mt-0.5" /><span><strong>{t('migration.confirmTitle')}</strong><span className="mt-1 block ui-text-muted">{t('migration.confirmHelp')}</span></span></label><button type="button" className="page-action mt-4" disabled={!confirmed || busy !== ''} onClick={commit}>{busy === 'import' ? <Loader2 className="h-4 w-4 animate-spin" /> : <Upload className="h-4 w-4" />}{t(busy === 'import' ? 'migration.importing' : 'migration.import')}</button></div> : <div className="flex items-start gap-3 rounded-lg border ui-border-danger p-4 ui-text-danger"><AlertTriangle className="mt-0.5 h-5 w-5 shrink-0" /><p className="text-sm">{t('migration.fixErrors')}</p></div>}
      </div>
    </section>}
  </div>;
}
