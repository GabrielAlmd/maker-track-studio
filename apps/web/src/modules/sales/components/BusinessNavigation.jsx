import { NavLink } from 'react-router-dom';
import { useTranslation } from 'react-i18next';

export default function BusinessNavigation() {
  const { t } = useTranslation();
  return (
    <nav aria-label={t('nav.sales')} className="mb-5 flex flex-wrap gap-2">
      {[
        ['overview', '/business'],
        ['proposals', '/business/proposals'],
        ['orders', '/business/orders'],
        ['budgets', '/business/budgets'],
        ['customers', '/business/customers'],
      ].map(([key, path]) => (
        <NavLink key={key} end={key === 'overview'} to={path} className={({ isActive }) => `page-action ${isActive ? '' : 'page-action-secondary'}`}>
          {t(`business.${key}`)}
        </NavLink>
      ))}
    </nav>
  );
}
