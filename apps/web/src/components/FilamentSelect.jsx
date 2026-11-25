import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import SearchableSelect from './ui/SearchableSelect';

const label = f => [f.brand, f.type].filter(Boolean).join(' ') + (f.color ? ' — ' + f.color : '');
export default function FilamentSelect({ filaments, ...props }) {
  const { t } = useTranslation();
  const options = useMemo(() => [...(filaments ?? [])].sort((a, b) =>
    label(a).localeCompare(label(b), undefined, { sensitivity: 'base', numeric: true }) ||
    String(a.id).localeCompare(String(b.id))), [filaments]);
  return <SearchableSelect placeholder={props.clearLabel ?? t('purchases.selectFilament')} {...props} options={options} getLabel={label}
    emptyLabel={t('filamentSelect.noResults')} toggleLabel={t('filamentSelect.showOptions')} />;
}
