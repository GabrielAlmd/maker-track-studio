import { useTranslation } from 'react-i18next';
import { materialStockStatus } from '../utils/productionEstimates';

export default function MaterialEstimateDetails({ row, rows, filaments, stock }) {
  const { t } = useTranslation();
  if (!row._sources) return null;
  const id = row.filament_id;
  const total = rows.filter(candidate => candidate.filament_id === id)
    .reduce((sum, candidate) => sum + (Number(candidate.weight_used) || 0), 0);
  const balance = stock.get(id);
  const status = id ? materialStockStatus(filaments.find(filament => filament.id === id), balance, total) : 'unmapped';
  return <div className="basis-full col-span-2 text-xs ui-text-muted">
    <ul>{row._sources.map(source => <li key={source.key}>
      {t(source.type ? 'materialEstimates.source' : 'materialEstimates.legacy', {
        ...source, material: [source.type, source.color].filter(Boolean).join(' / '),
      })}
    </li>)}</ul>
    <p>{t(`materialEstimates.${status}`, { grams: balance?.balance?.available / 1000 })}</p>
    {row.waste_weight_g === '' && <p>{t('materialEstimates.unknownWaste')}</p>}
  </div>;
}
