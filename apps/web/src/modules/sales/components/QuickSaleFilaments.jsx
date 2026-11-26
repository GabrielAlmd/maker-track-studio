import { useTranslation } from 'react-i18next';
import SearchableSelect from '../../../components/ui/SearchableSelect';
import { formatCurrency, formatNumber } from '../../../utils/formatters';

export default function QuickSaleFilaments({ choices, onSelect }) {
  const { t } = useTranslation();
  return choices.length > 0 && <div className="space-y-3 text-sm sm:col-span-2">
    {choices.map(choice => {
      const material = [choice.requirement?.material_type, choice.requirement?.color].filter(Boolean).join(' — ');
      const label = choice.legacyWeight ? t('business.quickLegacyFilament') : t('business.quickRequirementFilament', { material });
      const filamentLabel = filament => t('business.quickPricedFilament', {
        name: [filament.brand, filament.type, filament.color].filter(Boolean).join(' · '),
        price: formatCurrency(filament.avg_price),
      });
      return <div key={choice.key} className="space-y-1">
        {choice.legacyWeight && <p className="ui-text-secondary">{t('business.quickLegacyWeight', { weight: formatNumber(choice.legacyWeight, { maximumFractionDigits: 3 }) })}</p>}
        {choice.options.length ? <SearchableSelect label={label} options={choice.options} getLabel={filamentLabel}
          placeholder={t('business.quickSelectFilament')} required clearable={false} value={choice.value}
          onChange={value => onSelect(choice.key, value)} /> :
          <p className="ui-text-secondary">{material ? t('business.quickNoPricedFilaments', { material }) : t('business.quickNoPricedFilamentsForWeight')}</p>}
        {choice.substitutesColor && <p className="ui-text-secondary">{t('business.quickColorSubstitution')}</p>}
      </div>;
    })}
  </div>;
}
