import { AlertCircle, Loader2 } from 'lucide-react';
import { useTranslation } from 'react-i18next';

export function InlineError({ title, message, onRetry, children }) {
  const { t } = useTranslation();
  return (
    <div className="page-inline-error" role="alert">
      <AlertCircle className="mt-0.5 h-5 w-5 shrink-0" aria-hidden="true" />
      <div>
        <h2 className="font-semibold">{title || t('states.loadData')}</h2>
        <p className="mt-1 text-sm">{message || t('common.existingDataSafe')}</p>
        {children}
        {onRetry && <button type="button" onClick={onRetry} className="page-action page-action-secondary mt-3">{t('common.retry')}</button>}
      </div>
    </div>
  );
}

export function EmptyState({ icon: Icon, title, description, action, filtered = false }) {
  const { t } = useTranslation();
  return (
    <div className="page-empty" role="status">
      {Icon && <Icon className="mx-auto mb-3 h-10 w-10 ui-text-muted" aria-hidden="true" />}
      <h2 className="mb-2 text-lg font-medium ui-text-primary">{title}</h2>
      <p className="mb-4 text-sm ui-text-muted">{description}</p>
      {action}
      {filtered && <span className="sr-only">{t('states.filteredEmpty')}</span>}
    </div>
  );
}

export function TableSkeleton({ columns = 4, rows = 5, label }) {
  const { t } = useTranslation();
  const accessibleLabel = label || t('states.loadingData');
  return (
    <div className="overflow-x-auto" role="status" aria-live="polite" aria-label={accessibleLabel}>
      <table className="min-w-full" aria-hidden="true"><thead><tr>{Array.from({ length: columns }, (_, i) => <th key={i} className="px-6 py-3"><span className="skeleton-block h-3 w-20" /></th>)}</tr></thead>
        <tbody>{Array.from({ length: rows }, (_, r) => <tr key={r} className="border-t ui-border-subtle">{Array.from({ length: columns }, (_, c) => <td key={c} className="px-6 py-4"><span className={`skeleton-block h-4 ${c === 0 ? 'w-32' : 'w-20'}`} /></td>)}</tr>)}</tbody></table>
      <span className="sr-only">{accessibleLabel}</span>
    </div>
  );
}

export function LoadingState({ label }) {
  const { t } = useTranslation();
  return <div className="page-loading" role="status" aria-live="polite"><Loader2 className="h-6 w-6 animate-spin ui-text-primary-action" aria-hidden="true" /><span>{label || t('common.loading')}</span></div>;
}

export function StatusBadge({ children, tone = 'neutral', label }) {
  const { t } = useTranslation();
  return <span className={`status-badge status-badge-${tone}`} aria-label={label}>{children || t('states.incomplete')}</span>;
}

export function MetricValue({ children, label }) {
  return <p className="text-xl font-bold ui-text-primary font-mono" aria-label={label}>{children}</p>;
}
