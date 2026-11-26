import { useTranslation } from 'react-i18next';
import { formatNumber } from '../utils/formatters';
import { useEstimateStock } from '../hooks/useEstimateStock';
import { AlertTriangle, ChevronRight } from 'lucide-react';
import { StatusBadge } from './ui/PageState';
import { stockWarning } from '../services/inventoryService';

export default function StockWarnings({ requirements }) {
  const inventory = useEstimateStock();
  return <StockWarningList requirements={requirements} inventory={inventory} />;
}

export function StockWarningList({ requirements, inventory: { rows: items, failed } }) {
  const { t } = useTranslation();
  if (failed) return <p role="status">{t('inventory.warnings.unavailable')} <a className="stock-warnings-link" href="#/inventory">{t('inventory.warnings.review')}</a></p>;
  if (!items) return <p role="status">{t('inventory.warnings.loading')}</p>;
  const required = requirements && new Map();
  for (const { filament_id, weight_used } of requirements ?? []) {
    required.set(filament_id, (required.get(filament_id) ?? 0) + Math.round(Number(weight_used || 0) * 1000));
  }
  const warnings = items.filter((item) => !required || required.has(item.id)).map((item) => ({ item, message: stockWarning(item, required?.get(item.id) ?? null) })).filter((row) => row.message);
  if (!warnings.length) return null;
  return <section className={`dashboard-secondary-panel stock-warnings${required ? ' stock-warnings-compact' : ''}`} aria-label={t('inventory.warnings.title')}>
    <div className="dashboard-panel-header">
      <h2 className="flex items-center gap-2"><AlertTriangle className="h-4 w-4 ui-text-warning" aria-hidden="true" />{t('inventory.warnings.title')}</h2>
      <a className="stock-warnings-link" href="#/inventory">{t('inventory.warnings.review')}<ChevronRight className="h-4 w-4" aria-hidden="true" /></a>
    </div>
    <ul className="stock-warnings-list">{warnings.map(({ item, message }) => <li key={item.id}>
      <div className="min-w-0"><p className="stock-warnings-material">{item.brand} {item.type} — {item.color}</p><p className="stock-warnings-detail">{t(`inventory.warnings.${message}`)}</p></div>
      <StatusBadge tone={item.balance.verified ? 'warning' : 'neutral'}>{item.balance.verified ? t('inventory.warnings.available', { weight: formatNumber(item.balance.available / 1000, { maximumFractionDigits: 3 }) }) : t('inventory.unverified')}</StatusBadge>
    </li>)}</ul>
    {required && <p className="stock-warnings-note">{t('inventory.warnings.note')}</p>}
  </section>;
}
