import { useTranslation } from 'react-i18next';
import { StatusBadge } from '../../../components/ui/PageState';

const fulfilmentTones = {
  draft: 'neutral',
  confirmed: 'primary',
  preparing: 'warning',
  ready_to_ship: 'warning',
  shipped: 'primary',
  completed: 'success',
  cancelled: 'danger',
};

const paymentTones = { unpaid: 'warning', paid: 'success', refunded: 'neutral' };

export default function SalesOrderStatus({ fulfilmentStatus, paymentStatus, labelled = false }) {
  const { t } = useTranslation();
  const statuses = [
    {
      key: 'fulfilment',
      label: t('sales.fulfilmentStatus'),
      value: t(`sales.fulfilment.${fulfilmentStatus}`),
      tone: fulfilmentTones[fulfilmentStatus],
    },
    {
      key: 'payment',
      label: t('sales.paymentStatus'),
      value: t(`sales.payment.${paymentStatus}`),
      tone: paymentTones[paymentStatus],
    },
  ];

  return (
    <div className={`flex ${labelled ? 'flex-wrap gap-4' : 'flex-col items-start gap-1'}`}>
      {statuses.map((status) => (
        <div key={status.key} className={labelled ? 'flex items-center gap-2' : undefined}>
          {labelled ? <span className="text-xs ui-text-muted">{status.label}</span> : null}
          <StatusBadge tone={status.tone} label={`${status.label}: ${status.value}`}>{status.value}</StatusBadge>
        </div>
      ))}
    </div>
  );
}
