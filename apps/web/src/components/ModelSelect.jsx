import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import SearchableSelect from './ui/SearchableSelect';

export default function ModelSelect({ models, emptyLabel, ...props }) {
  const { t } = useTranslation();
  const options = useMemo(() => [...(models ?? [])].sort((a, b) =>
    a.name.localeCompare(b.name, undefined, { sensitivity: 'base', numeric: true }) ||
    String(a.id).localeCompare(String(b.id))), [models]);
  return <SearchableSelect {...props} options={options} clearLabel={emptyLabel ?? t('printJobs.noModel')}
    placeholder={t('modelSelect.placeholder')} emptyLabel={t('modelSelect.noResults')} toggleLabel={t('modelSelect.showOptions')} />;
}
