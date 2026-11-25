import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { uniqueBrands } from '../utils/brands';
import SearchableSelect from './ui/SearchableSelect';

const identity = value => value;
export default function BrandSelect({ brands, ...props }) {
  const { t } = useTranslation();
  const options = useMemo(() => uniqueBrands(brands), [brands]);
  return <SearchableSelect {...props} options={options} getKey={identity} getLabel={identity} clearable={false}
    createLabel={brand => t('brandSelect.create', { brand })} placeholder={t('brandSelect.placeholder')}
    emptyLabel={t('brandSelect.noBrands')} toggleLabel={t('brandSelect.showOptions')} />;
}
