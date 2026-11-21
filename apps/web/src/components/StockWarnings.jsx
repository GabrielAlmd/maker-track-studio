import { useEffect, useState } from 'react';
import { AlertTriangle, ChevronRight } from 'lucide-react';
import { StatusBadge } from './ui/PageState';
import { getMaterials, stockWarning } from '../services/inventoryService';

export default function StockWarnings({ requirements }) {
  const [items, setItems] = useState(null), [failed, setFailed] = useState(false);
  useEffect(() => { let active = true; getMaterials().then((rows) => { if (active) setItems(rows); }).catch(() => { if (active) setFailed(true); }); return () => { active = false; }; }, []);
  if (failed) return <p role="status">Stock warnings unavailable. <a href="#/inventory">Review inventory</a></p>;
  if (!items) return <p role="status">Loading stock warnings…</p>;
  const required = requirements && requirements.reduce((totals, row) => { totals[row.filament_id] = (totals[row.filament_id] || 0) + Math.round(Number(row.weight_used || 0) * 1000); return totals; }, {});
  const warnings = items.filter((item) => !required || item.id in required).map((item) => ({ item, message: stockWarning(item, required ? required[item.id] : null) })).filter((row) => row.message);
  if (!warnings.length) return null;
  return <section className={`dashboard-secondary-panel stock-warnings${required ? ' stock-warnings-compact' : ''}`} aria-label="Stock warnings">
    <div className="dashboard-panel-header">
      <h2 className="flex items-center gap-2"><AlertTriangle className="h-4 w-4 ui-text-warning" aria-hidden="true" />Stock warnings</h2>
      <a className="stock-warnings-link" href="#/inventory">Review inventory<ChevronRight className="h-4 w-4" aria-hidden="true" /></a>
    </div>
    <ul className="stock-warnings-list">{warnings.map(({ item, message }) => <li key={item.id}>
      <div className="min-w-0"><p className="stock-warnings-material">{item.brand} {item.type} — {item.color}</p><p className="stock-warnings-detail">{message}</p></div>
      <StatusBadge tone={item.balance.verified ? 'warning' : 'neutral'}>{item.balance.verified ? `${item.balance.available / 1000} g available` : 'Unverified'}</StatusBadge>
    </li>)}</ul>
    {required && <p className="stock-warnings-note">Warnings do not block planning. Estimates do not deduct physical stock.</p>}
  </section>;
}
