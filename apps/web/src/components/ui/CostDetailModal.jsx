import { X } from 'lucide-react';
import { VALUE_FALLBACKS, formatCurrency } from '../../utils/formatters';
import { useTranslation } from 'react-i18next';

function CostRow({ label, value }) {
  return <div className="flex justify-between text-sm"><span className="ui-text-secondary">{label}</span><span className="ui-text-primary font-medium">{value}</span></div>;
}

export default function CostDetailModal({ cost, onClose }) {
  const { t } = useTranslation();
  if (!cost) return null;
  return <div className="ui-modal-backdrop fixed inset-0 z-50 flex items-center justify-center">
    <div className="ui-modal mx-4 w-full max-w-md p-6">
      <div className="mb-4 flex items-center justify-between">
        <h2 className="text-xl font-semibold">{t('cost.breakdown')}</h2>
        <button type="button" onClick={onClose} className="ui-icon-button transition" aria-label={t('common.close')}><X size={24} /></button>
      </div>
      <div className="ui-modal-header mb-6 space-y-3">
        <CostRow label={t('cost.filament')} value={formatCurrency(cost.filament_cost, { fallback: VALUE_FALLBACKS.notCalculated })} />
        <CostRow label={t('cost.electricity')} value={formatCurrency(cost.electricity_cost, { fallback: VALUE_FALLBACKS.notCalculated })} />
        <CostRow label={t('cost.wear')} value={formatCurrency(cost.wear_cost, { fallback: VALUE_FALLBACKS.notCalculated })} />
        <CostRow label={t('cost.labor')} value={formatCurrency(cost.labor_cost, { fallback: VALUE_FALLBACKS.notCalculated })} />
        <div className="flex justify-between pt-2 text-base font-semibold"><span className="ui-text-primary">{t('cost.total')}</span><span className="ui-text-primary-action">{formatCurrency(cost.total_cost, { fallback: VALUE_FALLBACKS.notCalculated })}</span></div>
      </div>
      <button type="button" onClick={onClose} className="button-secondary mt-6 w-full px-4 py-2 font-medium transition">{t('common.close')}</button>
    </div>
  </div>;
}
